package hydrate_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/hydrate"
	"spotussi/internal/keyring"
	"spotussi/internal/spotify"
	"spotussi/internal/spotify/spotifytest"
)

const clientID = "0123456789abcdef0123456789abcdef"

type env struct {
	t       *testing.T
	db      *db.DB
	fake    *spotifytest.Fake
	client  *spotify.Client
	limiter *spotify.Limiter
	mu      sync.Mutex
	now     time.Time
	changes atomic.Int32
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
	l := spotify.NewLimiter(spotify.LimiterConfig{Rate: 2000, Burst: 100, MinBackoff: 20 * time.Millisecond,
		QuotaBackoff: time.Millisecond}, nil)
	c := spotify.New(spotify.Options{DB: d, Keyring: k, AccountsURL: f.AccountsURL(), APIURL: f.APIURL(), Limiter: l})
	e := &env{t: t, db: d, fake: f, client: c, limiter: l, now: time.Now()}
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
func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

func (e *env) hydrator(o hydrate.Options) *hydrate.Hydrator {
	o.DB, o.Spotify, o.Now = e.db, e.client, e.clock
	if o.OnCatalogChanged == nil {
		o.OnCatalogChanged = func() { e.changes.Add(1) }
	}
	o.LimiterStats = e.client.LimiterStats
	return hydrate.New(o)
}

// run starts h and returns a function stopping it (and waiting for it).
func (e *env) run(h *hydrate.Hydrator) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.Run(ctx); close(done) }()
	var once sync.Once
	stop = func() { once.Do(func() { cancel(); <-done }) }
	e.t.Cleanup(stop)
	return stop
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Write.Exec(q, args...); err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
}

func (e *env) int(q string, args ...any) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.Read.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	return n
}

func (e *env) str(q string, args ...any) string {
	e.t.Helper()
	var s sql.NullString
	if err := e.db.Read.QueryRow(q, args...).Scan(&s); err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	return s.String
}

// waitFor polls cond until it holds (or fails the test after 10 s).
func (e *env) waitFor(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			e.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

var exportSeq atomic.Int64

// exportRows inserts plays export rows for uri (ms_played each).
func (e *env) exportRows(uri string, plays int, msPlayed int64) {
	e.t.Helper()
	e.exec(`INSERT INTO imports (id, filename, origin, kind, state, created_at) VALUES (1, 'x.zip', 'cli', 'zip', 'done', 0)
		ON CONFLICT DO NOTHING`)
	for range plays {
		n := exportSeq.Add(1)
		h := sha256.Sum256([]byte(fmt.Sprint(n)))
		ct := "track"
		if strings.HasPrefix(uri, "spotify:episode:") {
			ct = "episode"
		}
		e.exec(`INSERT INTO raw_export_rows (import_id, row_hash, ts_ms, ms_played, content_type, uri, json)
			VALUES (1, ?, ?, ?, ?, ?, '{}')`, h[:], n*1000, msPlayed, ct, uri)
	}
}

var pollerSeq atomic.Int64

// pollerPlay stores a poller row whose JSON is the player response p.
func (e *env) pollerPlay(raw []byte, contextURI string) {
	e.t.Helper()
	n := pollerSeq.Add(1)
	var ctxURI any
	if contextURI != "" {
		ctxURI = contextURI
	}
	e.exec(`INSERT INTO raw_poller_plays (track_id, track_uri, duration_ms, started_ms, ended_ms, ms_played, skipped,
		end_reason, context_uri, observations, first_seen_ms, last_seen_ms, json, created_at)
		VALUES (?, ?, 200000, ?, ?, 200000, 0, 'track_change', ?, 3, 0, 0, ?, 0)`,
		fmt.Sprint("t", n), "spotify:track:x", n*1000, n*1000+200000, ctxURI, string(raw))
}

func TestSeedFromLiveCapture(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.exportRows("spotify:track:live1", 3, 60000)
	e.pollerPlay(spotifytest.PlayerSpec{TrackID: "live1", TrackName: "Live One", DurationMs: 180000, Playing: true,
		ContextURI: "spotify:playlist:pl1"}.JSON(), "spotify:playlist:pl1")
	// A relinked track (linked_from) and a local file.
	relinked := `{"is_playing":true,"currently_playing_type":"track","item":{"id":"newid","uri":"spotify:track:newid",
		"name":"Relinked","duration_ms":1000,"type":"track","is_local":false,"linked_from":{"id":"oldid","uri":"spotify:track:oldid"},
		"artists":[{"id":"art9","name":"Nine"}],"album":{"id":"alb9","name":"Album Nine","images":[]}}}`
	e.pollerPlay([]byte(relinked), "spotify:user:me:collection")
	local := `{"is_playing":true,"currently_playing_type":"track","item":{"id":null,"uri":"spotify:local:A:B:Song:200",
		"name":"Song","duration_ms":200000,"type":"track","is_local":true,"artists":[{"id":null,"name":"A"}],"album":{"id":null,"name":"B"}}}`
	e.pollerPlay([]byte(local), "")
	e.pollerPlay([]byte("null"), "") // no snapshot kept
	recent, _ := json.Marshal(map[string]any{"played_at": "2026-10-17T10:00:00Z", "context": map[string]any{"uri": "spotify:album:album-rec1"},
		"track": map[string]any{"id": "rec1", "uri": "spotify:track:rec1", "name": "Recent", "duration_ms": 1, "type": "track",
			"artists": []any{map[string]any{"id": "art1", "name": "One"}}, "album": map[string]any{"id": "album-rec1", "name": "Rec Album",
				"release_date": "2001", "release_date_precision": "year", "total_tracks": 9, "album_type": "single",
				"images":  []any{map[string]any{"url": "big", "width": 640, "height": 640}, map[string]any{"url": "tiny", "width": 32, "height": 32}, map[string]any{"url": "small", "width": 64, "height": 64}},
				"artists": []any{map[string]any{"id": "art1", "name": "One"}}}}})
	e.exec(`INSERT INTO raw_recent_plays (played_at_ms, track_id, duration_ms, context_uri, json, fetched_at) VALUES (1, 'rec1', 1, ?, ?, 5)`,
		"spotify:album:album-rec1", string(recent))

	h := e.hydrator(hydrate.Options{})
	n, err := h.Seed(ctx)
	if err != nil || n != 4 {
		t.Fatalf("seed: %d, %v; want 4 tracks", n, err)
	}
	if got := e.int(`SELECT COUNT(*) FROM tracks WHERE hydrated_at IS NOT NULL`); got != 4 {
		t.Fatalf("hydrated tracks = %d, want 4", got)
	}
	if s := e.str(`SELECT source || '/' || name || '/' || album_id FROM tracks WHERE id = 'live1'`); s != "poller/Live One/album-live1" {
		t.Errorf("live1 = %q", s)
	}
	if s := e.str(`SELECT linked_from FROM tracks WHERE id = 'newid'`); s != "oldid" {
		t.Errorf("linked_from = %q, want oldid", s)
	}
	if n := e.int(`SELECT is_local FROM tracks WHERE id = 'spotify:local:A:B:Song:200'`); n != 1 {
		t.Errorf("local file not stored as local")
	}
	if n := e.int(`SELECT COUNT(*) FROM track_artists WHERE track_id LIKE 'spotify:local:%'`); n != 0 {
		t.Errorf("local file got %d artist credits", n)
	}
	if s := e.str(`SELECT image_url || ' ' || image_url_small || ' ' || release_date_precision || ' ' || total_tracks || ' ' || album_type
		FROM albums WHERE id = 'album-rec1'`); s != "big small year 9 single" {
		t.Errorf("album = %q", s)
	}
	// Artists are known by name and queued; live1's artist at its export
	// plays + 1.
	if p := e.int(`SELECT priority FROM hydration_queue WHERE entity_type = 'artist' AND id = 'artist-live1'`); p != 4 {
		t.Errorf("artist priority = %d, want 4", p)
	}
	if n := e.int(`SELECT COUNT(*) FROM artists WHERE hydrated_at IS NULL`); n != 3 {
		t.Errorf("unhydrated artists = %d, want 3", n)
	}
	// Contexts: the playlist and the album are queued; Liked Songs is
	// resolved without a request.
	if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE entity_type = 'context'`); n != 2 {
		t.Errorf("queued contexts = %d, want 2", n)
	}
	if s := e.str(`SELECT type || ' ' || name_source || ' ' || (resolved_at IS NOT NULL) FROM contexts WHERE uri = 'spotify:user:me:collection'`); s != "collection unresolved 1" {
		t.Errorf("collection = %q", s)
	}
	// Watermarks: nothing is seeded twice.
	if n, err := h.Seed(ctx); err != nil || n != 0 {
		t.Fatalf("second seed: %d, %v", n, err)
	}
	e.pollerPlay(spotifytest.PlayerSpec{TrackID: "live1", Playing: true, ContextURI: "spotify:playlist:pl1"}.JSON(), "spotify:playlist:pl1")
	if n, _ := h.Seed(ctx); n != 1 {
		t.Fatalf("third seed: %d, want 1", n)
	}
	if p := e.int(`SELECT priority FROM hydration_queue WHERE entity_type = 'context' AND id = 'spotify:playlist:pl1'`); p != 2 {
		t.Errorf("context priority = %d, want 2 (plays from it)", p)
	}
}

func TestPlanQueuesExportTracksByPlays(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.exportRows("spotify:track:aaa", 5, 60000)
	e.exportRows("spotify:track:aaa", 4, 1000) // skips: not plays
	e.exportRows("spotify:track:bbb", 2, 30000)
	e.exportRows("spotify:track:ccc", 1, 29999)
	e.exportRows("spotify:episode:pod", 3, 60000)
	e.exportRows("spotify:local:x:y:z:1", 2, 60000)
	e.exec(`INSERT INTO tracks (id, name, source, hydrated_at) VALUES ('ddd', 'Known', 'poller', 1)`)
	e.exportRows("spotify:track:ddd", 7, 60000)

	h := e.hydrator(hydrate.Options{})
	if n, err := h.Plan(ctx); err != nil || n != 3 {
		t.Fatalf("plan: %d, %v; want 3", n, err)
	}
	got := map[string]int64{}
	rs, _ := e.db.Read.Query(`SELECT id, priority FROM hydration_queue WHERE entity_type = 'track'`)
	for rs.Next() {
		var id string
		var p int64
		rs.Scan(&id, &p)
		got[id] = p
	}
	rs.Close()
	if fmt.Sprint(got) != "map[aaa:5 bbb:2 ccc:0]" {
		t.Errorf("queue = %v", got)
	}
	if n, _ := h.Plan(ctx); n != 0 {
		t.Errorf("re-plan queued %d", n)
	}
	// A later import reprioritises.
	e.exportRows("spotify:track:ccc", 9, 60000)
	if n, _ := h.Plan(ctx); n != 1 {
		t.Errorf("plan after import: %d, want 1", n)
	}
	if p := e.int(`SELECT priority FROM hydration_queue WHERE id = 'ccc'`); p != 9 {
		t.Errorf("ccc priority = %d, want 9", p)
	}
	st, err := h.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.TracksTotal != 4 || st.TracksHydrated != 1 || st.Queue["track"] != 3 {
		t.Errorf("status = %+v", st)
	}
}

func TestRunHydratesMostPlayedFirst(t *testing.T) {
	e := newEnv(t)
	e.fake.SetArtistsOf(func(id string) []string { return []string{"art-" + id, "shared"} })
	for i, id := range []string{"t1", "t2", "t3", "t4"} {
		e.exportRows("spotify:track:"+id, 10-i*3, 60000) // t1 10, t2 7, t3 4, t4 1
	}
	h := e.hydrator(hydrate.Options{Workers: 1, BatchSize: 1})
	e.run(h)
	e.waitFor("queue empty", func() bool { return e.int(`SELECT COUNT(*) FROM hydration_queue`) == 0 && e.changes.Load() > 0 })

	log := e.fake.CatalogLog()
	// Every track first, most-played first; then artists by priority (D24).
	want := []string{"/v1/tracks/t1", "/v1/tracks/t2", "/v1/tracks/t3", "/v1/tracks/t4",
		"/v1/artists/art-t1", "/v1/artists/shared", "/v1/artists/art-t2", "/v1/artists/art-t3", "/v1/artists/art-t4"}
	if fmt.Sprint(log) != fmt.Sprint(want) {
		t.Errorf("requests:\n got %v\nwant %v", log, want)
	}
	if n := e.int(`SELECT COUNT(*) FROM tracks WHERE hydrated_at IS NOT NULL AND source = 'api' AND isrc IS NOT NULL`); n != 4 {
		t.Errorf("hydrated tracks = %d", n)
	}
	if n := e.int(`SELECT COUNT(*) FROM artists WHERE hydrated_at IS NOT NULL AND image_url IS NOT NULL`); n != 5 {
		t.Errorf("hydrated artists = %d, want 5", n)
	}
	if s := e.str(`SELECT group_concat(artist_id, ',') FROM (SELECT artist_id FROM track_artists WHERE track_id = 't2' ORDER BY position)`); s != "art-t2,shared" {
		t.Errorf("credits = %q", s)
	}
	if n := e.int(`SELECT COUNT(*) FROM artist_genres WHERE artist_id = 'shared'`); n != 2 {
		t.Errorf("genres = %d", n)
	}
	if n := e.int(`SELECT COUNT(*) FROM albums WHERE hydrated_at IS NOT NULL`); n != 4 {
		t.Errorf("albums = %d, want 4 (from track objects, no album calls)", n)
	}
	st, _ := h.Status(context.Background())
	if st.State != hydrate.StateIdle || st.QueueLength() != 0 || st.ArtistsHydrated != 5 || !st.ETA.IsZero() {
		t.Errorf("status = %+v", st)
	}
	if s := e.limiter.Stats(); s.Requests["background"] != 9 || s.Requests["live"] != 0 {
		t.Errorf("limiter lanes = %v", s.Requests)
	}
}

func TestFailuresBackOffAndPark(t *testing.T) {
	e := newEnv(t)
	e.exportRows("spotify:track:gone", 3, 60000)
	e.exportRows("spotify:track:flaky", 2, 60000)
	e.exportRows("spotify:track:bad%20id", 1, 60000)
	e.fake.SetMissing("/v1/tracks/gone")
	e.fake.FailNext("/v1/tracks/flaky", spotifytest.Failure{Status: 502})
	h := e.hydrator(hydrate.Options{Workers: 1})
	e.run(h)
	e.waitFor("failures recorded", func() bool {
		return e.int(`SELECT COUNT(*) FROM hydration_queue WHERE attempts > 0`) == 3
	})
	if s := e.str(`SELECT last_error FROM hydration_queue WHERE id = 'gone'`); !strings.HasPrefix(s, "permanent: ") {
		t.Errorf("gone error = %q", s)
	}
	if s := e.str(`SELECT last_error FROM hydration_queue WHERE id = 'bad%20id'`); !strings.HasPrefix(s, "permanent: ") {
		t.Errorf("invalid ID error = %q", s)
	}
	parked := e.int(`SELECT next_attempt_at FROM hydration_queue WHERE id = 'gone'`) - e.clock().UnixMilli()
	retry := e.int(`SELECT next_attempt_at FROM hydration_queue WHERE id = 'flaky'`) - e.clock().UnixMilli()
	if parked < int64(29*24*time.Hour/time.Millisecond) || retry < 50_000 || retry > 70_000 {
		t.Errorf("parked for %d ms, retry in %d ms", parked, retry)
	}
	st, _ := h.Status(context.Background())
	if st.Failed != 2 || st.Queue["track"] != 1 || st.LastError == "" {
		t.Errorf("status = %+v", st)
	}
	// A minute later the flaky track is retried and hydrated.
	e.advance(2 * time.Minute)
	h.Wake()
	e.waitFor("flaky hydrated", func() bool { return e.int(`SELECT COUNT(*) FROM tracks WHERE id = 'flaky'`) == 1 })
	if e.fake.Hits("/v1/tracks/gone") != 1 {
		t.Errorf("parked track requested again")
	}
}

func TestRateLimitRetriesWithoutAttempt(t *testing.T) {
	e := newEnv(t)
	e.exportRows("spotify:track:slow", 1, 60000)
	e.fake.FailNext("/v1/tracks/slow", spotifytest.Failure{Status: 429, RetryAfter: "1"})
	h := e.hydrator(hydrate.Options{})
	start := time.Now()
	e.run(h)
	e.waitFor("hydrated", func() bool { return e.int(`SELECT COUNT(*) FROM tracks WHERE id = 'slow'`) == 1 })
	if d := time.Since(start); d < 900*time.Millisecond {
		t.Errorf("retried after %s, before Retry-After", d)
	}
	if s := e.limiter.Stats(); s.Throttled != 1 {
		t.Errorf("throttled = %d", s.Throttled)
	}
	if e.fake.Hits("/v1/tracks/slow") != 2 {
		t.Errorf("hits = %d", e.fake.Hits("/v1/tracks/slow"))
	}
}

func TestContexts(t *testing.T) {
	e := newEnv(t)
	e.fake.SetPlaylist(spotifytest.CatalogPlaylist{ID: "mine", Name: "Road Trip", OwnerID: "listener42"})
	plays := []string{"spotify:playlist:mine", "spotify:playlist:deleted", "spotify:playlist:37i9dQZF1E4Daily",
		"spotify:album:album-t1", "spotify:artist:art7", "spotify:show:pod", "spotify:playlist:named"}
	for _, uri := range plays {
		e.pollerPlay(spotifytest.PlayerSpec{TrackID: "t1", Playing: true, ContextURI: uri}.JSON(), uri)
	}
	// The owner named this one by hand (D17).
	e.exec(`INSERT INTO contexts (uri, type, name, name_source) VALUES ('spotify:playlist:named', 'playlist', 'Mine', 'manual')`)
	e.fake.SetPlaylist(spotifytest.CatalogPlaylist{ID: "named", Name: "Spotify Name", OwnerID: "x"})

	h := e.hydrator(hydrate.Options{Workers: 1})
	e.run(h)
	e.waitFor("contexts resolved", func() bool {
		return e.int(`SELECT COUNT(*) FROM contexts WHERE resolved_at IS NULL`) == 0 && e.int(`SELECT COUNT(*) FROM hydration_queue`) == 0
	})
	got := map[string]string{}
	rs, _ := e.db.Read.Query(`SELECT uri, type, COALESCE(name, '-'), name_source, COALESCE(owner_id, '-') FROM contexts`)
	for rs.Next() {
		var uri, typ, name, src, owner string
		rs.Scan(&uri, &typ, &name, &src, &owner)
		got[uri] = typ + "|" + name + "|" + src + "|" + owner
	}
	rs.Close()
	want := map[string]string{
		"spotify:playlist:mine":             "playlist|Road Trip|api|listener42",
		"spotify:playlist:deleted":          "playlist|-|unresolved|-",
		"spotify:playlist:37i9dQZF1E4Daily": "playlist|-|unresolved|-",
		"spotify:album:album-t1":            "album|Album of Track t1|api|-",
		"spotify:artist:art7":               "artist|Name of art7|api|-",
		"spotify:show:pod":                  "show|-|unresolved|-",
		"spotify:playlist:named":            "playlist|Mine|manual|-",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	// The album came with the seeded track (no album call); the artist
	// context hydrated the artist; Spotify-made playlists are not asked.
	if n := e.fake.HitsPrefix("/v1/albums/"); n != 0 {
		t.Errorf("album requests = %d", n)
	}
	if n := e.int(`SELECT COUNT(*) FROM artists WHERE id = 'art7' AND hydrated_at IS NOT NULL`); n != 1 {
		t.Errorf("artist context did not hydrate the artist")
	}
	if n := e.fake.Hits("/v1/playlists/37i9dQZF1E4Daily"); n != 0 {
		t.Errorf("Spotify-made playlist requested")
	}
}

func TestWaitsForSpotifyAndResumes(t *testing.T) {
	e := newEnv(t)
	for i := range 20 {
		e.exportRows(fmt.Sprintf("spotify:track:w%02d", i), 20-i, 60000)
	}
	if err := e.client.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := e.hydrator(hydrate.Options{})
	stop := e.run(h)
	e.waitFor("waiting", func() bool { st, _ := h.Status(context.Background()); return st.State == hydrate.StateWaiting })
	if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE attempts > 0`); n != 0 {
		t.Errorf("%d entries charged an attempt while disconnected", n)
	}
	stop()

	// A restart with Spotify connected resumes the stored queue.
	e.connect()
	h2 := e.hydrator(hydrate.Options{})
	e.client.OnConnect(h2.Wake)
	e.run(h2)
	e.waitFor("done", func() bool { return e.int(`SELECT COUNT(*) FROM tracks WHERE hydrated_at IS NOT NULL`) == 20 })
}

func TestQuotaPausesUntilReset(t *testing.T) {
	e := newEnv(t)
	e.fake.SetArtistsOf(func(string) []string { return []string{"same"} })
	for i := range 10 {
		e.exportRows(fmt.Sprintf("spotify:track:q%d", i), 20-i, 60000)
	}
	// The 6th lookup (q5, after q0 to q4; artists wait for every track)
	// finds the daily quota spent, with Retry-After until the reset.
	e.fake.FailNext("/v1/tracks/q5", spotifytest.Failure{Status: 429, RetryAfter: "2",
		Body: `{"error":{"status":429,"message":"Quota exceeded","reason":"QUOTA_EXCEEDED"}}`})
	h := e.hydrator(hydrate.Options{Workers: 1, BatchSize: 1})
	e.run(h)
	e.waitFor("quota state", func() bool { st, _ := h.Status(context.Background()); return st.State == hydrate.StateQuota })
	st, _ := h.Status(context.Background())
	if st.QuotaLookups != 5 || st.ResumesAt.IsZero() || st.TracksHydrated != 5 {
		t.Fatalf("status = %+v", st)
	}
	// 6 left at 5 a day: one more whole day after the reset.
	if d := st.ETA.Sub(st.ResumesAt); d < 24*time.Hour || d > 25*time.Hour {
		t.Errorf("ETA %s after the reset", d)
	}
	if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE attempts > 0`); n != 0 {
		t.Errorf("%d entries charged an attempt for the quota", n)
	}
	if n := e.int(`SELECT value FROM settings WHERE key = 'hydrate_quota_lookups'`); n != 5 {
		t.Errorf("stored quota = %d", n)
	}
	// After the reset it carries on. The limiter runs on the wall clock,
	// the hydrator on the test's: let both pass the reset.
	e.waitFor("limiter reset", func() bool { return e.client.LimiterStats().QuotaBlocked[spotify.BucketCatalog].IsZero() })
	e.advance(3 * time.Second)
	h.Wake()
	e.waitFor("all hydrated", func() bool { return e.int(`SELECT COUNT(*) FROM tracks WHERE hydrated_at IS NOT NULL`) == 10 })
	if n := e.fake.Hits("/v1/tracks/q5"); n != 2 {
		t.Errorf("q5 requested %d times, want 2 (no requests while paused)", n)
	}
}

// Playlists are outside Spotify's catalog quota (D24): with the track and
// album quota spent, playlist contexts keep resolving while catalog
// entries wait for the reset.
func TestTrackQuotaKeepsResolvingPlaylists(t *testing.T) {
	e := newEnv(t)
	e.fake.SetPlaylist(spotifytest.CatalogPlaylist{ID: "mine", Name: "Road Trip", OwnerID: "listener42"})
	e.fake.SetPlaylist(spotifytest.CatalogPlaylist{ID: "gym", Name: "Gym", OwnerID: "listener42"})
	for _, p := range []string{"/v1/tracks/", "/v1/albums/", "/v1/artists/"} {
		e.fake.SetQuotaExceeded(p, "3600")
	}
	for i := range 5 {
		e.exportRows(fmt.Sprintf("spotify:track:k%d", i), 20-i, 60000) // ahead of the contexts
	}
	for _, uri := range []string{"spotify:playlist:mine", "spotify:user:listener42:playlist:gym", "spotify:album:ctx-album"} {
		e.pollerPlay(spotifytest.PlayerSpec{TrackID: "t1", Playing: true, ContextURI: uri}.JSON(), uri)
	}

	h := e.hydrator(hydrate.Options{Workers: 1, BatchSize: 2})
	e.run(h)
	e.waitFor("playlists resolved", func() bool {
		return e.int(`SELECT COUNT(*) FROM contexts WHERE type = 'playlist' AND name_source = 'api'`) == 2
	})
	e.waitFor("quota state", func() bool { st, _ := h.Status(context.Background()); return st.State == hydrate.StateQuota })
	if s := e.str(`SELECT name || ' ' || owner_id FROM contexts WHERE uri = 'spotify:playlist:mine'`); s != "Road Trip listener42" {
		t.Errorf("mine = %q", s)
	}
	if s := e.str(`SELECT name FROM contexts WHERE uri = 'spotify:user:listener42:playlist:gym'`); s != "Gym" {
		t.Errorf("legacy playlist URI = %q", s)
	}
	// Catalog entries (tracks and the album context) wait, uncharged; one
	// request found the quota spent, the rest weren't sent.
	if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE entity_type = 'track'`); n != 5 {
		t.Errorf("queued tracks = %d, want 5", n)
	}
	if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE id = 'spotify:album:ctx-album'`); n != 1 {
		t.Errorf("album context left the queue during the catalog pause")
	}
	if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE attempts > 0`); n != 0 {
		t.Errorf("%d entries charged an attempt for the quota", n)
	}
	if n := e.fake.HitsPrefix("/v1/tracks/") + e.fake.HitsPrefix("/v1/albums/"); n != 1 {
		t.Errorf("%d catalog requests sent, want 1", n)
	}
	st, _ := h.Status(context.Background())
	if st.ResumesAt.IsZero() || st.QuotaLookups != 0 {
		t.Errorf("status = %+v (playlist lookups must not count as catalog quota)", st)
	}
}

// A spent playlist quota pauses only playlist contexts: tracks carry on.
func TestPlaylistQuotaPausesOnlyPlaylists(t *testing.T) {
	e := newEnv(t)
	e.fake.SetPlaylist(spotifytest.CatalogPlaylist{ID: "mine", Name: "Road Trip", OwnerID: "listener42"})
	e.fake.SetQuotaExceeded("/v1/playlists/", "3600")
	for range 30 {
		e.pollerPlay(spotifytest.PlayerSpec{TrackID: "t1", Playing: true, ContextURI: "spotify:playlist:mine"}.JSON(),
			"spotify:playlist:mine") // ahead of the tracks
	}
	for i := range 5 {
		e.exportRows(fmt.Sprintf("spotify:track:p%d", i), 5-i, 60000)
	}

	h := e.hydrator(hydrate.Options{Workers: 1, BatchSize: 2})
	e.run(h)
	e.waitFor("tracks hydrated", func() bool {
		return e.int(`SELECT COUNT(*) FROM tracks WHERE id LIKE 'p%' AND hydrated_at IS NOT NULL`) == 5
	})
	e.waitFor("idle", func() bool { st, _ := h.Status(context.Background()); return st.State == hydrate.StateIdle })
	if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE id = 'spotify:playlist:mine' AND attempts = 0`); n != 1 {
		t.Errorf("playlist context not waiting uncharged in the queue")
	}
	if n := e.fake.Hits("/v1/playlists/mine"); n != 1 {
		t.Errorf("playlist requested %d times, want 1 (no requests while paused)", n)
	}
	if st, _ := h.Status(context.Background()); !st.ResumesAt.IsZero() {
		t.Errorf("a playlist quota reported as the catalog quota: %+v", st)
	}
}
