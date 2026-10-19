package hydrate_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"spotussi/internal/hydrate"
	"spotussi/internal/metadata"
	"spotussi/internal/metadata/metadatatest"
	"spotussi/internal/spotify/spotifytest"
)

// namedRows inserts n export rows of a track with names, as the real
// export has them.
func (e *env) namedRows(id, track, artist, album string, n int, msPlayed int64, reasonEnd string) {
	e.t.Helper()
	e.exec(`INSERT INTO imports (id, filename, origin, kind, state, created_at) VALUES (1, 'x.zip', 'cli', 'zip', 'done', 0)
		ON CONFLICT DO NOTHING`)
	for range n {
		k := exportSeq.Add(1)
		h := sha256.Sum256([]byte(fmt.Sprint("named", k)))
		e.exec(`INSERT INTO raw_export_rows (import_id, row_hash, ts_ms, ms_played, content_type, uri, track_name,
			artist_name, album_name, reason_end, json) VALUES (1, ?, ?, ?, 'track', ?, ?, ?, ?, ?, '{}')`,
			h[:], k*1000, msPlayed, "spotify:track:"+id, track, artist, album, reasonEnd)
	}
}

func (e *env) fallbacks(s *metadatatest.Server) hydrate.Fallbacks {
	o, d, m := s.Clients()
	return hydrate.Fallbacks{OEmbed: o, Deezer: d, MusicBrainz: m, Always: true}
}

func (e *env) step(h *hydrate.Hydrator, src string) int {
	e.t.Helper()
	n, err := h.FallbackStep(context.Background(), src)
	if err != nil {
		e.t.Fatalf("%s step: %v", src, err)
	}
	return n
}

const unknownID = "1111111111111111111111"

func TestFallbackPipeline(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := metadatatest.New(t)
	e.namedRows(metadatatest.TrackID, metadatatest.Track, metadatatest.Artist, metadatatest.Album, 5, metadatatest.DurationMs, "trackdone")
	e.namedRows(metadatatest.TrackID, metadatatest.Track, metadatatest.Artist, metadatatest.Album, 2, 40000, "fwdbtn")
	e.namedRows(unknownID, "Nothing Here", "Nobody", "Nowhere", 1, 100000, "trackdone")
	h := e.hydrator(hydrate.Options{Fallbacks: e.fallbacks(s)})
	if _, err := h.Plan(ctx); err != nil {
		t.Fatal(err)
	}
	artistID := hydrate.ProvArtistID(metadatatest.Artist)
	albumID := hydrate.ProvAlbumID(metadatatest.Artist, metadatatest.Album)

	// oEmbed: exact title and cover; the unknown track is a 404.
	if n := e.step(h, metadata.SourceOEmbed); n != 2 {
		t.Fatalf("oembed settled %d, want 2", n)
	}
	if got := e.str(`SELECT name || '|' || source || '|' || COALESCE(hydrated_at, 'null') FROM tracks WHERE id = ?`, metadatatest.TrackID); got != metadatatest.Track+"|fallback|null" {
		t.Errorf("track row = %q", got)
	}
	if got := e.str(`SELECT image_url FROM fallback_albums WHERE id = ?`, albumID); got != "https://i.scdn.co/image/ab67616d0000b273baf89eb11ec7c657805d2da0" {
		t.Errorf("album cover = %q", got)
	}
	if got := e.str(`SELECT status FROM fallback_state WHERE source = 'oembed' AND id = ?`, unknownID); got != "no_match" {
		t.Errorf("unknown track oembed = %q", got)
	}
	if n := e.step(h, metadata.SourceOEmbed); n != 0 {
		t.Errorf("oembed repeated %d lookups", n)
	}

	// Deezer: duration, ISRC, artist picture, album date; the cover stays
	// oEmbed's (higher confidence).
	if n := e.step(h, metadata.SourceDeezer); n != 2 {
		t.Fatalf("deezer settled %d, want 2", n)
	}
	if got := e.str(`SELECT duration_ms || '|' || isrc || '|' || explicit FROM tracks WHERE id = ?`, metadatatest.TrackID); got != "213000|"+metadatatest.ISRC+"|0" {
		t.Errorf("track fields = %q", got)
	}
	if c := e.str(`SELECT confidence FROM fallback_state WHERE source = 'deezer' AND entity_type = 'track' AND id = ?`, metadatatest.TrackID); c < "0.9" {
		t.Errorf("track confidence %s", c)
	}
	if got := e.str(`SELECT source FROM catalog_provenance WHERE entity_type = 'track' AND entity_id = ? AND field = 'duration_ms'`, metadatatest.TrackID); got != "deezer" {
		t.Errorf("duration provenance %q", got)
	}
	if got := e.str(`SELECT release_date || '|' || release_date_precision || '|' || album_type || '|' || total_tracks || '|' || image_url
		FROM fallback_albums WHERE id = ?`, albumID); !strings.HasPrefix(got, "1987-11-12|day|album|10|https://i.scdn.co/") {
		t.Errorf("album = %q", got)
	}
	if got := e.str(`SELECT source FROM catalog_provenance WHERE entity_type = 'album' AND entity_id = ? AND field = 'image_url'`, albumID); got != "oembed" {
		t.Errorf("cover provenance %q, want oembed kept", got)
	}
	if got := e.str(`SELECT image_url FROM fallback_artists WHERE id = ?`, artistID); !strings.Contains(got, "dzcdn.net/images/artist/") {
		t.Errorf("artist picture = %q", got)
	}
	if got := e.str(`SELECT status FROM fallback_state WHERE source = 'deezer' AND id = ?`, unknownID); got != "no_match" {
		t.Errorf("unknown track deezer = %q", got)
	}

	// MusicBrainz: genres by the Deezer ISRC (2 requests), under the
	// provisional artist ID.
	before := s.Hits(metadata.SourceMusicBrainz)
	if n := e.step(h, metadata.SourceMusicBrainz); n != 2 {
		t.Fatalf("musicbrainz settled %d, want 2", n)
	}
	if got := e.str(`SELECT group_concat(genre, ',') FROM (SELECT genre FROM artist_genres WHERE artist_id = ? ORDER BY genre)`, artistID); got != "blue-eyed soul,dance-pop,pop" {
		t.Errorf("genres = %q", got)
	}
	if got := e.str(`SELECT mbid FROM fallback_artists WHERE id = ?`, artistID); got != metadatatest.MBID {
		t.Errorf("mbid = %q", got)
	}
	// ISRC + artist for Rick Astley, a search for the unknown artist.
	if got := s.Hits(metadata.SourceMusicBrainz) - before; got != 3 {
		t.Errorf("musicbrainz requests = %d, want 3", got)
	}

	// Still queued for Spotify; status counts per source.
	if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE entity_type = 'track'`); n != 2 {
		t.Errorf("queue = %d, want both tracks still queued", n)
	}
	st, err := h.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Fallbacks) != 3 {
		t.Fatalf("fallback status: %+v", st.Fallbacks)
	}
	for _, f := range st.Fallbacks {
		if f.Matched != 1 || f.Unmatched != 1 || f.Pending != 0 || f.Requests == 0 {
			t.Errorf("%s status %+v", f.Source, f)
		}
	}
	if st.Fallbacks[1].AlbumsMatched != 1 || st.Fallbacks[1].ArtistsMatched != 1 {
		t.Errorf("deezer entities %+v", st.Fallbacks[1])
	}

	// A real Spotify lookup replaces every fallback field.
	e.fake.SetTrack(spotifytest.CatalogTrack{ID: metadatatest.TrackID, Name: "Spotify Name", DurationMs: 213573})
	e.fake.SetMissing("/v1/tracks/" + unknownID)
	e.run(h)
	e.waitFor("Spotify hydration", func() bool {
		return e.int(`SELECT COUNT(*) FROM tracks WHERE id = ? AND hydrated_at IS NOT NULL`, metadatatest.TrackID) == 1
	})
	if got := e.str(`SELECT name || '|' || duration_ms || '|' || source || '|' || (isrc = ?) FROM tracks WHERE id = ?`, metadatatest.ISRC, metadatatest.TrackID); got != "Spotify Name|213573|api|0" {
		t.Errorf("after Spotify: %q", got)
	}
	if n := e.int(`SELECT COUNT(*) FROM catalog_provenance WHERE entity_type = 'track' AND entity_id = ?`, metadatatest.TrackID); n != 0 {
		t.Errorf("%d fallback provenance rows left after Spotify", n)
	}
}

// Without Spotify (nil) the worker runs the fallbacks alone.
func TestFallbacksOnlyRun(t *testing.T) {
	e := newEnv(t)
	s := metadatatest.New(t)
	e.namedRows(metadatatest.TrackID, metadatatest.Track, metadatatest.Artist, metadatatest.Album, 3, metadatatest.DurationMs, "trackdone")
	fb := e.fallbacks(s)
	fb.Always = false
	h := hydrate.New(hydrate.Options{DB: e.db, Fallbacks: fb, Now: e.clock})
	e.run(h)
	e.waitFor("all three stages", func() bool {
		return e.int(`SELECT COUNT(*) FROM fallback_state WHERE entity_type = 'track' AND status = 'matched'`) == 3
	})
	st, err := h.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.State != hydrate.StateDisabled {
		t.Errorf("spotify state %q", st.State)
	}
}

// A failing source backs the entry off; it isn't retried until due.
func TestFallbackFailureBacksOff(t *testing.T) {
	e := newEnv(t)
	s := metadatatest.New(t)
	s.Fail = func(r *http.Request) int { return http.StatusInternalServerError }
	e.namedRows(metadatatest.TrackID, metadatatest.Track, metadatatest.Artist, metadatatest.Album, 1, 1000, "")
	h := e.hydrator(hydrate.Options{Fallbacks: e.fallbacks(s)})
	if _, err := h.Plan(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.step(h, metadata.SourceOEmbed)
	if got := e.str(`SELECT status || '|' || attempts FROM fallback_state WHERE source = 'oembed'`); got != "failed|1" {
		t.Errorf("state %q", got)
	}
	if n := e.step(h, metadata.SourceOEmbed); n != 0 {
		t.Errorf("retried before due")
	}
	s.Fail = nil
	e.advance(2 * time.Minute)
	if n := e.step(h, metadata.SourceOEmbed); n != 1 {
		t.Errorf("not retried when due")
	}
	if got := e.str(`SELECT status FROM fallback_state WHERE source = 'oembed'`); got != "matched" {
		t.Errorf("state %q", got)
	}
}

// An unhydrated Spotify artist in the queue gets its oEmbed image, which
// a Spotify artist lookup replaces.
func TestOEmbedArtistImage(t *testing.T) {
	e := newEnv(t)
	s := metadatatest.New(t)
	e.exec(`INSERT INTO artists (id, name) VALUES (?, 'Rick Astley')`, metadatatest.ArtistID)
	e.exec(`INSERT INTO hydration_queue (entity_type, id, priority) VALUES ('artist', ?, 5)`, metadatatest.ArtistID)
	h := e.hydrator(hydrate.Options{Fallbacks: e.fallbacks(s)})
	if n := e.step(h, metadata.SourceOEmbed); n != 1 {
		t.Fatalf("settled %d", n)
	}
	if got := e.str(`SELECT image_url || '|' || image_url_small FROM artists WHERE id = ?`, metadatatest.ArtistID); !strings.Contains(got, "ab6761610000e5eb") || !strings.Contains(got, "ab6761610000f178") {
		t.Errorf("artist images %q", got)
	}
	e.run(h)
	e.waitFor("artist hydrated", func() bool {
		return e.int(`SELECT COUNT(*) FROM artists WHERE id = ? AND hydrated_at IS NOT NULL`, metadatatest.ArtistID) == 1
	})
	if n := e.int(`SELECT COUNT(*) FROM catalog_provenance WHERE entity_id = ?`, metadatatest.ArtistID); n != 0 {
		t.Errorf("provenance kept after Spotify")
	}
}

// The provisional IDs are the ones derivation makes (D20).
func TestProvisionalIDs(t *testing.T) {
	if got, want := hydrate.ProvArtistID("  Rick Astley "), "ar_"+base64.RawURLEncoding.EncodeToString([]byte("rick astley")); got != want {
		t.Errorf("artist %q, want %q", got, want)
	}
	if got, want := hydrate.ProvAlbumID("Rick Astley", "Whenever You Need Somebody"),
		"al_"+base64.RawURLEncoding.EncodeToString([]byte("rick astley\x00whenever you need somebody")); got != want {
		t.Errorf("album %q, want %q", got, want)
	}
}

// HYDRATE_SPOTIFY_LOOKUPS=false (no Spotify client): seeding and the
// fallbacks run, and not one Spotify catalog request is made, for tracks,
// artists or contexts; queue entries stay queued and uncharged.
func TestNoSpotifyLookups(t *testing.T) {
	e := newEnv(t)
	s := metadatatest.New(t)
	e.namedRows(metadatatest.TrackID, metadatatest.Track, metadatatest.Artist, metadatatest.Album, 3, metadatatest.DurationMs, "trackdone")
	e.exec(`INSERT INTO artists (id, name) VALUES ('art-q', 'Queued Artist')`)
	e.exec(`INSERT INTO contexts (uri, type, name_source) VALUES ('spotify:playlist:pl1', 'playlist', 'unresolved')`)
	e.pollerPlay(spotifytest.PlayerSpec{TrackID: "live1", TrackName: "Live One", DurationMs: 180000, Playing: true,
		ContextURI: "spotify:playlist:pl1"}.JSON(), "spotify:playlist:pl1")
	fb := e.fallbacks(s)
	fb.Always = false
	h := hydrate.New(hydrate.Options{DB: e.db, Fallbacks: fb, Now: e.clock, LimiterStats: e.client.LimiterStats})
	before := e.fake.HitsPrefix("/v1/")
	e.run(h)
	e.waitFor("all three fallback stages", func() bool {
		return e.int(`SELECT COUNT(*) FROM fallback_state WHERE entity_type = 'track' AND id = ? AND status = 'matched'`, metadatatest.TrackID) == 3
	})
	e.waitFor("seeding", func() bool {
		return e.int(`SELECT COUNT(*) FROM tracks WHERE id = 'live1' AND hydrated_at IS NOT NULL`) == 1
	})
	h.Wake()
	time.Sleep(100 * time.Millisecond) // a woken worker would look things up now
	if log := e.fake.CatalogLog(); len(log) != 0 {
		t.Errorf("Spotify catalog requests: %v", log)
	}
	if n := e.fake.HitsPrefix("/v1/") - before; n != 0 {
		t.Errorf("%d Spotify API requests", n)
	}
	if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE attempts > 0`); n != 0 {
		t.Errorf("%d queue entries charged", n)
	}
	for _, typ := range []string{"track", "artist", "context"} {
		if n := e.int(`SELECT COUNT(*) FROM hydration_queue WHERE entity_type = ?`, typ); n == 0 {
			t.Errorf("no %s left queued for Spotify", typ)
		}
	}
	st, err := h.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.State != hydrate.StateDisabled || !st.ETA.IsZero() || !st.ResumesAt.IsZero() {
		t.Errorf("status %q eta %v resumes %v", st.State, st.ETA, st.ResumesAt)
	}
}

// A track no stream played to the end (its longest stream outlasts the
// real length, as seeking back does) matches Deezer on exact names.
func TestDeezerWeakDuration(t *testing.T) {
	e := newEnv(t)
	s := metadatatest.New(t)
	e.namedRows(metadatatest.TrackID, metadatatest.Track, metadatatest.Artist, metadatatest.Album, 2, 400000, "endplay")
	fb := e.fallbacks(s)
	fb.OEmbed, fb.MusicBrainz = nil, nil
	h := e.hydrator(hydrate.Options{Fallbacks: fb})
	if _, err := h.Plan(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.step(h, metadata.SourceDeezer)
	if got := e.str(`SELECT status FROM fallback_state WHERE source = 'deezer' AND entity_type = 'track' AND id = ?`, metadatatest.TrackID); got != "matched" {
		t.Errorf("state %q, want matched", got)
	}
}

// A matcher change re-queues Deezer's unmatched tracks once, and nothing
// else: matches and artist and album states stay.
func TestRematch(t *testing.T) {
	e := newEnv(t)
	for _, r := range []struct{ typ, id, status string }{
		{"track", "t-nomatch", "no_match"}, {"track", "t-match", "matched"}, {"track", "t-failed", "failed"},
		{"artist", "ar_x", "no_match"}, {"album", "al_x", "no_match"},
	} {
		e.exec(`INSERT INTO fallback_state (source, entity_type, id, status, updated_at) VALUES ('deezer', ?, ?, ?, 1)`, r.typ, r.id, r.status)
	}
	e.exec(`INSERT INTO fallback_state (source, entity_type, id, status, updated_at) VALUES ('oembed', 'track', 't-nomatch', 'no_match', 1)`)
	h := e.hydrator(hydrate.Options{})
	ctx := context.Background()
	if n, err := h.Rematch(ctx); err != nil || n != 1 {
		t.Fatalf("rematch = %d, %v; want 1", n, err)
	}
	if got := e.str(`SELECT group_concat(source || ':' || id, ' ') FROM (SELECT * FROM fallback_state ORDER BY source, id)`); got != "deezer:al_x deezer:ar_x deezer:t-failed deezer:t-match oembed:t-nomatch" {
		t.Errorf("states left: %s", got)
	}
	e.exec(`INSERT INTO fallback_state (source, entity_type, id, status, updated_at) VALUES ('deezer', 'track', 't-nomatch', 'no_match', 2)`)
	if n, err := h.Rematch(ctx); err != nil || n != 0 {
		t.Errorf("second rematch = %d, %v; want 0", n, err)
	}
}
