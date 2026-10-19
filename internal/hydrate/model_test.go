package hydrate_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"spotussi/internal/hydrate"
)

// TestModelLifetimeExport replays the shape of a large lifetime export through
// the real worker and the fake, and reports how many requests it takes
// until the most-played 1,000 tracks are hydrated and until the queue is
// empty. Divide by the request rate for wall time.
//
// Shape (rounded aggregate counts from a real ~300k-row export,
// 2026-10-18): ~34,000 unique track URIs, ~10,000 primary artists, the top
// 1,000 tracks by about 500 primary artists; a quarter of tracks credit a second
// artist (an estimate). By default the model runs at 1/20 scale (top
// 50); HYDRATE_MODEL=full runs it at full size (~45k requests, about a
// minute).
func TestModelLifetimeExport(t *testing.T) {
	tracks, top, newEarly, newLate := 1706, 50, 0.49, 0.29
	if os.Getenv("HYDRATE_MODEL") == "full" {
		tracks, top = 34000, 1000
	} else if testing.Short() {
		t.Skip("model run")
	}
	e := newEnv(t)
	rng := rand.New(rand.NewPCG(1, 2))
	// Primary artists: a new one with probability newEarly among the top
	// tracks, newLate after; otherwise a reuse weighted by tracks so far.
	var credits []int // track -> primary artist, by rank
	artists := 0
	for i := range tracks {
		p := newLate
		if i < top {
			p = newEarly
		}
		if artists == 0 || rng.Float64() < p {
			credits = append(credits, artists)
			artists++
		} else {
			credits = append(credits, credits[rng.IntN(len(credits))])
		}
	}
	feature := make([]int, tracks) // -1: none
	for i := range feature {
		feature[i] = -1
		if rng.Float64() < 0.25 {
			feature[i] = rng.IntN(artists)
		}
	}
	ids := make(map[string]int, tracks)
	tx, _ := e.db.Write.Begin()
	for i := range tracks {
		id := fmt.Sprintf("t%05d", i)
		ids[id] = i
		// Priority falls with rank like a real export's plays (~46 at rank 1,000).
		prio := int64(46_000 / (i + 1))
		if _, err := tx.Exec(`INSERT INTO hydration_queue (entity_type, id, priority) VALUES ('track', ?, ?)`, id, prio); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	e.fake.SetArtistsOf(func(id string) []string {
		i, ok := ids[id]
		if !ok {
			return []string{"a-" + id}
		}
		out := []string{fmt.Sprintf("a%05d", credits[i])}
		if f := feature[i]; f >= 0 && f != credits[i] {
			out = append(out, fmt.Sprintf("a%05d", f))
		}
		return out
	})

	h := e.hydrator(hydrate.Options{Workers: 4})
	start := time.Now()
	e.run(h)
	deadline := time.Now().Add(5 * time.Minute)
	for e.int(`SELECT COUNT(*) FROM hydration_queue`) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("model did not finish")
		}
		time.Sleep(50 * time.Millisecond)
	}
	took := time.Since(start)

	log := e.fake.CatalogLog()
	lastTop, distinctTopArtists := 0, map[string]bool{}
	for n, p := range log {
		var id string
		if _, err := fmt.Sscanf(p, "/v1/tracks/%s", &id); err == nil {
			if ids[id] < top {
				lastTop = n + 1
			}
		}
	}
	for i := range top {
		distinctTopArtists[fmt.Sprint(credits[i])] = true
	}
	if lastTop > top*2 {
		t.Errorf("top %d tracks took %d requests: artists or the long tail are crowding them out", top, lastTop)
	}
	total := len(log)
	t.Logf("model: %d tracks, %d artists (%d primary among the top %d)", tracks, e.int(`SELECT COUNT(*) FROM artists`),
		len(distinctTopArtists), top)
	t.Logf("requests until the top %d tracks are hydrated: %d; until the queue is empty: %d (fake ran them in %s)",
		top, lastTop, total, took.Round(time.Millisecond))
	for _, rate := range []float64{1, 2, 4, 8} {
		t.Logf("at %g req/s: top %d in %s, everything in %s", rate, top,
			time.Duration(float64(lastTop)/rate*float64(time.Second)).Round(time.Second),
			time.Duration(float64(total)/rate*float64(time.Second)).Round(time.Minute))
	}
	if st, _ := h.Status(context.Background()); st.TracksHydrated != int64(tracks) {
		t.Errorf("hydrated %d of %d tracks", st.TracksHydrated, tracks)
	}
}
