package hydrate_test

import (
	"context"
	"testing"

	"spotussi/internal/hydrate"
)

// lender sets up a provisional artist with a fallback image and genres.
func (e *env) lender(name, image string, genres ...string) string {
	e.t.Helper()
	id := hydrate.ProvArtistID(name)
	e.exec(`INSERT INTO fallback_artists (id, name, image_url, image_url_small, updated_at) VALUES (?, ?, ?, ?, 1)`,
		id, name, image, image+"-small")
	e.exec(`INSERT INTO catalog_provenance (entity_type, entity_id, field, source, confidence, updated_at)
		VALUES ('artist', ?, 'image_url', 'deezer', 0.85, 1)`, id)
	for _, g := range genres {
		e.exec(`INSERT INTO artist_genres (artist_id, genre) VALUES (?, ?)`, id, g)
	}
	if len(genres) > 0 {
		e.exec(`INSERT INTO catalog_provenance (entity_type, entity_id, field, source, confidence, updated_at)
			VALUES ('artist', ?, 'genres', 'musicbrainz', 0.7, 1)`, id)
	}
	return id
}

// realArtist adds a real (unhydrated) Spotify artist credited on a
// Spotify-hydrated track whose export rows name exportArtist.
func (e *env) realArtist(id, name, trackID, exportArtist string) {
	e.t.Helper()
	e.exec(`INSERT INTO artists (id, name) VALUES (?, ?)`, id, name)
	e.exec(`INSERT INTO tracks (id, name, hydrated_at, source) VALUES (?, 'Song', 1, 'api') ON CONFLICT DO NOTHING`, trackID)
	e.exec(`INSERT INTO track_artists (track_id, artist_id, position)
		VALUES (?, ?, (SELECT COUNT(*) FROM track_artists WHERE track_id = ?))`, trackID, id, trackID)
	e.namedRows(trackID, "Song", exportArtist, "Album", 1, 200000, "trackdone")
}

func (e *env) borrow(h *hydrate.Hydrator) int {
	e.t.Helper()
	n, err := h.Borrow(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) genres(id string) string {
	return e.str(`SELECT COALESCE(group_concat(genre, ','), '') FROM (SELECT genre FROM artist_genres WHERE artist_id = ? ORDER BY genre)`, id)
}

// The only real artist of a name, sharing a track with the provisional
// artist of that name, borrows its image and genres as 'borrowed'.
func TestBorrowSameArtist(t *testing.T) {
	e := newEnv(t)
	e.lender("Band", "https://img/band", "rock", "indie")
	e.realArtist("real-band", "BAND", "t-band", " band")
	h := e.hydrator(hydrate.Options{})
	if n := e.borrow(h); n != 1 {
		t.Fatalf("borrowed %d, want 1", n)
	}
	if got := e.str(`SELECT image_url || '|' || image_url_small FROM artists WHERE id = 'real-band'`); got != "https://img/band|https://img/band-small" {
		t.Errorf("image %q", got)
	}
	if got := e.genres("real-band"); got != "indie,rock" {
		t.Errorf("genres %q", got)
	}
	if got := e.str(`SELECT group_concat(field || ':' || source || ':' || confidence, ' ') FROM (SELECT * FROM catalog_provenance
		WHERE entity_id = 'real-band' ORDER BY field)`); got != "genres:borrowed:0.7 image_url:borrowed:0.85" {
		t.Errorf("provenance %q", got)
	}
	if n := e.borrow(h); n != 0 {
		t.Errorf("second pass changed %d", n)
	}
}

// Two real bands of one name: neither borrows, even though each shares a
// track with the provisional artist of that name.
func TestBorrowTwoBandsSameName(t *testing.T) {
	e := newEnv(t)
	e.lender("Nirvana", "https://img/nirvana", "grunge")
	e.realArtist("nirvana-us", "Nirvana", "t-us", "Nirvana")
	e.realArtist("nirvana-uk", "Nirvana", "t-uk", "Nirvana")
	h := e.hydrator(hydrate.Options{})
	if n := e.borrow(h); n != 0 {
		t.Fatalf("borrowed %d, want 0", n)
	}
	if n := e.int(`SELECT COUNT(*) FROM artists WHERE image_url IS NOT NULL`); n != 0 {
		t.Errorf("%d artists got an image", n)
	}
	if n := e.int(`SELECT COUNT(*) FROM artist_genres WHERE artist_id LIKE 'nirvana-%'`); n != 0 {
		t.Errorf("%d genres borrowed", n)
	}
}

// Same name but no shared track (the real artist's track is credited to
// someone else in the export): no borrowing.
func TestBorrowNeedsSharedTrack(t *testing.T) {
	e := newEnv(t)
	e.lender("Low", "https://img/low", "slowcore")
	e.namedRows("t-prov", "Other Song", "Low", "Album", 1, 200000, "trackdone") // the provisional artist's own track
	e.realArtist("real-low", "Low", "t-real", "Someone Else")
	h := e.hydrator(hydrate.Options{})
	if n := e.borrow(h); n != 0 {
		t.Fatalf("borrowed %d, want 0", n)
	}
	if got := e.genres("real-low"); got != "" {
		t.Errorf("genres %q", got)
	}
}

// Borrowing fills gaps only: an artist's own (oEmbed) image stays, its
// genres are borrowed.
func TestBorrowKeepsOwnImage(t *testing.T) {
	e := newEnv(t)
	e.lender("Band", "https://img/band", "rock")
	e.realArtist("real-band", "Band", "t-band", "Band")
	e.exec(`UPDATE artists SET image_url = 'https://i.scdn.co/own', image_url_small = 'https://i.scdn.co/own-s' WHERE id = 'real-band'`)
	e.exec(`INSERT INTO catalog_provenance (entity_type, entity_id, field, source, confidence, updated_at)
		VALUES ('artist', 'real-band', 'image_url', 'oembed', 1, 1)`)
	h := e.hydrator(hydrate.Options{})
	e.borrow(h)
	if got := e.str(`SELECT image_url FROM artists WHERE id = 'real-band'`); got != "https://i.scdn.co/own" {
		t.Errorf("image %q", got)
	}
	if got := e.genres("real-band"); got != "rock" {
		t.Errorf("genres %q", got)
	}
}

// When a second real artist of the name turns up, the borrowed fields are
// withdrawn: the first one is no longer provably the lender.
func TestBorrowWithdrawn(t *testing.T) {
	e := newEnv(t)
	e.lender("Band", "https://img/band", "rock")
	e.realArtist("band-1", "Band", "t-1", "Band")
	h := e.hydrator(hydrate.Options{})
	if n := e.borrow(h); n != 1 {
		t.Fatalf("borrowed %d", n)
	}
	e.realArtist("band-2", "Band", "t-2", "Band")
	e.borrow(h)
	if got := e.str(`SELECT COALESCE(image_url, 'null') FROM artists WHERE id = 'band-1'`); got != "null" {
		t.Errorf("image %q kept", got)
	}
	if got := e.genres("band-1"); got != "" {
		t.Errorf("genres %q kept", got)
	}
	if n := e.int(`SELECT COUNT(*) FROM catalog_provenance WHERE source = 'borrowed'`); n != 0 {
		t.Errorf("%d borrowed provenance rows left", n)
	}
}

// A Spotify artist lookup replaces borrowed fields.
func TestBorrowedReplacedBySpotify(t *testing.T) {
	e := newEnv(t)
	e.lender("Band", "https://img/band", "rock")
	e.realArtist("real-band", "Band", "t-band", "Band")
	e.exec(`INSERT INTO hydration_queue (entity_type, id, priority) VALUES ('artist', 'real-band', 5)`)
	h := e.hydrator(hydrate.Options{})
	if n := e.borrow(h); n != 1 {
		t.Fatalf("borrowed %d", n)
	}
	e.run(h)
	e.waitFor("artist hydrated", func() bool {
		return e.int(`SELECT COUNT(*) FROM artists WHERE id = 'real-band' AND hydrated_at IS NOT NULL`) == 1
	})
	if n := e.int(`SELECT COUNT(*) FROM catalog_provenance WHERE entity_id = 'real-band'`); n != 0 {
		t.Errorf("borrowed provenance kept after Spotify")
	}
	if got := e.str(`SELECT COALESCE(image_url, '') FROM artists WHERE id = 'real-band'`); got == "https://img/band" {
		t.Errorf("borrowed image kept after Spotify")
	}
	if got := e.genres("real-band"); got == "rock" {
		t.Errorf("borrowed genres kept after Spotify")
	}
	// Hydrated artists never borrow again.
	if n := e.borrow(h); n != 0 {
		t.Errorf("borrowed %d after hydration", n)
	}
}
