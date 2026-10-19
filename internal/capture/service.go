package capture

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/spotify"
)

// Spotify is what capture needs from the Spotify client.
type Spotify interface {
	Player(ctx context.Context) (*spotify.PlayerState, []byte, error)
	RecentlyPlayed(ctx context.Context, afterMs int64, limit int) (*spotify.RecentPage, error)
}

// Options configure a Service.
type Options struct {
	DB      *db.DB
	Spotify Spotify
	// Intervals default to DefaultIntervals (zero fields too).
	Intervals Intervals
	// RecentInterval is the recently-played backstop period (default 20 min).
	RecentInterval time.Duration
	Machine        Config
	// OnStored is called after new raw rows were committed (completed
	// plays or backstop items), e.g. to seed the catalog from them (M4).
	// It must not block.
	OnStored func()
	// Now defaults to time.Now.
	Now func() time.Time
	// OnNewRows, if set, is called (without blocking) after new raw rows
	// are stored: the derivation engine folds them into plays.
	OnNewRows func()
}

// Capture states reported in system status.
const (
	StateStopped    = "stopped"     // not connected, reconnect needed, or not running
	StateIdle       = "idle"        // paused or nothing playing
	StatePlaying    = "playing"     // a track is playing
	StateBackingOff = "backing_off" // rate limited or errors; retrying later
)

// Status is the poller's view for system status.
type Status struct {
	State        string
	PollInterval time.Duration // current interval until the next poll
	LastPollAt   time.Time     // last successful poll
	NextPollAt   time.Time
	LastError    string
	LastErrorAt  time.Time
	LastPlayAt   time.Time // end of the last completed play written
	PlaysWritten int64     // since start

	RecentLastRunAt time.Time
	RecentLastError string
	RecentInserted  int64 // since start

	Calibration *Calibration
}

// Service runs the player poller and the recently-played backstop.
type Service struct {
	db       *db.DB
	sp       Spotify
	iv       Intervals
	recentIv time.Duration
	mcfg     Config
	onStored func()
	now      func() time.Time
	onNew    func()

	pollMu sync.Mutex // one poll at a time (the machine is not concurrent)
	m      *Machine

	mu          sync.Mutex
	status      Status
	errStreak   int
	lastState   *spotify.PlayerState // last successful snapshot, for now-playing
	lastAt      int64
	openStarted int64 // estimated start of the open play (0: none)

	wakePoll     chan struct{} // poll now instead of waiting out the interval
	wakeBackstop chan struct{} // run the backstop now
}

// Wake makes the poller poll and the backstop run right away, e.g. just
// after Spotify is (re)connected. It never blocks.
func (s *Service) Wake() {
	for _, ch := range []chan struct{}{s.wakePoll, s.wakeBackstop} {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// New builds a Service, resuming the poller's persisted state.
func New(ctx context.Context, o Options) (*Service, error) {
	s := &Service{db: o.DB, sp: o.Spotify, iv: o.Intervals, recentIv: o.RecentInterval, mcfg: o.Machine, onStored: o.OnStored, now: o.Now, onNew: o.OnNewRows,
		wakePoll: make(chan struct{}, 1), wakeBackstop: make(chan struct{}, 1)}
	d := DefaultIntervals
	if s.iv.Playing <= 0 {
		s.iv.Playing = d.Playing
	}
	if s.iv.Idle <= 0 {
		s.iv.Idle = d.Idle
	}
	if s.iv.EndSlack <= 0 {
		s.iv.EndSlack = d.EndSlack
	}
	if s.iv.Min <= 0 {
		s.iv.Min = d.Min
	}
	if s.recentIv <= 0 {
		s.recentIv = 20 * time.Minute
	}
	if s.now == nil {
		s.now = time.Now
	}
	st, err := loadState(ctx, o.DB)
	if err != nil {
		return nil, err
	}
	s.m = NewMachine(s.mcfg, st)
	s.status.State = StateStopped
	if cal, err := s.Calibration(ctx); err == nil {
		s.status.Calibration = cal
	}
	return s, nil
}

func loadState(ctx context.Context, d *db.DB) (MachineState, error) {
	var raw string
	var st MachineState
	err := d.Read.QueryRowContext(ctx, `SELECT state FROM capture_state WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("load capture state: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		slog.Warn("capture: discarding unreadable in-flight state", "err", err)
		return MachineState{}, nil
	}
	return st, nil
}

// Status returns the current status.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// NowPlaying returns the last successful player snapshot (nil if none
// yet or nothing was playing), when it was observed, and the estimated
// start of the play in progress (0 if none).
func (s *Service) NowPlaying() (st *spotify.PlayerState, observedAt, startedAt int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastState, s.lastAt, s.openStarted
}

// Run polls and runs the backstop until ctx is done.
func (s *Service) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.runBackstop(ctx)
	}()
	for {
		d := s.Poll(ctx)
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			wg.Wait()
			return
		case <-t.C:
		case <-s.wakePoll:
			t.Stop()
		}
	}
}

func (s *Service) runBackstop(ctx context.Context) {
	// First run shortly after start: catches plays made while the server
	// was down.
	t := time.NewTimer(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.wakeBackstop:
			t.Stop()
		}
		if err := s.Backstop(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("capture: recently-played backstop failed", "err", err)
		}
		t.Reset(s.recentIv)
	}
}

// ---------- poller ----------

// snapshotFrom converts a /me/player response into a Snapshot.
func snapshotFrom(st *spotify.PlayerState, raw []byte, at int64) Snapshot {
	if st == nil {
		return Snapshot{At: at, Kind: KindNothing}
	}
	s := Snapshot{At: at, Playing: st.IsPlaying, Shuffle: st.ShuffleState, Repeat: st.RepeatState, Raw: raw}
	if st.ProgressMs != nil {
		s.ProgressMs = *st.ProgressMs
	}
	if st.Context != nil {
		s.ContextURI = st.Context.URI
	}
	if st.Device != nil {
		s.DeviceName, s.DeviceType, s.Private = st.Device.Name, st.Device.Type, st.Device.IsPrivateSession
	}
	switch {
	case st.Item != nil && (st.Item.Type == "" || st.Item.Type == "track") && st.CurrentlyPlayingType != "episode":
		s.Kind = KindTrack
		s.Track = &Track{ID: st.Item.Key(), URI: st.Item.URI, DurationMs: st.Item.DurationMs}
	case st.CurrentlyPlayingType == "episode" || (st.Item != nil && st.Item.Type == "episode"):
		s.Kind = KindEpisode
	case st.CurrentlyPlayingType == "ad":
		s.Kind = KindAd
	default:
		s.Kind = KindUnknown
	}
	return s
}

// Poll polls the player once, feeds the state machine, persists
// completed plays with the new state, and returns the delay until the
// next poll.
func (s *Service) Poll(ctx context.Context) time.Duration {
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	st, raw, err := s.sp.Player(spotify.WithPriority(ctx, spotify.PriorityLive))
	if err != nil {
		return s.pollFailed(ctx, err)
	}
	snap := snapshotFrom(st, raw, s.now().UnixMilli())

	prevJSON, _ := json.Marshal(s.m.State()) // before Observe mutates it
	plays := s.m.Observe(snap)
	if err := s.persist(ctx, plays, s.m.State()); err != nil {
		// Roll the machine back so the snapshot is replayed against the
		// stored state next time: nothing is lost or written twice.
		var restored MachineState
		_ = json.Unmarshal(prevJSON, &restored)
		s.m = NewMachine(s.mcfg, restored)
		return s.pollFailed(ctx, fmt.Errorf("store capture: %w", err))
	}

	if len(plays) > 0 && s.onStored != nil {
		s.onStored()
	}
	next := NextPoll(snap, s.iv)
	at := time.UnixMilli(snap.At)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errStreak = 0
	s.status.State = StateIdle
	if snap.Kind == KindTrack && snap.Playing {
		s.status.State = StatePlaying
	}
	s.status.LastPollAt, s.status.PollInterval, s.status.NextPollAt = at, next, at.Add(next)
	s.status.LastError, s.status.LastErrorAt = "", time.Time{}
	for _, p := range plays {
		s.status.PlaysWritten++
		s.status.LastPlayAt = time.UnixMilli(p.EndedMs)
	}
	s.lastState, s.lastAt, s.openStarted = st, snap.At, 0
	if o := s.m.Current(); o != nil {
		s.openStarted = o.StartedMs
	}
	return next
}

func (s *Service) pollFailed(ctx context.Context, err error) time.Duration {
	if ctx.Err() != nil {
		return s.iv.Idle
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.LastError, s.status.LastErrorAt = err.Error(), now
	var rl *spotify.RateLimitedError
	var next time.Duration
	switch {
	case errors.Is(err, spotify.ErrNotConnected):
		s.status.State, next = StateStopped, s.iv.Idle
		s.status.LastError = "Spotify is not connected"
	case errors.Is(err, spotify.ErrReconnectRequired):
		s.status.State, next = StateStopped, s.iv.Idle
		s.status.LastError = "Spotify access expired or was revoked: reconnect Spotify"
	case errors.As(err, &rl):
		s.status.State, next = StateBackingOff, max(rl.Backoff, s.iv.Playing)
	default:
		// Network errors and 5xx: retry quickly at first, then back off
		// towards the idle interval.
		s.errStreak++
		s.status.State = StateBackingOff
		next = min(s.iv.Playing<<min(s.errStreak-1, 10), s.iv.Idle)
		if s.errStreak == 1 || s.errStreak%10 == 0 {
			slog.Warn("capture: player poll failed", "err", err, "consecutive", s.errStreak)
		}
	}
	if s.status.State == StateStopped {
		s.lastState, s.openStarted = nil, 0 // no longer observable
	}
	s.status.PollInterval, s.status.NextPollAt = next, now.Add(next)
	return next
}

// persist writes completed plays and the machine state in one
// transaction.
func (s *Service) persist(ctx context.Context, plays []Play, st MachineState) error {
	state, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	now := s.now().UnixMilli()
	for _, p := range plays {
		raw := string(p.Raw)
		if raw == "" {
			raw = "null"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO raw_poller_plays
			(track_id, track_uri, duration_ms, started_ms, ended_ms, ms_played, skipped, end_reason, shuffle, repeat,
			 device_name, device_type, context_uri, private_session, seeks, pauses, device_switches, observations,
			 first_seen_ms, last_seen_ms, json, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (track_id, started_ms) DO NOTHING`,
			p.TrackID, p.TrackURI, p.DurationMs, p.StartedMs, p.EndedMs, p.MsPlayed, p.Skipped, p.EndReason, p.Shuffle,
			nullString(p.Repeat), nullString(p.DeviceName), nullString(p.DeviceType), nullString(p.ContextURI), p.Private,
			p.Seeks, p.Pauses, p.DeviceSwitches, p.Observations, p.FirstSeenMs, p.LastSeenMs, raw, now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO capture_state (id, state, updated_at) VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at`, string(state), now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if len(plays) > 0 && s.onNew != nil {
		s.onNew()
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ---------- recently-played backstop ----------

// maxRecentPages bounds one backstop run (50 plays per page).
const maxRecentPages = 10

// Backstop fetches recently-played after the newest stored item, stores
// new items (deduplicated on played_at + track), then updates the
// played_at calibration.
func (s *Service) Backstop(ctx context.Context) error {
	ctx = spotify.WithPriority(ctx, spotify.PriorityLive)
	var after int64
	if err := s.db.Read.QueryRowContext(ctx, `SELECT COALESCE(MAX(played_at_ms), 0) FROM raw_recent_plays`).Scan(&after); err != nil {
		return err
	}
	var inserted int64
	var runErr error
	for page := 0; page < maxRecentPages; page++ {
		p, err := s.sp.RecentlyPlayed(ctx, after, 50)
		if err != nil {
			runErr = err
			break
		}
		n, newest, err := s.storeRecent(ctx, p.Items)
		if err != nil {
			runErr = err
			break
		}
		inserted += n
		if len(p.Items) < 50 || newest <= after {
			break
		}
		after = newest
	}
	cal, calErr := s.calibrate(ctx)
	s.mu.Lock()
	s.status.RecentLastRunAt = s.now()
	s.status.RecentInserted += inserted
	s.status.RecentLastError = ""
	if runErr != nil {
		s.status.RecentLastError = runErr.Error()
	}
	if calErr == nil {
		s.status.Calibration = cal
	}
	s.mu.Unlock()
	if inserted > 0 {
		slog.Info("capture: recently-played backstop", "new_items", inserted)
		if s.onStored != nil {
			s.onStored()
		}
		if s.onNew != nil {
			s.onNew()
		}
	}
	if runErr != nil {
		return runErr
	}
	return calErr
}

// storeRecent inserts items and returns how many were new and the newest
// played_at among them.
func (s *Service) storeRecent(ctx context.Context, items []spotify.RecentItem) (inserted, newest int64, err error) {
	if len(items) == 0 {
		return 0, 0, nil
	}
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	now := s.now().UnixMilli()
	for _, it := range items {
		t, err := time.Parse(time.RFC3339Nano, it.PlayedAt)
		if err != nil {
			slog.Warn("capture: skipping recently-played item with bad played_at", "played_at", it.PlayedAt)
			continue
		}
		at := t.UnixMilli()
		newest = max(newest, at)
		var ctxURI any
		if it.Context != nil && it.Context.URI != "" {
			ctxURI = it.Context.URI
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO raw_recent_plays (played_at_ms, track_id, duration_ms, context_uri, json, fetched_at)
			VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (played_at_ms, track_id) DO NOTHING`,
			at, it.Track.Key(), it.Track.DurationMs, ctxURI, string(it.Raw), now)
		if err != nil {
			return 0, 0, err
		}
		n, _ := res.RowsAffected()
		inserted += n
	}
	return inserted, newest, tx.Commit()
}
