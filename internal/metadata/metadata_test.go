package metadata_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"spotussi/internal/metadata"
	"spotussi/internal/metadata/metadatatest"
)

func TestOEmbed(t *testing.T) {
	s := metadatatest.New(t)
	o, _, _ := s.Clients()
	ctx := context.Background()
	r, err := o.Lookup(ctx, "spotify:track:"+metadatatest.TrackID)
	if err != nil || r.Title != metadatatest.Track || r.ThumbnailURL == "" {
		t.Fatalf("track: %+v %v", r, err)
	}
	large, small := metadata.SpotifyImages(r.ThumbnailURL)
	if large != "https://i.scdn.co/image/ab67616d0000b273baf89eb11ec7c657805d2da0" ||
		small != "https://i.scdn.co/image/ab67616d00004851baf89eb11ec7c657805d2da0" {
		t.Errorf("album images: %s %s", large, small)
	}
	r, err = o.Lookup(ctx, "spotify:artist:"+metadatatest.ArtistID)
	if err != nil || r.Title != metadatatest.Artist {
		t.Fatalf("artist: %+v %v", r, err)
	}
	if large, small := metadata.SpotifyImages(r.ThumbnailURL); !strings.Contains(large, "/ab6761610000e5eb") ||
		!strings.Contains(small, "/ab6761610000f178") {
		t.Errorf("artist images: %s %s", large, small)
	}
	if _, err := o.Lookup(ctx, "spotify:track:0000000000000000000000"); !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	if got := s.Hits("oembed"); got != 3 {
		t.Errorf("hits = %d", got)
	}
}

func TestSpotifyImagesUnknown(t *testing.T) {
	for _, u := range []string{"https://example.org/x.jpg", "https://i.scdn.co/image/ab67616d", "https://i.scdn.co/image/abcdef0123456789aaaa"} {
		if l, s := metadata.SpotifyImages(u); l != u || s != u {
			t.Errorf("%s: %s %s", u, l, s)
		}
	}
}

func TestDeezer(t *testing.T) {
	s := metadatatest.New(t)
	_, d, _ := s.Clients()
	ctx := context.Background()
	ts, err := d.SearchTracks(ctx, metadatatest.Artist+" track:"+metadata.Quote(metadatatest.Track), 10)
	if err != nil || len(ts) == 0 {
		t.Fatalf("search: %d %v", len(ts), err)
	}
	if ts[0].ISRC == "" || ts[0].Duration == 0 || ts[0].Artist.Name != metadatatest.Artist || ts[0].Album.ID == 0 {
		t.Errorf("first hit: %+v", ts[0])
	}
	if ts, err := d.SearchTracks(ctx, "nothing", 10); err != nil || len(ts) != 0 {
		t.Errorf("empty: %v %v", ts, err)
	}
	a, err := d.Album(ctx, 901415162)
	if err != nil || a.ReleaseDate != "1987-11-12" || a.RecordType != "album" || a.Tracks != 10 || !metadata.DeezerImage(a.CoverXL) {
		t.Fatalf("album: %+v %v", a, err)
	}
	if _, err := d.Album(ctx, 1); !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("unknown album: %v", err)
	}
	as, err := d.SearchArtists(ctx, metadatatest.Artist, 10)
	if err != nil || len(as) == 0 || as[0].Name != metadatatest.Artist {
		t.Fatalf("artists: %v %v", as, err)
	}
}

// Deezer's in-body quota error pauses the limiter and is retried.
func TestDeezerQuotaRetried(t *testing.T) {
	s := metadatatest.New(t)
	s.DeezerQuota = 2
	ua := metadata.UserAgent("test", "")
	lim := metadata.NewLimiter(1000, 100)
	d := metadata.NewDeezer(s.URL, ua, s.Client(), lim)
	start := time.Now()
	// The pause after a quota error is 5 s, then 10 s: too long for a
	// test, so pass a context that ends first.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := d.SearchArtists(ctx, metadatatest.Artist, 10)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if lim.PausedUntil().Before(start.Add(4 * time.Second)) {
		t.Errorf("limiter not paused: %v", lim.PausedUntil())
	}
}

// Rate limits retry after the pause and give up with RateLimitedError.
func TestRateLimitedGivesUp(t *testing.T) {
	s := metadatatest.New(t)
	var n atomic.Int32
	s.Fail = func(r *http.Request) int {
		n.Add(1)
		return http.StatusServiceUnavailable
	}
	// A zero-length Retry-After isn't sent; build a client whose default
	// backoff the test can afford by retrying against a limiter whose
	// pauses we clear.
	lim := metadata.NewLimiter(1000, 100)
	m := metadata.NewMusicBrainz(s.URL, "ua", s.Client(), lim)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := m.Artist(ctx, metadatatest.MBID)
	if err == nil || n.Load() != 1 {
		t.Fatalf("err %v after %d requests", err, n.Load())
	}
	if lim.PausedUntil().IsZero() {
		t.Error("503 did not pause the limiter")
	}
}

func TestMusicBrainz(t *testing.T) {
	s := metadatatest.New(t)
	_, _, m := s.Clients()
	ctx := context.Background()
	recs, err := m.ISRC(ctx, metadatatest.ISRC)
	if err != nil || len(recs) == 0 || recs[0].Credits[0].Artist.ID != metadatatest.MBID {
		t.Fatalf("isrc: %+v %v", recs, err)
	}
	if _, err := m.ISRC(ctx, "XX0000000000"); !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("unknown isrc: %v", err)
	}
	a, err := m.Artist(ctx, metadatatest.MBID)
	if err != nil || len(a.Genres) == 0 {
		t.Fatalf("artist: %+v %v", a, err)
	}
	if g := metadata.TopGenres(a.Genres, 5); len(g) == 0 || g[0] != "blue-eyed soul" {
		t.Errorf("genres: %v", g)
	}
	as, err := m.SearchArtists(ctx, metadatatest.Artist, 5)
	if err != nil || len(as) < 2 || as[0].ID != metadatatest.MBID || as[0].Score != 100 {
		t.Fatalf("search: %+v %v", as, err)
	}
}

func TestTopGenres(t *testing.T) {
	gs := []metadata.MBGenre{{Name: "Rock", Count: 9}, {Name: "pop", Count: 3}, {Name: "jazz", Count: 2},
		{Name: "rock", Count: 1}, {Name: "", Count: 5}}
	got := metadata.TopGenres(gs, 5)
	if strings.Join(got, ",") != "rock,pop" {
		t.Errorf("got %v", got)
	}
	if got := metadata.TopGenres(gs, 1); len(got) != 1 {
		t.Errorf("cap: %v", got)
	}
}

func TestUserAgent(t *testing.T) {
	if ua := metadata.UserAgent("", ""); !strings.HasPrefix(ua, "spotussi/dev (") || strings.Contains(ua, "@") {
		t.Errorf("ua %q", ua)
	}
	if ua := metadata.UserAgent("1.2", "https://example.org"); !strings.Contains(ua, "spotussi/1.2") ||
		!strings.Contains(ua, "https://example.org") {
		t.Errorf("ua %q", ua)
	}
}

func TestLimiter(t *testing.T) {
	l := metadata.NewLimiter(50, 2)
	ctx := context.Background()
	start := time.Now()
	for range 7 { // 2 from the burst, 5 at 50/s: about 100 ms
		if err := l.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if el := time.Since(start); el < 70*time.Millisecond || el > time.Second {
		t.Errorf("7 waits took %v", el)
	}
	l.Pause(150 * time.Millisecond)
	start = time.Now()
	if err := l.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el < 140*time.Millisecond {
		t.Errorf("pause not honoured: %v", el)
	}
	cctx, cancel := context.WithCancel(ctx)
	l.Pause(time.Hour)
	cancel()
	if err := l.Wait(cctx); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled wait: %v", err)
	}
}

func TestListenBrainz(t *testing.T) {
	s := metadatatest.New(t)
	lb := s.ListenBrainz()
	ctx := context.Background()
	items := []metadata.LBLookup{
		{Artist: "Nobody", Recording: "Nothing", Release: "Nowhere"},
		{Artist: "rick astley", Recording: "Never Gonna Give You Up", Release: metadatatest.Album},
		{Artist: metadatatest.Artist, Recording: metadatatest.Track, Release: "Another Album"},
	}
	hits, err := lb.Lookup(ctx, items, true)
	if err != nil || len(hits) != 1 || hits[0].Index != 1 || hits[0].RecordingID != metadatatest.RecordingMBID ||
		len(hits[0].ArtistMBIDs) != 1 || hits[0].ArtistMBIDs[0] != metadatatest.MBID {
		t.Fatalf("lookup with release: %+v %v", hits, err)
	}
	// Without the release, the other album's item matches too.
	if hits, err := lb.Lookup(ctx, items, false); err != nil || len(hits) != 2 || hits[1].Index != 2 {
		t.Fatalf("lookup without release: %+v %v", hits, err)
	}
	recs, err := lb.Recordings(ctx, []string{metadatatest.RecordingMBID, "00000000-0000-0000-0000-000000000000"})
	r, ok := recs[metadatatest.RecordingMBID]
	if err != nil || len(recs) != 1 || !ok || r.Recording.Name != metadatatest.Track || r.Recording.LengthMs != 212960 ||
		len(r.Artist.Artists) != 1 || r.Artist.Artists[0].ID != metadatatest.MBID {
		t.Fatalf("recordings: %+v %v", recs, err)
	}
	if g := metadata.TopGenres(r.ArtistGenres(metadatatest.MBID), 5); strings.Join(g, ",") != "blue-eyed soul,dance-pop,pop" {
		t.Errorf("genres %v", g)
	}
	if g := r.ArtistGenres("someone-else"); len(g) != 0 {
		t.Errorf("another artist's genres %v", g)
	}
	ids, err := lb.SpotifyTrackIDs(ctx, []string{metadatatest.RecordingMBID})
	if err != nil || len(ids[metadatatest.RecordingMBID]) == 0 {
		t.Fatalf("spotify ids: %v %v", ids, err)
	}
	if lb.Requests() != 4 {
		t.Errorf("requests %d", lb.Requests())
	}
}

func TestSpotifyArtistID(t *testing.T) {
	s := metadatatest.New(t)
	_, _, m := s.Clients()
	a, err := m.Artist(context.Background(), metadatatest.MBID)
	if err != nil || a.SpotifyArtistID() != metadatatest.ArtistID {
		t.Fatalf("recorded artist: %q %v", a.SpotifyArtistID(), err)
	}
	link := func(urls ...string) *metadata.MBArtist {
		var a metadata.MBArtist
		for _, u := range urls {
			a.Relations = append(a.Relations, struct {
				Type string `json:"type"`
				URL  struct {
					Resource string `json:"resource"`
				} `json:"url"`
			}{})
			a.Relations[len(a.Relations)-1].URL.Resource = u
		}
		return &a
	}
	const other = "4tZwfgrHOc3mvqYlEYSvVi"
	for _, c := range []struct {
		urls []string
		want string
	}{
		{nil, ""},
		{[]string{"https://open.spotify.com/artist/" + other}, other},
		{[]string{"https://open.spotify.com/artist/" + other, "https://open.spotify.com/artist/" + other}, other},
		{[]string{"https://open.spotify.com/artist/" + other, "https://open.spotify.com/artist/" + metadatatest.ArtistID}, ""},
		{[]string{"https://open.spotify.com/album/" + other, "https://example.org/artist/" + other}, ""},
		{[]string{"https://open.spotify.com/artist/short"}, ""},
	} {
		if got := link(c.urls...).SpotifyArtistID(); got != c.want {
			t.Errorf("%v: got %q, want %q", c.urls, got, c.want)
		}
	}
}
