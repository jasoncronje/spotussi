// Package hydrate fills the catalog (D9, D17; schema in migration 0005)
// from Spotify: tracks, albums, artists with genres, and play contexts.
//
//   - Seeding: live plays carry full track objects, which are upserted
//     straight from the raw capture tables (seed.go).
//   - Planning: every unhydrated track URI of the export is queued,
//     most-played first (enqueue.go). Hydrated tracks queue their
//     artists (for images and genres) at the track's priority; albums
//     come with every track object and need no calls of their own.
//     Contexts seen live are queued for D17.
//   - Order: Spotify lookups take every due track, album and context
//     before any artist (D24). With a quota of about 500 lookups a day,
//     tracks are what only Spotify can give (real artist IDs, credits,
//     the canonical album), while the fallbacks and Borrow already
//     supply artist images and genres. Artists follow, by priority.
//   - The worker sends one request per entity through the shared
//     limiter's background lane, so live capture keeps its reserve and a
//     429 pauses everything (Retry-After). Catalog lookups also have a
//     daily quota (429 QUOTA_EXCEEDED, Retry-After until the reset): it
//     pauses only catalog requests, and the worker leaves catalog
//     entries until the reset, recording how many lookups the quota
//     allowed for the ETA. Playlist contexts are outside that quota
//     (D24) and keep resolving meanwhile; a quota on playlists pauses
//     only them.
//     Failures back off per entry (attempts, next_attempt_at); 404s and
//     invalid IDs are parked for 30 days. The queue lives in SQLite, so a
//     restart resumes it.
//
// After each batch of catalog changes the worker calls OnCatalogChanged,
// the derivation engine's cue to rebuild (M5).
package hydrate

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/spotify"
)

// Spotify is the metadata source (D9: pluggable, so DF5 sources can be
// added). *spotify.Client implements it.
type Spotify interface {
	Track(ctx context.Context, id string) (*spotify.Track, error)
	Artist(ctx context.Context, id string) (*spotify.Artist, error)
	Album(ctx context.Context, id string) (*spotify.Album, error)
	Playlist(ctx context.Context, id string) (*spotify.Playlist, error)
}

// Options configure a Hydrator.
type Options struct {
	DB *db.DB
	// Spotify is the primary source (nil: fallbacks only).
	Spotify Spotify
	// Fallbacks are the free public sources that fill what Spotify
	// hasn't hydrated (D27).
	Fallbacks Fallbacks
	// OnCatalogChanged is called (from the worker goroutine; it must not
	// block for long) after a batch of catalog changes: at most once per
	// ChangeInterval while hydrating, and when the queue runs dry.
	OnCatalogChanged func()
	// ChangeInterval bounds how often OnCatalogChanged fires while the
	// queue is busy (default 2 min), ChangeBatch is the number of changed
	// entities that fires it sooner (default 1000).
	ChangeInterval time.Duration
	ChangeBatch    int
	// Workers is the number of requests in flight (default 2); the
	// limiter sets the actual rate.
	Workers int
	// BatchSize is how many due entries are taken from the queue at a
	// time (default 8).
	BatchSize int
	// PlanInterval is how often export rows are checked for new tracks
	// (imports by the CLI run in another process; default 1 min).
	PlanInterval time.Duration
	// LimiterStats feeds the ETA before a throughput is measured.
	LimiterStats func() spotify.LimiterStats
	// Now defaults to time.Now.
	Now func() time.Time
}

// Worker states reported in system status.
const (
	StateIdle       = "idle"           // nothing due
	StateHydrating  = "hydrating"      // working through the queue
	StateWaiting    = "waiting"        // Spotify not connected or needs a reconnect
	StateBackingOff = "backing_off"    // rate limited or failing; retrying later
	StateQuota      = "quota_exceeded" // Spotify's lookup quota is spent until ResumesAt
	StateDisabled   = "disabled"       // not running (HYDRATE_ENABLED=false)
)

const (
	// parkFor is how long a permanent failure (404, invalid ID) waits
	// before being tried again: Spotify sometimes restores tracks.
	parkFor = 30 * 24 * time.Hour
	// maxBackoff caps the per-entry backoff of transient failures.
	maxBackoff = 6 * time.Hour
	// throughputWindow is the span the measured rate is averaged over.
	throughputWindow = 5 * time.Minute
)

// Hydrator runs seeding, planning and the hydration worker.
type Hydrator struct {
	db  *db.DB
	sp  Spotify
	o   Options
	now func() time.Time

	seedMu sync.Mutex
	planMu sync.Mutex

	wake     chan struct{} // re-check the queue
	seedWake chan struct{} // new live rows
	planWake chan struct{} // new export rows

	mu          sync.Mutex
	state       string
	lastError   string
	lastErrorAt time.Time
	lastDoneAt  time.Time
	done        []time.Time // completions within throughputWindow
	requests    int64       // since start
	pending     int         // changes not yet announced
	pendingAt   time.Time   // first unannounced change

	// Quota windows: lookups that succeeded since the last quota reset
	// (or since start), and whether the window began at a reset (then
	// its count is the whole quota).
	resumesAt     time.Time
	windowCount   int64
	windowAtReset bool
	quotaPerDay   int64 // observed lookups per quota window (0: unknown)
	// paused are the quota buckets (spotify.Bucket*) whose entries wait
	// for a reset, with when each pause ends.
	paused map[string]time.Time

	fb     map[string]*fbStats      // per fallback source, under mu
	fbWake map[string]chan struct{} // re-check a fallback stage
	// borrowMu serializes Borrow passes; lastBorrow (under mu) is the
	// last one's start.
	borrowMu   sync.Mutex
	lastBorrow time.Time
}

// quotaKey stores the observed lookups per quota window.
const quotaKey = "hydrate_quota_lookups"

// quotaPause stops a batch when a lookup quota is spent.
type quotaPause struct {
	bucket string
	until  time.Time
}

func (q *quotaPause) Error() string {
	what := "lookup"
	if q.bucket == spotify.BucketPlaylists {
		what = "playlist lookup"
	}
	return "Spotify's " + what + " quota for this developer account is used up; those lookups resume at " +
		q.until.UTC().Format(time.RFC3339)
}

// playlistEntry is the SQL condition for queue entries that are
// playlist contexts (the playlists quota bucket); every other entry
// that makes a request is in the catalog bucket.
const playlistEntry = `(entity_type = 'context' AND (id LIKE 'spotify:playlist:%' OR id LIKE 'spotify:user:%:playlist:%'))`

// bucketOf is the quota bucket an entry's lookup counts against.
func bucketOf(it item) string {
	if it.typ == "context" {
		if typ, _ := ParseContext(it.id); typ == "playlist" {
			return spotify.BucketPlaylists
		}
	}
	return spotify.BucketCatalog
}

// pausedBuckets returns the buckets still paused for a quota at now.
func (h *Hydrator) pausedBuckets(now time.Time) map[string]time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[string]time.Time{}
	for b, u := range h.paused {
		if u.After(now) {
			out[b] = u
		} else {
			delete(h.paused, b)
		}
	}
	return out
}

// New builds a Hydrator.
func New(o Options) *Hydrator {
	if o.ChangeInterval <= 0 {
		o.ChangeInterval = 2 * time.Minute
	}
	if o.ChangeBatch <= 0 {
		o.ChangeBatch = 1000
	}
	if o.Workers <= 0 {
		o.Workers = 2
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 8
	}
	if o.PlanInterval <= 0 {
		o.PlanInterval = time.Minute
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	h := &Hydrator{db: o.DB, sp: o.Spotify, o: o, now: o.Now, state: StateIdle,
		wake: make(chan struct{}, 1), seedWake: make(chan struct{}, 1), planWake: make(chan struct{}, 1),
		fb: map[string]*fbStats{}, fbWake: map[string]chan struct{}{}, paused: map[string]time.Time{}}
	if o.Spotify == nil {
		h.state = StateDisabled
	}
	for _, src := range FallbackSources {
		st := &fbStats{state: FallbackIdle}
		if !o.Fallbacks.enabled(src) {
			st.state = FallbackDisabled
		}
		h.fb[src] = st
		h.fbWake[src] = make(chan struct{}, 1)
	}
	return h
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (h *Hydrator) wakeWorker() { signal(h.wake) }

// Wake re-checks the queue now, e.g. after Spotify is (re)connected.
func (h *Hydrator) Wake() { signal(h.wake) }

// Captured tells the hydrator that live capture stored new rows; they are
// seeded into the catalog shortly. It never blocks.
func (h *Hydrator) Captured() { signal(h.seedWake) }

// Imported tells the hydrator that an import finished; its tracks are
// queued right away. It never blocks.
func (h *Hydrator) Imported() { signal(h.planWake) }

func (h *Hydrator) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := h.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// changed records n catalog changes for the next OnCatalogChanged.
func (h *Hydrator) changed(n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pending == 0 {
		h.pendingAt = h.now()
	}
	h.pending += n
}

// announce calls OnCatalogChanged if changes are pending and either force
// is set or the batch is big or old enough.
func (h *Hydrator) announce(force bool) {
	h.mu.Lock()
	due := h.pending > 0 && (force || h.pending >= h.o.ChangeBatch || h.now().Sub(h.pendingAt) >= h.o.ChangeInterval)
	if due {
		h.pending = 0
	}
	h.mu.Unlock()
	if due && h.o.OnCatalogChanged != nil {
		h.o.OnCatalogChanged()
	}
}

func (h *Hydrator) setState(s string) {
	h.mu.Lock()
	was := h.state
	h.state = s
	h.mu.Unlock()
	if was == StateHydrating && s != StateHydrating {
		h.wakeFallbacks() // Spotify stalled: the fallbacks' turn
	}
}

func (h *Hydrator) fail(err error) {
	h.mu.Lock()
	h.lastError, h.lastErrorAt = err.Error(), h.now()
	h.mu.Unlock()
}

// Run seeds, plans and hydrates until ctx is done.
func (h *Hydrator) Run(ctx context.Context) {
	if q, err := readMark(ctx, h.db.Read, quotaKey); err == nil {
		h.mu.Lock()
		h.quotaPerDay = q
		h.mu.Unlock()
	}
	if err := h.repair(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("hydrate: repair queue", "err", err)
	}
	if _, err := h.Rematch(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("hydrate: re-try unmatched fallback tracks", "err", err)
	}
	h.housekeep(ctx, true, true)
	var fbWG sync.WaitGroup
	for _, src := range FallbackSources {
		if h.o.Fallbacks.enabled(src) {
			fbWG.Add(1)
			go func() { defer fbWG.Done(); h.runFallback(ctx, src) }()
		}
	}
	defer fbWG.Wait()
	planTick := time.NewTicker(h.o.PlanInterval)
	defer planTick.Stop()
	for ctx.Err() == nil {
		seed, plan := false, false
		select {
		case <-h.seedWake:
			seed = true
		default:
		}
		select {
		case <-h.planWake:
			plan = true
		case <-planTick.C:
			plan = true
		default:
		}
		h.housekeep(ctx, seed, plan)

		wait, err := h.step(ctx)
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			slog.Warn("hydrate: step failed", "err", err)
			h.fail(err)
			h.setState(StateBackingOff)
			wait = time.Minute
		}
		if wait <= 0 {
			h.announce(false)
			continue
		}
		h.announce(true) // queue dry or paused: announce what is done
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
		case <-t.C:
		case <-h.wake:
		case <-h.seedWake:
			signal(h.seedWake) // handled at the top of the loop
		case <-h.planWake:
			signal(h.planWake)
		case <-planTick.C:
			signal(h.planWake)
		}
		t.Stop()
	}
	h.announce(true)
}

func (h *Hydrator) housekeep(ctx context.Context, seed, plan bool) {
	if seed {
		if _, err := h.Seed(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("hydrate: seed from live capture", "err", err)
		}
	}
	if plan {
		n, err := h.Plan(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Warn("hydrate: queue export tracks", "err", err)
		} else if n > 0 {
			slog.Info("hydrate: queued export tracks", "entries", n)
		}
	}
}

type item struct {
	typ, id  string
	priority int64
	attempts int
}

// step hydrates one batch of due entries. It returns how long to wait
// before the next step (0: continue right away).
func (h *Hydrator) step(ctx context.Context) (time.Duration, error) {
	if h.sp == nil {
		return time.Hour, nil // fallbacks only: woken by seeding and planning
	}
	// Entries of a bucket paused for a quota wait for its reset; the
	// others carry on.
	paused := h.pausedBuckets(h.now())
	_, catalogPaused := paused[spotify.BucketCatalog]
	_, playlistsPaused := paused[spotify.BucketPlaylists]
	filter := ""
	switch {
	case catalogPaused && playlistsPaused:
		filter = " AND 0"
	case catalogPaused:
		filter = " AND " + playlistEntry
	case playlistsPaused:
		filter = " AND NOT " + playlistEntry
	}
	busy := StateHydrating
	if catalogPaused {
		// Still quota_exceeded while playlist contexts resolve: the
		// catalog is what the fallbacks fill meanwhile.
		busy = StateQuota
	}
	now := h.now().UnixMilli()
	rs, err := h.db.Read.QueryContext(ctx, `SELECT entity_type, id, priority, attempts FROM hydration_queue
		WHERE next_attempt_at <= ?`+filter+` ORDER BY entity_type = 'artist', priority DESC, entity_type, id LIMIT ?`, now, h.o.BatchSize)
	if err != nil {
		return 0, err
	}
	var items []item
	for rs.Next() {
		var it item
		if err := rs.Scan(&it.typ, &it.id, &it.priority, &it.attempts); err != nil {
			rs.Close()
			return 0, err
		}
		items = append(items, it)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		if catalogPaused {
			h.setState(StateQuota)
		} else {
			h.setState(StateIdle)
		}
		wait := time.Hour // woken by seeding, planning or connecting
		for _, u := range paused {
			wait = min(wait, time.Until(u))
		}
		var next sql.NullInt64
		if err := h.db.Read.QueryRowContext(ctx, `SELECT MIN(next_attempt_at) FROM hydration_queue
			WHERE 1`+filter).Scan(&next); err != nil {
			return 0, err
		}
		if next.Valid {
			wait = min(wait, time.Until(time.UnixMilli(next.Int64)))
		}
		return max(wait, time.Second), nil
	}
	h.setState(busy)

	bctx, cancel := context.WithCancel(spotify.WithPriority(ctx, spotify.PriorityBackground))
	defer cancel()
	var mu sync.Mutex
	var pause error // not connected: stop the batch
	work := make(chan item)
	var wg sync.WaitGroup
	for range h.o.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range work {
				if err := h.hydrate(bctx, it); err != nil {
					mu.Lock()
					if pause == nil {
						pause = err
					}
					mu.Unlock()
					cancel()
				}
			}
		}()
	}
feed:
	for _, it := range items {
		select {
		case work <- it:
		case <-bctx.Done():
			break feed
		}
	}
	close(work)
	wg.Wait()
	if pause != nil && ctx.Err() == nil {
		h.fail(pause)
		var q *quotaPause
		if errors.As(pause, &q) {
			if q.bucket == spotify.BucketCatalog {
				h.setState(StateQuota)
			}
			return 0, nil // the next step leaves the paused bucket's entries
		}
		h.setState(StateWaiting)
		return 5 * time.Minute, nil // or until OnConnect wakes us
	}
	if !catalogPaused {
		h.mu.Lock()
		h.resumesAt = time.Time{}
		h.mu.Unlock()
	}
	return 0, nil
}

// hydrate looks up one entry and stores the result. It returns an error
// only when the whole worker should pause (Spotify not connected);
// everything else is recorded on the entry.
func (h *Hydrator) hydrate(ctx context.Context, it item) error {
	var requested bool
	var err error
	switch it.typ {
	case "track":
		requested = true
		var t *spotify.Track
		if t, err = h.sp.Track(ctx, it.id); err == nil {
			if t.ID != "" && t.ID != it.id {
				slog.Debug("hydrate: Spotify answered with another track", "requested", it.id, "got", t.ID)
			}
			err = h.tx(ctx, func(tx *sql.Tx) error {
				return storeTrack(ctx, tx, it.id, t, SourceAPI, h.now().UnixMilli(), max(it.priority, 1))
			})
		}
	case "artist":
		requested = true
		var a *spotify.Artist
		if a, err = h.sp.Artist(ctx, it.id); err == nil {
			err = h.tx(ctx, func(tx *sql.Tx) error { return storeArtist(ctx, tx, it.id, a, h.now().UnixMilli()) })
		}
	case "album":
		requested = true
		var a *spotify.Album
		if a, err = h.sp.Album(ctx, it.id); err == nil {
			err = h.tx(ctx, func(tx *sql.Tx) error {
				return storeAlbum(ctx, tx, &a.SimpleAlbum, h.now().UnixMilli(), max(it.priority, 1))
			})
		}
	case "context":
		requested, err = h.resolveContext(ctx, it.id, max(it.priority, 1))
	}
	if requested && err == nil {
		h.mu.Lock()
		h.requests++
		if bucketOf(it) == spotify.BucketCatalog {
			h.windowCount++ // playlists don't count against the catalog quota
		}
		h.mu.Unlock()
	}
	return h.settle(ctx, it, err)
}

// settle records the outcome of one entry.
func (h *Hydrator) settle(ctx context.Context, it item, err error) error {
	var rl *spotify.RateLimitedError
	switch {
	case err == nil:
		now := h.now()
		h.mu.Lock()
		h.lastDoneAt = now
		h.done = append(h.done, now)
		cut := 0
		for cut < len(h.done) && now.Sub(h.done[cut]) > throughputWindow {
			cut++
		}
		h.done = h.done[cut:]
		h.mu.Unlock()
		h.changed(1)
		return nil
	case ctx.Err() != nil:
		return nil // shutting down or batch cancelled: retried later
	case errors.Is(err, spotify.ErrNotConnected), errors.Is(err, spotify.ErrReconnectRequired):
		return err
	case errors.As(err, &rl) && rl.QuotaExceeded:
		return h.quotaSpent(ctx, rl)
	case errors.As(err, &rl):
		// The limiter already paused every lane for Retry-After; the
		// entry is simply tried again, without counting an attempt.
		h.fail(err)
		return nil
	}
	now := h.now()
	code := spotify.StatusOf(err)
	permanent := errors.Is(err, spotify.ErrInvalidID) || code == http.StatusNotFound || code == http.StatusBadRequest
	if it.typ == "context" && (permanent || code == http.StatusForbidden) {
		// An album or artist context that no longer exists: done,
		// unresolved.
		return h.tx(ctx, func(tx *sql.Tx) error { return setContext(ctx, tx, it.id, "", "", nil, now.UnixMilli()) })
	}
	var next time.Time
	msg := err.Error()
	if permanent {
		next, msg = now.Add(parkFor), permanentPrefix+msg
	} else {
		next = now.Add(min(time.Minute<<min(it.attempts, 20), maxBackoff))
		h.fail(err)
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	_, werr := h.db.Write.ExecContext(ctx, `UPDATE hydration_queue SET attempts = attempts + 1, next_attempt_at = ?,
		last_error = ? WHERE entity_type = ? AND id = ?`, next.UnixMilli(), msg, it.typ, it.id)
	return werr
}

// permanentPrefix marks entries parked after a permanent failure.
const permanentPrefix = "permanent: "

// quotaSpent handles QUOTA_EXCEEDED: the entry is not charged, the batch
// stops, and the bucket's entries wait until the quota resets. For the
// catalog bucket the lookups the window allowed are recorded (exactly
// when the window began at a reset, else as a lower bound).
func (h *Hydrator) quotaSpent(ctx context.Context, rl *spotify.RateLimitedError) error {
	until := h.now().Add(rl.Backoff)
	bucket := rl.Bucket
	if bucket == "" {
		bucket = spotify.BucketCatalog
	}
	h.mu.Lock()
	if until.After(h.paused[bucket]) {
		h.paused[bucket] = until
	}
	if bucket != spotify.BucketCatalog {
		h.mu.Unlock()
		slog.Info("hydrate: quota spent; pausing these lookups", "bucket", bucket,
			"resumes_at", until.UTC().Format(time.RFC3339))
		return &quotaPause{bucket: bucket, until: until}
	}
	first := h.resumesAt.IsZero() || until.After(h.resumesAt.Add(time.Minute))
	var record int64
	if first && !rl.NotSent {
		if h.windowAtReset || h.windowCount > h.quotaPerDay {
			record = h.windowCount
		}
		if record > 0 {
			h.quotaPerDay = record
		}
		h.windowCount, h.windowAtReset = 0, true
	}
	h.resumesAt = until
	h.mu.Unlock()
	if record > 0 {
		slog.Info("hydrate: lookup quota spent", "lookups_in_window", record, "resumes_at", until.UTC().Format(time.RFC3339))
		wctx := context.WithoutCancel(ctx) // the batch is being cancelled
		if err := h.tx(wctx, func(tx *sql.Tx) error {
			return writeMark(wctx, tx, quotaKey, record, h.now().UnixMilli())
		}); err != nil {
			slog.Warn("hydrate: store observed quota", "err", err)
		}
	}
	return &quotaPause{bucket: bucket, until: until}
}
