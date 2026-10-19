package spotify_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"spotussi/internal/spotify"
	"spotussi/internal/spotify/spotifytest"
)

func waitIn(l *spotify.Limiter, p spotify.Priority, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(spotify.WithPriority(context.Background(), p), timeout)
	defer cancel()
	return l.Wait(ctx)
}

func TestLimiterReservesLiveShare(t *testing.T) {
	// 5 tokens, 2 reserved for live capture, refilling at 1/s (so nothing
	// refills during the test).
	l := spotify.NewLimiter(spotify.LimiterConfig{Rate: 1, Burst: 5, LiveReserve: 0.4}, nil)
	for i := range 3 {
		if err := waitIn(l, spotify.PriorityBackground, 20*time.Millisecond); err != nil {
			t.Fatalf("background %d: %v", i, err)
		}
	}
	if err := waitIn(l, spotify.PriorityBackground, 30*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("background must not touch the live reserve: %v", err)
	}
	if err := waitIn(l, spotify.PriorityInteractive, 30*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("interactive must not touch the live reserve: %v", err)
	}
	for i := range 2 {
		if err := waitIn(l, spotify.PriorityLive, 20*time.Millisecond); err != nil {
			t.Fatalf("live %d: %v", i, err)
		}
	}
	st := l.Stats()
	if st.Requests["live"] != 2 || st.Requests["background"] != 3 || st.LiveReserve != 2 || st.Burst != 5 {
		t.Fatalf("stats: %+v", st)
	}
	if st.Waiting["background"] != 0 {
		t.Fatalf("abandoned waiters still counted: %+v", st.Waiting)
	}
}

func TestLimiterServesHigherLaneFirst(t *testing.T) {
	l := spotify.NewLimiter(spotify.LimiterConfig{Rate: 20, Burst: 1, LiveReserve: 0.01}, nil)
	if err := waitIn(l, spotify.PriorityBackground, time.Second); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var order []string
	var wg sync.WaitGroup
	run := func(p spotify.Priority) {
		defer wg.Done()
		if err := waitIn(l, p, 2*time.Second); err != nil {
			t.Error(err)
		}
		mu.Lock()
		order = append(order, p.String())
		mu.Unlock()
	}
	wg.Add(3)
	go run(spotify.PriorityBackground)
	for l.Stats().Waiting["background"] == 0 {
		time.Sleep(time.Millisecond)
	}
	go run(spotify.PriorityInteractive)
	for l.Stats().Waiting["interactive"] == 0 {
		time.Sleep(time.Millisecond)
	}
	go run(spotify.PriorityLive)
	wg.Wait()
	if len(order) != 3 || order[0] != "live" || order[1] != "interactive" || order[2] != "background" {
		t.Fatalf("served in order %v", order)
	}
}

func TestLimiterBackoff(t *testing.T) {
	l := spotify.NewLimiter(spotify.LimiterConfig{Rate: 100, Burst: 10, MinBackoff: 10 * time.Millisecond,
		MaxBackoff: 35 * time.Millisecond, QuotaBackoff: time.Hour}, nil)
	// Without Retry-After: exponential, capped.
	for i, want := range []time.Duration{10, 20, 35, 35} {
		if got := l.Throttled(0, false, ""); got != want*time.Millisecond {
			t.Fatalf("strike %d: backoff %s", i, got)
		}
	}
	l.Success() // resets strikes
	if got := l.Throttled(0, false, ""); got != 10*time.Millisecond {
		t.Fatalf("after success: %s", got)
	}
	// Retry-After wins; the whole budget, live lane included, waits.
	start := time.Now()
	l.Throttled(150*time.Millisecond, false, "")
	st := l.Stats()
	if st.BlockedUntil.IsZero() || st.Throttled != 6 || st.EffectiveRatePerSec >= st.RatePerSec {
		t.Fatalf("stats after 429: %+v", st)
	}
	if err := waitIn(l, spotify.PriorityLive, 50*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("live lane during backoff: %v", err)
	}
	if err := waitIn(l, spotify.PriorityLive, time.Second); err != nil || time.Since(start) < 150*time.Millisecond {
		t.Fatalf("after backoff: %v after %s", err, time.Since(start))
	}
	// The rate recovers with successes.
	for range 1000 {
		l.Success()
	}
	if st := l.Stats(); st.EffectiveRatePerSec != st.RatePerSec {
		t.Fatalf("rate did not recover: %+v", st)
	}
	// QUOTA_EXCEEDED backs off for at least QuotaBackoff.
	if got := l.Throttled(time.Second, true, "QUOTA_EXCEEDED"); got != time.Hour {
		t.Fatalf("quota backoff %s", got)
	}
	if st := l.Stats(); st.QuotaExceeded != 1 || st.LastReason != "QUOTA_EXCEEDED" {
		t.Fatalf("quota stats %+v", st)
	}
}

func TestClientHonoursRetryAfterAndQuota(t *testing.T) {
	e := newEnv(t)
	lim := spotify.NewLimiter(spotify.LimiterConfig{Rate: 100, Burst: 10, QuotaBackoff: 300 * time.Millisecond}, nil)
	c := spotify.New(spotify.Options{DB: e.db, Keyring: e.keys, AccountsURL: e.fake.AccountsURL(),
		APIURL: e.fake.APIURL(), Now: e.clock, Limiter: lim})
	e.connect(c)
	ctx := context.Background()

	e.fake.FailNext("/v1/me/player", spotifytest.Failure{Status: 429, RetryAfter: "1"})
	_, _, err := c.Player(ctx)
	var rl *spotify.RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter != time.Second || rl.QuotaExceeded {
		t.Fatalf("429: %v", err)
	}
	start := time.Now()
	if _, _, err := c.Player(ctx); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 900*time.Millisecond {
		t.Fatalf("next request went out after %s, before Retry-After", waited)
	}
	if n := e.fake.Hits("/v1/me/player"); n != 2 {
		t.Fatalf("%d requests reached Spotify", n)
	}

	// QUOTA_EXCEEDED pauses only its bucket: catalog lookups stop (and
	// fail without being sent), live capture carries on.
	e.fake.FailNext("/v1/tracks/abc", spotifytest.Failure{Status: 429, RetryAfter: "67000",
		Body: `{"error":{"status":429,"message":"Quota exceeded","reason":"QUOTA_EXCEEDED"}}`})
	if _, err := c.Track(ctx, "abc"); !errors.As(err, &rl) || !rl.QuotaExceeded || rl.Bucket != spotify.BucketCatalog ||
		rl.Backoff != 67000*time.Second || rl.NotSent {
		t.Fatalf("quota: %v %+v", err, rl)
	}
	if _, err := c.Artist(ctx, "xyz"); !errors.As(err, &rl) || !rl.NotSent || !rl.QuotaExceeded {
		t.Fatalf("paused bucket: %v", err)
	}
	if n := e.fake.HitsPrefix("/v1/artists/"); n != 0 {
		t.Fatalf("paused bucket sent %d requests", n)
	}
	start = time.Now()
	if _, _, err := c.Player(ctx); err != nil || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("player during a catalog quota pause: %v after %s", err, time.Since(start))
	}
	st := c.LimiterStats()
	if st.Throttled != 2 || st.QuotaExceeded != 1 || st.QuotaBlocked[spotify.BucketCatalog].IsZero() ||
		!st.QuotaBlocked[spotify.BucketUser].IsZero() {
		t.Fatalf("stats %+v", st)
	}
}

// Playlists are outside Spotify's catalog quota (D24): a spent track and
// album quota leaves playlist lookups working, and a spent playlist
// quota pauses only playlists.
func TestPlaylistQuotaBucket(t *testing.T) {
	e := newEnv(t)
	lim := spotify.NewLimiter(spotify.LimiterConfig{Rate: 100, Burst: 10, QuotaBackoff: time.Millisecond}, nil)
	c := spotify.New(spotify.Options{DB: e.db, Keyring: e.keys, AccountsURL: e.fake.AccountsURL(),
		APIURL: e.fake.APIURL(), Now: e.clock, Limiter: lim})
	e.connect(c)
	ctx := context.Background()
	e.fake.SetPlaylist(spotifytest.CatalogPlaylist{ID: "mine", Name: "Road Trip", OwnerID: "listener42"})
	e.fake.SetQuotaExceeded("/v1/tracks/", "67000")
	e.fake.SetQuotaExceeded("/v1/albums/", "67000")

	var rl *spotify.RateLimitedError
	if _, err := c.Track(ctx, "abc"); !errors.As(err, &rl) || !rl.QuotaExceeded || rl.Bucket != spotify.BucketCatalog || rl.NotSent {
		t.Fatalf("track quota: %v %+v", err, rl)
	}
	if _, err := c.Album(ctx, "alb"); !errors.As(err, &rl) || !rl.NotSent || rl.Bucket != spotify.BucketCatalog {
		t.Fatalf("album during the catalog pause: %v", err)
	}
	for range 3 {
		if p, err := c.Playlist(ctx, "mine"); err != nil || p.Name != "Road Trip" {
			t.Fatalf("playlist during the catalog pause: %+v %v", p, err)
		}
	}
	if n := e.fake.Hits("/v1/playlists/mine"); n != 3 {
		t.Fatalf("%d playlist requests reached Spotify, want 3", n)
	}
	if n := e.fake.HitsPrefix("/v1/albums/"); n != 0 {
		t.Fatalf("paused catalog bucket sent %d album requests", n)
	}
	st := c.LimiterStats()
	if st.QuotaBlocked[spotify.BucketCatalog].IsZero() || !st.QuotaBlocked[spotify.BucketPlaylists].IsZero() {
		t.Fatalf("after the catalog quota: %+v", st.QuotaBlocked)
	}

	// A spent playlist quota pauses playlists, and only them.
	c2 := e.client()
	e.fake.SetQuotaExceeded("/v1/tracks/", "")
	e.fake.SetQuotaExceeded("/v1/albums/", "")
	e.fake.SetQuotaExceeded("/v1/playlists/", "3600")
	if _, err := c2.Playlist(ctx, "mine"); !errors.As(err, &rl) || !rl.QuotaExceeded || rl.Bucket != spotify.BucketPlaylists {
		t.Fatalf("playlist quota: %v %+v", err, rl)
	}
	before := e.fake.Hits("/v1/playlists/mine")
	if _, err := c2.Playlist(ctx, "mine"); !errors.As(err, &rl) || !rl.NotSent {
		t.Fatalf("paused playlists: %v", err)
	}
	if n := e.fake.Hits("/v1/playlists/mine"); n != before {
		t.Fatalf("paused playlists bucket sent a request")
	}
	if tr, err := c2.Track(ctx, "abc"); err != nil || tr.ID != "abc" {
		t.Fatalf("track during the playlist pause: %+v %v", tr, err)
	}
	if st := c2.LimiterStats(); st.QuotaBlocked[spotify.BucketPlaylists].IsZero() || !st.QuotaBlocked[spotify.BucketCatalog].IsZero() {
		t.Fatalf("after the playlist quota: %+v", st.QuotaBlocked)
	}
}

func TestBucket(t *testing.T) {
	for path, want := range map[string]string{"/tracks/x": "catalog", "/artists/x": "catalog", "/albums/x": "catalog",
		"/playlists/x?fields=id": "playlists", "/me/player": "user", "/me": "user", "/me/playlists": "user"} {
		if got := spotify.Bucket(path); got != want {
			t.Errorf("Bucket(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestPlayerEndpoint(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	e.connect(c)
	ctx := context.Background()
	st, raw, err := c.Player(ctx)
	if st != nil || raw != nil || err != nil {
		t.Fatalf("204: %v %v %v", st, raw, err)
	}
	q := e.fake.Seen["player"]
	if q.Get("market") != "from_token" || q.Has("additional_types") {
		t.Fatalf("query %v: must use market=from_token and never request episodes (D11)", q)
	}
	e.fake.SetPlayer(&spotifytest.PlayerSpec{TrackID: "t1", DurationMs: 200_000, ProgressMs: 5_000, Playing: true,
		Shuffle: true, Repeat: "context", ContextURI: "spotify:playlist:p1", DeviceName: "Kitchen", DeviceType: "Speaker"})
	st, raw, err = c.Player(ctx)
	if err != nil || st.Item == nil || st.Item.ID != "t1" || st.Item.DurationMs != 200_000 || *st.ProgressMs != 5_000 ||
		!st.IsPlaying || !st.ShuffleState || st.RepeatState != "context" || st.Context.URI != "spotify:playlist:p1" ||
		st.Device.Name != "Kitchen" || st.CurrentlyPlayingType != "track" || len(raw) == 0 {
		t.Fatalf("player: %+v %v", st, err)
	}
	e.fake.SetPlayer(&spotifytest.PlayerSpec{Type: "episode", ProgressMs: 1000, Playing: true})
	if st, _, _ = c.Player(ctx); st.Item != nil || st.CurrentlyPlayingType != "episode" {
		t.Fatalf("episode: %+v", st)
	}
	// Expired access token: refreshed transparently.
	e.fake.ExpireAccessTokens()
	if _, _, err := c.Player(ctx); err != nil {
		t.Fatalf("after expiry: %v", err)
	}
	// Revoked refresh token: reconnect required.
	e.fake.ExpireAccessTokens()
	e.fake.RevokeRefreshTokens()
	if _, _, err := c.Player(ctx); !errors.Is(err, spotify.ErrReconnectRequired) {
		t.Fatalf("revoked: %v", err)
	}
	// Other errors are APIErrors.
	e.connect(c)
	e.fake.FailNext("/v1/me/player", spotifytest.Failure{Status: 503})
	var ae *spotify.APIError
	if _, _, err := c.Player(ctx); !errors.As(err, &ae) || ae.Status != 503 {
		t.Fatalf("503: %v", err)
	}
}

func TestRecentlyPlayedEndpoint(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	e.connect(c)
	ctx := context.Background()
	base := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	for i := range 5 {
		e.fake.AddRecent(spotifytest.RecentPlay{TrackID: "r" + string(rune('a'+i)), DurationMs: 180_000,
			PlayedAt: base.Add(time.Duration(i) * 4 * time.Minute), ContextURI: "spotify:album:x"})
	}
	page, err := c.RecentlyPlayed(ctx, 0, 50)
	if err != nil || len(page.Items) != 5 || page.Items[0].Track.ID != "re" || page.Items[0].Context.URI != "spotify:album:x" ||
		len(page.Items[0].Raw) == 0 {
		t.Fatalf("page: %+v %v", page, err)
	}
	page, err = c.RecentlyPlayed(ctx, base.Add(5*time.Minute).UnixMilli(), 2)
	// Plays at +8, +12, +16 min are after the cursor; the oldest two come
	// back, newest first.
	if err != nil || len(page.Items) != 2 || page.Items[0].Track.ID != "rd" || page.Items[1].Track.ID != "rc" {
		t.Fatalf("after cursor: %+v %v", page, err)
	}
	if q := e.fake.Seen["recently-played"]; q.Get("limit") != "2" || q.Get("after") == "" {
		t.Fatalf("query %v", q)
	}
}
