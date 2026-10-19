package live_test

import (
	"context"
	"slices"
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
