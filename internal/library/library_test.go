package library_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/keyring"
	"spotussi/internal/library"
	"spotussi/internal/spotify"
	"spotussi/internal/spotify/spotifytest"
)

const clientID = "0123456789abcdef0123456789abcdef"

type env struct {
	t      *testing.T
	db     *db.DB
	fake   *spotifytest.Fake
	client *spotify.Client
	svc    *library.Service

	mu      sync.Mutex
	now     time.Time
	changes int
}

func newEnv(t *testing.T, scopes ...string) *env {
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
	e := &env{t: t, db: d, fake: f, client: c, now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	if scopes != nil {
		old := spotify.Scopes
		spotify.Scopes = scopes
		defer func() { spotify.Scopes = old }()
	}
	if err := c.SetClientID(ctx, clientID); err != nil {
		t.Fatal(err)
	}
	req, err := c.BeginAuth(ctx, spotify.LoopbackRedirectURI(8080, ""), "/")
	if err != nil {
		t.Fatal(err)
	}
	back, err := f.Approve(req.URL)
	if err != nil {
		t.Fatal(err)
	}
	p, err := spotify.ParseCallbackURL(back)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Complete(ctx, p); err != nil {
		t.Fatal(err)
	}
	e.svc = library.New(library.Options{DB: d, Client: c, Now: e.clock,
		OnCatalogChanged: func() { e.mu.Lock(); e.changes++; e.mu.Unlock() }})
	return e
}

func (e *env) clock() time.Time { e.mu.Lock(); defer e.mu.Unlock(); return e.now }
func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

func (e *env) sync(kind string) error {
	e.t.Helper()
	return e.svc.Sync(context.Background(), kind)
}

func (e *env) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.Read.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	return n
}

func (e *env) hits() map[string]int {
	return map[string]int{
		"tracks":    e.fake.HitsPrefix("/v1/me/tracks"),
		"albums":    e.fake.HitsPrefix("/v1/me/albums"),
		"playlists": e.fake.HitsPrefix("/v1/me/playlists"),
		"items":     e.fake.HitsPrefix("/v1/playlists/"),
		"catalog":   e.fake.HitsPrefix("/v1/tracks") + e.fake.HitsPrefix("/v1/artists") + e.fake.HitsPrefix("/v1/albums"),
	}
}

func diff(before, after map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range after {
		if d := v - before[k]; d != 0 {
			out[k] = d
		}
	}
	return out
}

var t0 = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// library builds n saved tracks (newest first), two albums and four
// playlists: own (a track, an episode, a local file, a track saved too),
// collaborative, followed and Spotify-made.
func fixture(n int) func(l *spotifytest.Library) {
	return func(l *spotifytest.Library) {
		l.Tracks = nil
		for i := range n {
			l.Tracks = append(l.Tracks, spotifytest.LibraryItem{Track: spotifytest.CatalogTrack{ID: fmt.Sprintf("t%03d", n-1-i)},
				AddedAt: t0.Add(time.Duration(n-1-i) * time.Hour)})
		}
		l.Albums = []spotifytest.LibraryAlbum{{ID: "alb2", AddedAt: t0.Add(48 * time.Hour)}, {ID: "alb1", AddedAt: t0}}
		l.Playlists = []spotifytest.LibraryPlaylist{
			{ID: "own1", Name: "Mine", OwnerID: spotifytest.Me, Snapshot: "s1", Items: []spotifytest.LibraryItem{
				{Track: spotifytest.CatalogTrack{ID: "p1"}, AddedAt: t0, AddedBy: spotifytest.Me},
				{Track: spotifytest.CatalogTrack{ID: "ep1", Name: "An episode"}, Episode: true, AddedAt: t0},
				{Track: spotifytest.CatalogTrack{Name: "Home demo"}, Local: true, AddedAt: t0},
				{Track: spotifytest.CatalogTrack{ID: "t000"}, AddedAt: t0.Add(time.Hour), AddedBy: spotifytest.Me},
			}},
			{ID: "collab1", Name: "Shared", OwnerID: "friend", Collaborative: true, Snapshot: "c1", Items: []spotifytest.LibraryItem{
				{Track: spotifytest.CatalogTrack{ID: "p2"}, AddedAt: t0, AddedBy: "friend"},
			}},
			{ID: "follow1", Name: "Theirs", OwnerID: "someone", Snapshot: "f1", Items: []spotifytest.LibraryItem{
				{Track: spotifytest.CatalogTrack{ID: "p3"}, AddedAt: t0},
			}},
			{ID: "37i9dQZF1DXcBWIGoYBM5M", Name: "Today's Top Hits", OwnerID: "spotify", Snapshot: "x"},
		}
	}
}

func TestFirstSyncWalksEverything(t *testing.T) {
	e := newEnv(t)
	e.fake.SetLibrary(fixture(120))
	if err := e.sync(library.KindFull); err != nil {
		t.Fatal(err)
	}
	h := e.hits()
	// 3 pages of tracks, 1 of albums, 1 of playlists, own and
	// collaborative items; followed and Spotify-made items are never
	// requested; no catalog lookups.
	want := map[string]int{"tracks": 3, "albums": 1, "playlists": 1, "items": 2, "catalog": 0}
	for k, v := range want {
		if h[k] != v {
			t.Errorf("%s requests = %d, want %d", k, h[k], v)
		}
	}
	if n := e.fake.HitsPrefix("/v1/playlists/follow1"); n != 0 {
		t.Errorf("followed playlist items requested %d times", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_spans WHERE list = 'tracks' AND gone_sync IS NULL`); n != 120 {
		t.Errorf("saved tracks = %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_spans WHERE list = 'albums'`); n != 2 {
		t.Errorf("saved albums = %d", n)
	}
	// The own playlist keeps its tracks and local file, not the episode.
	if n := e.count(`SELECT COUNT(*) FROM library_spans WHERE list = 'own1'`); n != 3 {
		t.Errorf("own playlist items = %d, want 3", n)
	}
	var by string
	_ = e.db.Read.QueryRow(`SELECT added_by FROM library_spans WHERE list = 'collab1'`).Scan(&by)
	if by != "friend" {
		t.Errorf("collaborative item added_by = %q", by)
	}
	rels := map[string]string{}
	rows, _ := e.db.Read.Query(`SELECT id, relation || ':' || items_available FROM library_playlists`)
	for rows.Next() {
		var id, r string
		rows.Scan(&id, &r)
		rels[id] = r
	}
	rows.Close()
	wantRel := map[string]string{"own1": "own:1", "collab1": "collaborative:1", "follow1": "followed:0", "37i9dQZF1DXcBWIGoYBM5M": "followed:0"}
	for id, r := range wantRel {
		if rels[id] != r {
			t.Errorf("playlist %s = %q, want %q", id, rels[id], r)
		}
	}
	// The catalog is seeded from the track objects, with nothing queued.
	if n := e.count(`SELECT COUNT(*) FROM tracks WHERE source = 'library' AND hydrated_at IS NOT NULL`); n != 120+2+1 {
		t.Errorf("seeded tracks = %d, want 123", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM albums WHERE id IN ('alb1', 'alb2')`); n != 2 {
		t.Errorf("seeded saved albums = %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM hydration_queue`); n != 0 {
		t.Errorf("queued %d lookups for unplayed library tracks", n)
	}
	// Playlists are named as play contexts; Spotify-made ones are not.
	if n := e.count(`SELECT COUNT(*) FROM contexts WHERE name_source = 'api' AND type = 'playlist'`); n != 3 {
		t.Errorf("named playlist contexts = %d, want 3", n)
	}
	if e.changes == 0 {
		t.Error("catalog change not announced")
	}
	st, err := e.svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.State != library.StateIdle || st.SavedTracks != 120 || st.SavedAlbums != 2 || st.Playlists != 4 ||
		st.PlaylistsWithItems != 2 || st.LastFullAt == 0 {
		t.Errorf("status = %+v", st)
	}
	for _, l := range []string{"tracks", "albums", "playlists"} {
		if st.Lists[l].State != library.ListOK {
			t.Errorf("list %s = %+v", l, st.Lists[l])
		}
	}
}

func TestQuickCheckCostsThreeRequests(t *testing.T) {
	e := newEnv(t)
	e.fake.SetLibrary(fixture(120))
	if err := e.sync(library.KindFull); err != nil {
		t.Fatal(err)
	}
	before := e.hits()
	if err := e.sync(library.KindQuick); err != nil {
		t.Fatal(err)
	}
	if d := diff(before, e.hits()); len(d) != 3 || d["tracks"] != 1 || d["albums"] != 1 || d["playlists"] != 1 {
		t.Errorf("quick check of an unchanged library made %v requests", d)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_walks WHERE mode = 'check'`); n != 2 {
		t.Errorf("checks recorded = %d, want 2", n)
	}
}

func TestRemovalAndResave(t *testing.T) {
	e := newEnv(t)
	e.fake.SetLibrary(fixture(120))
	if err := e.sync(library.KindFull); err != nil {
		t.Fatal(err)
	}
	// t010 is removed (deep in the list): the total drops, so the quick
	// check walks the list and closes its span.
	e.fake.SetLibrary(func(l *spotifytest.Library) {
		l.Tracks = slices.DeleteFunc(l.Tracks, func(it spotifytest.LibraryItem) bool { return it.Track.ID == "t010" })
	})
	e.advance(time.Hour)
	before := e.hits()
	if err := e.sync(library.KindQuick); err != nil {
		t.Fatal(err)
	}
	if d := diff(before, e.hits()); d["tracks"] != 3 {
		t.Errorf("walk after a removal: %v", d)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_spans WHERE item_uri = 'spotify:track:t010' AND gone_sync IS NOT NULL`); n != 1 {
		t.Fatalf("closed spans of t010 = %d", n)
	}
	// Saved again: a new span with the new added_at; the old one stays.
	e.fake.SetLibrary(func(l *spotifytest.Library) {
		l.Tracks = append([]spotifytest.LibraryItem{{Track: spotifytest.CatalogTrack{ID: "t010"}, AddedAt: t0.Add(1000 * time.Hour)}}, l.Tracks...)
	})
	if err := e.sync(library.KindQuick); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_spans WHERE item_uri = 'spotify:track:t010'`); n != 2 {
		t.Errorf("spans of t010 = %d, want 2", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_spans WHERE list = 'tracks' AND gone_sync IS NULL`); n != 120 {
		t.Errorf("present = %d", n)
	}
	// An add plus a removal deeper in the list keeps the total: the quick
	// check sees the new first page, walks and closes the removed one.
	e.fake.SetLibrary(func(l *spotifytest.Library) {
		l.Tracks = slices.DeleteFunc(l.Tracks, func(it spotifytest.LibraryItem) bool { return it.Track.ID == "t050" })
		l.Tracks = append([]spotifytest.LibraryItem{{Track: spotifytest.CatalogTrack{ID: "new1"}, AddedAt: t0.Add(2000 * time.Hour)}}, l.Tracks...)
	})
	if err := e.sync(library.KindQuick); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_spans WHERE item_uri = 'spotify:track:t050' AND gone_sync IS NULL`); n != 0 {
		t.Error("t050 still present")
	}
}

func TestPlaylistSnapshots(t *testing.T) {
	e := newEnv(t)
	e.fake.SetLibrary(fixture(5))
	if err := e.sync(library.KindFull); err != nil {
		t.Fatal(err)
	}
	before := e.hits()
	if err := e.sync(library.KindFull); err != nil {
		t.Fatal(err)
	}
	if d := diff(before, e.hits()); d["items"] != 0 {
		t.Errorf("unchanged playlists walked again: %v", d)
	}
	// A new snapshot: walked; the removed item's span closes.
	e.fake.SetLibrary(func(l *spotifytest.Library) {
		l.Playlists[0].Snapshot = "s2"
		l.Playlists[0].Items = l.Playlists[0].Items[1:]
	})
	before = e.hits()
	if err := e.sync(library.KindQuick); err != nil {
		t.Fatal(err)
	}
	if d := diff(before, e.hits()); d["items"] != 1 {
		t.Errorf("changed playlist: %v", d)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_spans WHERE list = 'own1' AND item_uri = 'spotify:track:p1' AND gone_sync IS NOT NULL`); n != 1 {
		t.Errorf("removed playlist item not closed")
	}
	// An unfollowed playlist is gone, and its items with it.
	e.fake.SetLibrary(func(l *spotifytest.Library) { l.Playlists = l.Playlists[2:] })
	if err := e.sync(library.KindQuick); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_playlists WHERE gone_sync IS NOT NULL`); n != 2 {
		t.Errorf("gone playlists = %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM library_spans WHERE list IN ('own1', 'collab1') AND gone_sync IS NULL`); n != 0 {
		t.Errorf("items of gone playlists still present: %d", n)
	}
}

func TestWithoutLibraryScope(t *testing.T) {
	e := newEnv(t, "user-read-playback-state", "user-read-recently-played", "playlist-read-private")
	e.fake.SetLibrary(fixture(5))
	if err := e.sync(library.KindFull); err != nil {
		t.Fatal(err)
	}
	h := e.hits()
	if h["tracks"] != 0 || h["albums"] != 0 || h["playlists"] != 1 {
		t.Errorf("requests without the scope: %v", h)
	}
	st, _ := e.svc.Status(context.Background())
	if st.Lists["tracks"].State != library.ListNeedsScope || st.Lists["albums"].State != library.ListNeedsScope ||
		st.Lists["playlists"].State != library.ListOK {
		t.Errorf("lists = %+v", st.Lists)
	}
}

func TestScopeRevokedInSpotify(t *testing.T) {
	e := newEnv(t)
	e.fake.SetLibrary(fixture(5))
	e.fake.FailNext("/v1/me/tracks", spotifytest.Failure{Status: 403})
	if err := e.sync(library.KindFull); err != nil {
		t.Fatal(err)
	}
	st, _ := e.svc.Status(context.Background())
	if st.Lists["tracks"].State != library.ListNeedsScope {
		t.Errorf("tracks = %+v", st.Lists["tracks"])
	}
	before := e.hits()
	_ = e.sync(library.KindQuick)
	if d := diff(before, e.hits()); d["tracks"] != 0 {
		t.Errorf("library endpoints retried after a 403: %v", d)
	}
	e.svc.Connected()
	if err := e.sync(library.KindQuick); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.svc.Status(context.Background()); st.Lists["tracks"].State != library.ListOK {
		t.Errorf("after reconnect: %+v", st.Lists["tracks"])
	}
}

func TestQuotaPausesSync(t *testing.T) {
	e := newEnv(t)
	e.fake.SetLibrary(fixture(5))
	e.fake.SetQuotaExceeded("/v1/me/", "3600")
	err := e.sync(library.KindFull)
	if err == nil {
		t.Fatal("sync succeeded with a spent quota")
	}
	st, _ := e.svc.Status(context.Background())
	if st.State != library.StateQuotaExceeded || st.ResumesAt == 0 {
		t.Errorf("status = %+v", st)
	}
	// Spans are untouched; nothing was closed.
	if n := e.count(`SELECT COUNT(*) FROM library_spans`); n != 0 {
		t.Errorf("spans = %d", n)
	}
}

func TestExportPlaysQueueArtists(t *testing.T) {
	e := newEnv(t)
	e.fake.SetLibrary(fixture(3))
	// t001 has export plays: its artist is queued at that priority.
	if _, err := e.db.Write.Exec(`INSERT INTO imports (id, filename, origin, kind, state, created_at) VALUES (1, 'x', 'cli', 'zip', 'done', 0)`); err != nil {
		t.Fatal(err)
	}
	for i := range 4 {
		if _, err := e.db.Write.Exec(`INSERT INTO raw_export_rows (import_id, row_hash, ts_ms, ms_played, content_type, uri, json)
			VALUES (1, ?, ?, 60000, 'track', 'spotify:track:t001', '{}')`, []byte{byte(i)}, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.sync(library.KindFull); err != nil {
		t.Fatal(err)
	}
	var prio int
	if err := e.db.Read.QueryRow(`SELECT priority FROM hydration_queue WHERE entity_type = 'artist' AND id = 'artist-t001'`).Scan(&prio); err != nil || prio != 4 {
		t.Errorf("artist of a played library track: priority %d, %v", prio, err)
	}
	if n := e.count(`SELECT COUNT(*) FROM hydration_queue`); n != 1 {
		t.Errorf("queue = %d, want 1", n)
	}
}

func TestRunSchedules(t *testing.T) {
	e := newEnv(t)
	e.fake.SetLibrary(fixture(3))
	svc := library.New(library.Options{DB: e.db, Client: e.client, FirstDelay: time.Millisecond, Interval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { svc.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(5 * time.Second)
	for e.count(`SELECT COUNT(*) FROM library_syncs WHERE finished_at IS NOT NULL`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("first sync did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var kind string
	_ = e.db.Read.QueryRow(`SELECT kind FROM library_syncs ORDER BY id LIMIT 1`).Scan(&kind)
	if kind != library.KindFull {
		t.Errorf("first sync kind = %s, want full", kind)
	}
	// Sync now runs a quick check at once.
	deadline = time.Now().Add(5 * time.Second)
	for !svc.SyncNow() {
		if time.Now().After(deadline) {
			t.Fatal("SyncNow refused")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for e.count(`SELECT COUNT(*) FROM library_syncs WHERE kind = 'manual' AND finished_at IS NOT NULL`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("manual sync did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
