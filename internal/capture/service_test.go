package capture_test

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"spotussi/internal/capture"
	"spotussi/internal/db"
	"spotussi/internal/keyring"
	"spotussi/internal/spotify"
	"spotussi/internal/spotify/spotifytest"
)

const clientID = "0123456789abcdef0123456789abcdef"

var t0 = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

type env struct {
	t      *testing.T
	db     *db.DB
	fake   *spotifytest.Fake
	client *spotify.Client
	mu     sync.Mutex
	now    time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	k, _ := keyring.New(bytes.Repeat([]byte{7}, 32))
	f := spotifytest.New(clientID)
	t.Cleanup(f.Close)
	c := spotify.New(spotify.Options{DB: d, Keyring: k, AccountsURL: f.AccountsURL(), APIURL: f.APIURL(),
		Limiter: spotify.NewLimiter(spotify.LimiterConfig{Rate: 1000, Burst: 1000, MinBackoff: 20 * time.Millisecond}, nil)})
	e := &env{t: t, db: d, fake: f, client: c, now: t0}
	e.connect()
	return e
}

func (e *env) connect() {
	e.t.Helper()
	ctx := context.Background()
	if err := e.client.SetClientID(ctx, clientID); err != nil {
		e.t.Fatal(err)
	}
	req, err := e.client.BeginAuth(ctx, spotify.LoopbackRedirectURI(8080, ""), "/")
	if err != nil {
		e.t.Fatal(err)
	}
	back, err := e.fake.Approve(req.URL)
	if err != nil {
		e.t.Fatal(err)
	}
	p, err := spotify.ParseCallbackURL(back)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.client.Complete(ctx, p); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) clock() time.Time { e.mu.Lock(); defer e.mu.Unlock(); return e.now }
func (e *env) at(d time.Duration) {
	e.mu.Lock()
	e.now = t0.Add(d)
	e.mu.Unlock()
}

func (e *env) service(sp capture.Spotify) *capture.Service {
	e.t.Helper()
	if sp == nil {
		sp = e.client
	}
	s, err := capture.New(context.Background(), capture.Options{DB: e.db, Spotify: sp, Now: e.clock})
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

// observe sets the clock to t0+at, scripts the player and polls once.
func (e *env) observe(s *capture.Service, at time.Duration, p *spotifytest.PlayerSpec) time.Duration {
	e.at(at)
	e.fake.SetPlayer(p)
	return s.Poll(context.Background())
}

func playing(id string, progress time.Duration) *spotifytest.PlayerSpec {
	return &spotifytest.PlayerSpec{TrackID: id, DurationMs: 180_000, ProgressMs: progress.Milliseconds(), Playing: true,
		ContextURI: "spotify:playlist:p1", DeviceName: "Kitchen", DeviceType: "Speaker"}
}

type row struct {
	trackID              string
	started, ended, msPl int64
	skipped              bool
	reason               string
}

func (e *env) plays() []row {
	e.t.Helper()
	rs, err := e.db.Read.Query(`SELECT track_id, started_ms, ended_ms, ms_played, skipped, end_reason FROM raw_poller_plays ORDER BY started_ms`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rs.Close()
	var out []row
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.trackID, &r.started, &r.ended, &r.msPl, &r.skipped, &r.reason); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func near(got, want, tol int64) bool { return got >= want-tol && got <= want+tol }

func ms(d time.Duration) int64 { return t0.Add(d).UnixMilli() }

func TestServicePollsAndWritesPlays(t *testing.T) {
	e := newEnv(t)
	var stored atomic.Int32
	s, err := capture.New(context.Background(), capture.Options{DB: e.db, Spotify: e.client, Now: e.clock,
		OnStored: func() { stored.Add(1) }})
	if err != nil {
		t.Fatal(err)
	}
	if st := s.Status(); st.State != capture.StateStopped {
		t.Fatalf("before first poll: %+v", st)
	}
	// t1 plays from the start, polled every 12 s, until t2 takes over.
	for p := time.Duration(0); p < 180*time.Second; p += 12 * time.Second {
		next := e.observe(s, p, playing("t1", p))
		if next <= 0 || next > 12*time.Second {
			t.Fatalf("playing interval %s", next)
		}
	}
	st := s.Status()
	if st.State != capture.StatePlaying || st.PlaysWritten != 0 || !st.LastPollAt.Equal(t0.Add(168*time.Second)) {
		t.Fatalf("status while playing: %+v", st)
	}
	ps, _, started := s.NowPlaying()
	if ps == nil || ps.Item.ID != "t1" || !near(started, ms(0), 100) {
		t.Fatalf("now playing %+v from %d", ps, started)
	}
	// Near the end the poll is scheduled at the track end.
	if next := e.observe(s, 175*time.Second, playing("t1", 175*time.Second)); next > 7*time.Second {
		t.Fatalf("poll at track end: next in %s", next)
	}
	e.observe(s, 182*time.Second, playing("t2", 2*time.Second))
	if got := e.plays(); len(got) != 1 || got[0].trackID != "t1" || !near(got[0].started, ms(0), 100) ||
		!near(got[0].ended, ms(180*time.Second), 2500) || !near(got[0].msPl, 180_000, 2500) || got[0].skipped ||
		got[0].reason != capture.EndTrackChange {
		t.Fatalf("after track change: %+v", got)
	}
	// t2 plays ~40 s, then the player goes away (204). Stopping is not a
	// skip; the 12 s since the last poll are credited.
	e.observe(s, 194*time.Second, playing("t2", 14*time.Second))
	e.observe(s, 206*time.Second, playing("t2", 26*time.Second))
	e.observe(s, 218*time.Second, playing("t2", 38*time.Second))
	next := e.observe(s, 230*time.Second, nil)
	if next != capture.DefaultIntervals.Idle {
		t.Fatalf("idle interval %s", next)
	}
	got := e.plays()
	if len(got) != 2 || got[1].trackID != "t2" || got[1].skipped || got[1].reason != capture.EndStopped ||
		!near(got[1].msPl, 50_000, 100) {
		t.Fatalf("after 204: %+v", got)
	}
	st = s.Status()
	if st.State != capture.StateIdle || st.PlaysWritten != 2 || st.LastError != "" {
		t.Fatalf("idle status: %+v", st)
	}
	if n := stored.Load(); n != 2 {
		t.Errorf("OnStored called %d times, want 2 (once per poll that completed a play)", n)
	}
	if ps, _, _ := s.NowPlaying(); ps != nil {
		t.Fatalf("now playing after 204: %+v", ps)
	}
}

func TestServiceRateLimitAndTokenExpiry(t *testing.T) {
	e := newEnv(t)
	s := e.service(nil)
	e.observe(s, 0, playing("t1", 0))
	e.observe(s, 12*time.Second, playing("t1", 12*time.Second))

	// 429 with Retry-After: back off at least that long, keep the open play.
	e.fake.FailNext("/v1/me/player", spotifytest.Failure{Status: 429, RetryAfter: "1"})
	next := e.observe(s, 24*time.Second, playing("t1", 24*time.Second))
	st := s.Status()
	if next < time.Second || st.State != capture.StateBackingOff || !strings.Contains(st.LastError, "429") {
		t.Fatalf("after 429: next %s, %+v", next, st)
	}
	start := time.Now()
	e.observe(s, 36*time.Second, playing("t1", 36*time.Second))
	if waited := time.Since(start); waited < 900*time.Millisecond {
		t.Fatalf("poll went out %s after a Retry-After: 1", waited)
	}
	if st := s.Status(); st.State != capture.StatePlaying || st.LastError != "" {
		t.Fatalf("recovered: %+v", st)
	}

	// An expired access token is refreshed transparently.
	e.fake.ExpireAccessTokens()
	e.observe(s, 48*time.Second, playing("t1", 48*time.Second))
	if st := s.Status(); st.State != capture.StatePlaying {
		t.Fatalf("after access token expiry: %+v", st)
	}

	// A revoked refresh token stops capture until reconnected.
	e.fake.ExpireAccessTokens()
	e.fake.RevokeRefreshTokens()
	next = e.observe(s, 60*time.Second, playing("t1", 60*time.Second))
	st = s.Status()
	if st.State != capture.StateStopped || next != capture.DefaultIntervals.Idle || !strings.Contains(st.LastError, "reconnect") {
		t.Fatalf("revoked: next %s, %+v", next, st)
	}
	if ps, _, _ := s.NowPlaying(); ps != nil {
		t.Fatal("now playing while disconnected")
	}

	// Reconnected 30 s later, t1 is still playing: the same play continues.
	e.connect()
	e.observe(s, 90*time.Second, playing("t1", 90*time.Second))
	e.observe(s, 100*time.Second, playing("t2", 0))
	got := e.plays()
	if len(got) != 1 || got[0].trackID != "t1" || !near(got[0].started, ms(0), 100) || !near(got[0].msPl, 100_000, 2500) {
		t.Fatalf("plays: %+v", got)
	}
}

// flaky fails every call with a network error.
type flaky struct{ calls int }

func (f *flaky) Player(context.Context) (*spotify.PlayerState, []byte, error) {
	f.calls++
	return nil, nil, errors.New("dial tcp 1.2.3.4:443: connect: connection refused")
}

func (f *flaky) RecentlyPlayed(context.Context, int64, int) (*spotify.RecentPage, error) {
	return nil, errors.New("dial tcp 1.2.3.4:443: connect: connection refused")
}

func TestServiceNetworkErrorsBackOff(t *testing.T) {
	e := newEnv(t)
	f := &flaky{}
	s := e.service(f)
	iv := capture.DefaultIntervals
	var prev time.Duration
	for i := range 6 {
		next := s.Poll(context.Background())
		if next < prev || next > iv.Idle {
			t.Fatalf("poll %d: next %s after %s", i, next, prev)
		}
		prev = next
	}
	st := s.Status()
	if prev != iv.Idle || st.State != capture.StateBackingOff || !strings.Contains(st.LastError, "connection refused") {
		t.Fatalf("after errors: %s %+v", prev, st)
	}
	if err := s.Backstop(context.Background()); err == nil || !strings.Contains(s.Status().RecentLastError, "refused") {
		t.Fatalf("backstop: %v %+v", err, s.Status())
	}
}

func TestRestartMidTrack(t *testing.T) {
	e := newEnv(t)
	a := e.service(nil)
	for p := time.Duration(0); p <= 60*time.Second; p += 12 * time.Second {
		e.observe(a, p, playing("t1", p))
	}
	if len(e.plays()) != 0 {
		t.Fatal("open play written early")
	}
	// The process restarts; a new service resumes from capture_state.
	b := e.service(nil)
	if _, _, started := b.NowPlaying(); started != 0 {
		t.Fatal("now playing before the first poll")
	}
	for p := 80 * time.Second; p < 180*time.Second; p += 12 * time.Second {
		e.observe(b, p, playing("t1", p))
	}
	e.observe(b, 182*time.Second, playing("t2", 2*time.Second))
	got := e.plays()
	if len(got) != 1 || !near(got[0].started, ms(0), 100) || !near(got[0].msPl, 180_000, 2500) || got[0].skipped {
		t.Fatalf("one continuous play expected: %+v", got)
	}
	// Restart again after a long outage (t2 ended meanwhile, t3 plays).
	c := e.service(nil)
	e.observe(c, 900*time.Second, playing("t3", 30*time.Second))
	got = e.plays()
	if len(got) != 2 || got[1].trackID != "t2" || got[1].reason != capture.EndTrackChange {
		t.Fatalf("after long outage: %+v", got)
	}
	// t2 was playing when last seen: it is assumed to have played to its
	// natural end (never beyond its duration); the backstop's played_at
	// covers whatever played during the outage.
	if !near(got[1].msPl, 180_000, 100) || !near(got[1].ended, ms(360*time.Second), 100) {
		t.Fatalf("t2 across the outage: %+v", got[1])
	}
}

func TestBackstopDedupesAndCalibrates(t *testing.T) {
	e := newEnv(t)
	s := e.service(nil)
	// 25 one-minute tracks, polled every 10 s.
	const n = 25
	for i := range n {
		id := "c" + string(rune('a'+i))
		for p := time.Duration(0); p < 60*time.Second; p += 10 * time.Second {
			at := time.Duration(i)*time.Minute + p
			spec := playing(id, p)
			spec.DurationMs = 60_000
			e.observe(s, at, spec)
		}
	}
	e.observe(s, n*time.Minute+5*time.Second, nil)
	got := e.plays()
	if len(got) != n {
		t.Fatalf("%d poller plays", len(got))
	}
	// recently-played reports each play ~0.3 s after it ended.
	for i, p := range got {
		e.fake.AddRecent(spotifytest.RecentPlay{TrackID: p.trackID, DurationMs: 60_000,
			PlayedAt: time.UnixMilli(p.ended + 300 + int64(i%3)*100).UTC()})
	}
	ctx := context.Background()
	if err := s.Backstop(ctx); err != nil {
		t.Fatal(err)
	}
	if q := e.fake.Seen["recently-played"]; q.Has("after") || q.Get("limit") != "50" {
		t.Fatalf("first run query %v", q)
	}
	st := s.Status()
	if st.RecentInserted != n || st.RecentLastError != "" || st.RecentLastRunAt.IsZero() {
		t.Fatalf("backstop status %+v", st)
	}
	cal := st.Calibration
	if cal == nil || cal.Samples != n || cal.Discriminating != n || cal.Verdict != capture.AnchorEnd ||
		cal.End.HitRate != 1 || cal.Start.HitRate != 0 || !near(cal.End.MedianMs, 400, 100) ||
		!near(cal.Start.MedianMs, 60_400, 2600) {
		t.Fatalf("calibration %+v", cal)
	}

	// The next run asks only for plays after the newest stored one; one
	// new play arrives, the old ones are not stored twice.
	newest := got[n-1].ended + 300 + int64((n-1)%3)*100
	e.fake.AddRecent(spotifytest.RecentPlay{TrackID: "late", DurationMs: 60_000, PlayedAt: time.UnixMilli(newest + 120_000)})
	if err := s.Backstop(ctx); err != nil {
		t.Fatal(err)
	}
	if q := e.fake.Seen["recently-played"]; q.Get("after") != itoa(newest) {
		t.Fatalf("after cursor %q, want %d", q.Get("after"), newest)
	}
	var stored int
	if err := e.db.Read.QueryRow(`SELECT COUNT(*) FROM raw_recent_plays`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != n+1 || s.Status().RecentInserted != n+1 {
		t.Fatalf("stored %d, status %+v", stored, s.Status())
	}
	// Re-serving everything (e.g. the cursor is lost) inserts nothing.
	if _, err := e.db.Write.Exec(`DELETE FROM capture_calibration`); err != nil {
		t.Fatal(err)
	}
	if err := s.Backstop(ctx); err != nil {
		t.Fatal(err)
	}
	if s.Status().RecentInserted != n+1 {
		t.Fatalf("duplicates stored: %+v", s.Status())
	}
	// Calibration is recomputed idempotently.
	if cal := s.Status().Calibration; cal.Samples != n || cal.Verdict != capture.AnchorEnd {
		t.Fatalf("recalibrated %+v", cal)
	}
	// A new service reads the stored calibration at startup.
	if cal := e.service(nil).Status().Calibration; cal == nil || cal.Samples != n {
		t.Fatalf("calibration at startup: %+v", cal)
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func TestRunStopsWithContext(t *testing.T) {
	e := newEnv(t)
	e.fake.SetPlayer(playing("t1", time.Second))
	s, err := capture.New(context.Background(), capture.Options{DB: e.db, Spotify: e.client})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	for s.Status().LastPollAt.IsZero() {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if st := s.Status(); st.State != capture.StatePlaying || st.NextPollAt.IsZero() {
		t.Fatalf("status %+v", st)
	}
}

// Connecting Spotify wakes the poller and the backstop instead of
// leaving them to wait out long idle and backstop intervals.
func TestConnectWakesCapture(t *testing.T) {
	e := newEnv(t)
	s, err := capture.New(context.Background(), capture.Options{DB: e.db, Spotify: e.client,
		Intervals: capture.Intervals{Playing: time.Hour, Idle: time.Hour}, RecentInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	e.client.OnConnect(s.Wake)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	waitFor := func(what string, ok func(capture.Status) bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !ok(s.Status()) {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s: %+v", what, s.Status())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitFor("first poll", func(st capture.Status) bool { return !st.LastPollAt.IsZero() })
	firstPoll := s.Status().LastPollAt

	e.connect() // reconnect fires the OnConnect hook
	waitFor("woken poll", func(st capture.Status) bool { return st.LastPollAt.After(firstPoll) })
	waitFor("woken backstop", func(st capture.Status) bool { return !st.RecentLastRunAt.IsZero() })
}
