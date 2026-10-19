package hydrate_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"spotussi/internal/hydrate"
	"spotussi/internal/metadata"
	"spotussi/internal/metadata/metadatatest"
)

// ListenBrainz finds the artist and its genres in one batch, a second
// track of that artist is matched through it, and MusicBrainz then skips
// the artist without a request (D37).
func TestListenBrainzStage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := metadatatest.New(t)
	const sibling = "2222222222222222222222"
	e.namedRows(metadatatest.TrackID, metadatatest.Track, metadatatest.Artist, metadatatest.Album, 3, metadatatest.DurationMs, "trackdone")
	e.namedRows(sibling, "Together Forever", metadatatest.Artist, "Hold Me in Your Arms", 2, 200000, "trackdone")
	e.namedRows(unknownID, "Nothing Here", "Nobody", "Nowhere", 1, 100000, "trackdone")
	_, _, mb := s.Clients()
	h := e.hydrator(hydrate.Options{Fallbacks: hydrate.Fallbacks{ListenBrainz: s.ListenBrainz(), MusicBrainz: mb, Always: true}})
	if _, err := h.Plan(ctx); err != nil {
		t.Fatal(err)
	}
	artistID := hydrate.ProvArtistID(metadatatest.Artist)

	// MusicBrainz waits for ListenBrainz.
	if n := e.step(h, metadata.SourceMusicBrainz); n != 0 {
		t.Fatalf("musicbrainz ran %d before listenbrainz", n)
	}
	if n := e.step(h, metadata.SourceListenBrainz); n != 3 {
		t.Fatalf("listenbrainz settled %d, want 3", n)
	}
	// Lookups with the album (3 tracks), without it (the 2 not found),
	// then metadata and Spotify IDs for the one recording found.
	if got := s.Hits(metadata.SourceListenBrainz); got != 4 {
		t.Errorf("listenbrainz requests = %d, want 4", got)
	}
	if got := e.str(`SELECT group_concat(genre, ',') FROM (SELECT genre FROM artist_genres WHERE artist_id = ? ORDER BY genre)`, artistID); got != "blue-eyed soul,dance-pop,pop" {
		t.Errorf("genres = %q", got)
	}
	if got := e.str(`SELECT mbid FROM fallback_artists WHERE id = ?`, artistID); got != metadatatest.MBID {
		t.Errorf("mbid = %q", got)
	}
	if got := e.str(`SELECT source || '|' || confidence FROM catalog_provenance WHERE entity_type = 'artist' AND entity_id = ? AND field = 'genres'`, artistID); got != "listenbrainz|0.95" {
		t.Errorf("genres provenance = %q", got)
	}
	for id, want := range map[string]string{metadatatest.TrackID: "matched|" + metadatatest.MBID, sibling: "matched|" + metadatatest.MBID,
		unknownID: "no_match|"} {
		if got := e.str(`SELECT status || '|' || COALESCE(external_id, '') FROM fallback_state WHERE source = 'listenbrainz'
			AND entity_type = 'track' AND id = ?`, id); got != want {
			t.Errorf("track %s = %q, want %q", id, got, want)
		}
	}
	// No track fields: Deezer's job.
	if n := e.int(`SELECT COUNT(*) FROM catalog_provenance WHERE entity_type = 'track'`); n != 0 {
		t.Errorf("%d track fields written", n)
	}
	if n := e.step(h, metadata.SourceListenBrainz); n != 0 {
		t.Errorf("listenbrainz repeated %d", n)
	}

	// MusicBrainz: only the unknown artist costs a request (a search).
	if n := e.step(h, metadata.SourceMusicBrainz); n != 3 {
		t.Fatalf("musicbrainz settled %d, want 3", n)
	}
	if got := s.Hits(metadata.SourceMusicBrainz); got != 1 {
		t.Errorf("musicbrainz requests = %d, want 1", got)
	}
	if got := e.str(`SELECT status || '|' || external_id FROM fallback_state WHERE source = 'musicbrainz' AND entity_type = 'track' AND id = ?`,
		metadatatest.TrackID); got != "matched|"+metadatatest.MBID {
		t.Errorf("musicbrainz track state = %q", got)
	}
	if n := e.int(`SELECT COUNT(*) FROM fallback_state WHERE source = 'musicbrainz' AND entity_type = 'artist'`); n != 1 {
		t.Errorf("musicbrainz artist states = %d, want the unknown artist only", n)
	}

	st, err := h.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if f := st.Fallbacks[2]; f.Source != metadata.SourceListenBrainz || f.Matched != 2 || f.Unmatched != 1 || f.ArtistsMatched != 1 ||
		f.Requests != 4 {
		t.Errorf("listenbrainz status %+v", f)
	}
}

// A ListenBrainz outage backs the stage off and, after 3 failures, leaves
// the tracks to MusicBrainz, which keeps the artist's Spotify link.
func TestListenBrainzFailureLeavesToMusicBrainz(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := metadatatest.New(t)
	s.Fail = func(r *http.Request) int {
		if strings.HasSuffix(r.URL.Path, "-lookup/json") {
			return http.StatusBadGateway
		}
		return 0
	}
	e.namedRows(metadatatest.TrackID, metadatatest.Track, metadatatest.Artist, metadatatest.Album, 3, metadatatest.DurationMs, "trackdone")
	_, _, mb := s.Clients()
	h := e.hydrator(hydrate.Options{Fallbacks: hydrate.Fallbacks{ListenBrainz: s.ListenBrainz(), MusicBrainz: mb, Always: true}})
	if _, err := h.Plan(ctx); err != nil {
		t.Fatal(err)
	}
	for i, wait := range []time.Duration{0, 2 * time.Minute, 3 * time.Minute} {
		e.advance(wait)
		if n, err := h.FallbackStep(ctx, metadata.SourceListenBrainz); n != 0 || err == nil {
			t.Fatalf("attempt %d: settled %d, err %v", i+1, n, err)
		}
		if n := e.step(h, metadata.SourceMusicBrainz); n != 0 && i < 2 {
			t.Fatalf("musicbrainz ran after %d failures", i+1)
		}
	}
	if got := e.str(`SELECT status || '|' || attempts FROM fallback_state WHERE source = 'listenbrainz'`); got != "failed|3" {
		t.Errorf("listenbrainz state %q", got)
	}
	// MusicBrainz went ahead after the third failure (name search).
	if got := e.str(`SELECT mbid || '|' || spotify_id FROM fallback_artists WHERE id = ?`, hydrate.ProvArtistID(metadatatest.Artist)); got != metadatatest.MBID+"|"+metadatatest.ArtistID {
		t.Errorf("artist = %q", got)
	}
}
