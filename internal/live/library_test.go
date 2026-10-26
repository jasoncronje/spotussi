package live_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"spotussi/internal/live"
	"spotussi/internal/refdata"
)

// TestLibraryMatchesReleases checks what the mock can't: a saved track
// counts the plays of tracks with its ISRC or relinked to or from it.
func TestLibraryMatchesReleases(t *testing.T) {
	end := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	ds := refdata.Generate(refdata.Config{Size: refdata.Tiny, Seed: 3, End: end})
	d := openDB(t, t.TempDir())
	loadRefdata(t, d, ds)

	var extras []string
	for _, s := range ds.Library.Tracks {
		if int(s.Item) >= len(ds.Tracks) && s.Present() && s.AddedAt > 0 {
			extras = append(extras, ds.LibTrack(s.Item).ID)
		}
	}
	if len(extras) < 3 {
		t.Fatalf("only %d never-played saved tracks", len(extras))
	}
	byISRC, relinkedTo, relinkedFrom := extras[0], extras[1], extras[2]
	// Three different played tracks, one per case.
	var played []string
	for _, p := range ds.Plays {
		if id := ds.Tracks[p.Track].ID; !slices.Contains(played, id) {
			played = append(played, id)
		}
		if len(played) == 3 {
			break
		}
	}
	ctx := context.Background()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE tracks SET isrc = 'GBXXX0000001' WHERE id IN (?, ?)`, []any{byISRC, played[0]}},
		// Spotify relinked the saved track to the one played.
		{`UPDATE tracks SET linked_from = ? WHERE id = ?`, []any{relinkedTo, played[1]}},
		// The saved track is the relinked version of the one played.
		{`UPDATE tracks SET linked_from = ? WHERE id = ?`, []any{played[2], relinkedFrom}},
	} {
		if _, err := d.Write.ExecContext(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, d, live.Options{DefaultTimezone: ds.Timezone, Now: func() time.Time { return end }})
	plays := func(id string) any {
		return h.call("GET", "/tracks/{id}", "/tracks/"+id+"?range=all&metric=plays", nil, 200)["all_time"].(map[string]any)["plays"]
	}
	got := map[string]any{}
	for _, it := range h.call("GET", "/library/tracks", "/library/tracks?limit=500", nil, 200)["items"].([]any) {
		m := it.(map[string]any)
		got[m["track"].(map[string]any)["id"].(string)] = m
	}
	for id, want := range map[string]any{byISRC: plays(played[0]), relinkedTo: plays(played[1]), relinkedFrom: plays(relinkedFrom)} {
		m, ok := got[id].(map[string]any)
		if !ok {
			t.Fatalf("%s not listed", id)
		}
		if m["status"] != "played" || m["plays"] != want || want == 0.0 {
			t.Errorf("%s: status %v plays %v, want played %v", id, m["status"], m["plays"], want)
		}
	}
}

// TestUnplayedEntityPages checks that an album, song and artist saved but
// never played open with zero stats, scoped lists included, instead of
// "not in your library".
func TestUnplayedEntityPages(t *testing.T) {
	end := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	ds := refdata.Generate(refdata.Config{Size: refdata.Tiny, Seed: 3, End: end})
	d := openDB(t, t.TempDir())
	loadRefdata(t, d, ds)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO artists (id, name, image_url) VALUES ('unplayedArtist0000000', 'Nobody Heard', 'https://img/a.jpg')`,
		`INSERT INTO albums (id, name, album_type, release_date, total_tracks, image_url)
			VALUES ('unplayedAlbum00000000', 'Revolver, 2026', 'album', '2026-05-01', 14, 'https://img/al.jpg')`,
		`INSERT INTO album_artists (album_id, artist_id, position) VALUES ('unplayedAlbum00000000', 'unplayedArtist0000000', 0)`,
		`INSERT INTO tracks (id, name, duration_ms, explicit, album_id, source, hydrated_at)
			VALUES ('unplayedTrack00000000', 'Taxman, 2026', 150000, 0, 'unplayedAlbum00000000', 'library', 1)`,
		`INSERT INTO track_artists (track_id, artist_id, position) VALUES ('unplayedTrack00000000', 'unplayedArtist0000000', 0)`,
	} {
		if _, err := d.Write.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, d, live.Options{DefaultTimezone: ds.Timezone, Now: func() time.Time { return end }})
	const q = "?range=all&metric=plays"
	al := h.call("GET", "/albums/{id}", "/albums/unplayedAlbum00000000"+q, nil, 200)
	if a := al["album"].(map[string]any); a["name"] != "Revolver, 2026" || a["total_tracks"] != 14.0 ||
		a["artists"].([]any)[0].(map[string]any)["name"] != "Nobody Heard" {
		t.Errorf("album = %v", a)
	}
	if al["all_time"].(map[string]any)["plays"] != 0.0 || al["in_range"].(map[string]any)["rank"] != nil {
		t.Errorf("album stats = %v %v", al["all_time"], al["in_range"])
	}
	tr := h.call("GET", "/tracks/{id}", "/tracks/unplayedTrack00000000"+q, nil, 200)
	if x := tr["track"].(map[string]any); x["name"] != "Taxman, 2026" || x["album"].(map[string]any)["name"] != "Revolver, 2026" {
		t.Errorf("track = %v", x)
	}
	ar := h.call("GET", "/artists/{id}", "/artists/unplayedArtist0000000"+q, nil, 200)
	if ar["artist"].(map[string]any)["name"] != "Nobody Heard" || ar["all_time"].(map[string]any)["plays"] != 0.0 {
		t.Errorf("artist = %v", ar)
	}
	for _, s := range []string{"album_id=unplayedAlbum00000000", "track_id=unplayedTrack00000000", "artist_id=unplayedArtist0000000"} {
		top := h.call("GET", "/top/tracks", "/top/tracks"+q+"&"+s, nil, 200)
		if n := len(top["items"].([]any)); n != 0 && !strings.HasPrefix(s, "track_id") {
			t.Errorf("%s: %d top tracks, want 0", s, n)
		}
		h.call("GET", "/timeline", "/timeline"+q+"&"+s, nil, 200)
		h.call("GET", "/clock", "/clock"+q+"&"+s, nil, 200)
		h.call("GET", "/history", "/history?range=all&"+s, nil, 200)
		h.call("GET", "/contexts", "/contexts"+q+"&"+s, nil, 200)
	}
	h.call("GET", "/albums/{id}", "/albums/nosuchAlbum0000000000"+q, nil, 404)
}
