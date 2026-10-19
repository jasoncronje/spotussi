//go:build measure

// Measures the fallback sources (D27) against the real oEmbed, Deezer and
// MusicBrainz APIs on an imported export. It makes no Spotify Web API
// calls (no Spotify client at all). It needs a throwaway data directory
// with an export imported (`spotussi import`), whose queue it trims to
// the top MEASURE_TOP tracks by plays plus MEASURE_TAIL random ones from
// the rest. It prints aggregate rates only; MEASURE_SPOTCHECK names a
// file that receives 30 random Deezer matches for a manual check (keep
// it out of the repo and delete it afterwards).
//
//	MEASURE_DATA_DIR=/tmp/x mise exec -- go test -tags measure -run Measure -v -timeout 3h ./internal/hydrate/
package hydrate_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/hydrate"
	"spotussi/internal/metadata"
)

func TestMeasureFallbacks(t *testing.T) {
	dir := os.Getenv("MEASURE_DATA_DIR")
	if dir == "" {
		t.Skip("MEASURE_DATA_DIR not set")
	}
	top, tail := envInt("MEASURE_TOP", 1000), envInt("MEASURE_TAIL", 1000)
	ctx := context.Background()
	d, err := db.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ua := metadata.UserAgent("measure", "")
	fb := hydrate.Fallbacks{OEmbed: metadata.NewOEmbed("", ua, nil, nil), Deezer: metadata.NewDeezer("", ua, nil, nil),
		MusicBrainz: metadata.NewMusicBrainz("", ua, nil, nil), Always: true}
	h := hydrate.New(hydrate.Options{DB: d, Fallbacks: fb})
	if _, err := h.Plan(ctx); err != nil {
		t.Fatal(err)
	}
	var total int64
	d.Read.QueryRow(`SELECT COUNT(*) FROM hydration_queue WHERE entity_type = 'track'`).Scan(&total)
	// Keep the sample: top by priority, random from the rest.
	ids := func(q string, args ...any) []string {
		rs, err := d.Read.Query(q, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rs.Close()
		var out []string
		for rs.Next() {
			var s string
			rs.Scan(&s)
			out = append(out, s)
		}
		return out
	}
	all := ids(`SELECT id FROM hydration_queue WHERE entity_type = 'track' ORDER BY priority DESC, id`)
	topIDs := all[:min(top, len(all))]
	rest := append([]string(nil), all[len(topIDs):]...)
	r := rand.New(rand.NewPCG(1, 2))
	r.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	tailIDs := rest[:min(tail, len(rest))]
	keep := map[string]string{}
	for _, id := range topIDs {
		keep[id] = "top"
	}
	for _, id := range tailIDs {
		keep[id] = "tail"
	}
	tx, _ := d.Write.Begin()
	tx.Exec(`CREATE TEMP TABLE keep (id TEXT PRIMARY KEY)`)
	for id := range keep {
		tx.Exec(`INSERT INTO keep VALUES (?)`, id)
	}
	tx.Exec(`DELETE FROM hydration_queue WHERE entity_type <> 'track' OR id NOT IN (SELECT id FROM keep)`)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	t.Logf("export: %d queued tracks; sample %d top + %d tail", total, len(topIDs), len(tailIDs))

	// Run the three stages like the worker does, each in its own loop.
	start := time.Now()
	elapsed := map[string]time.Duration{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	deezerDone := make(chan struct{})
	for _, src := range hydrate.FallbackSources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s0 := time.Now()
			for {
				n, err := h.FallbackStep(ctx, src)
				if err != nil {
					t.Logf("%s: %v", src, err)
					time.Sleep(10 * time.Second)
					continue
				}
				if n == 0 {
					if src != metadata.SourceMusicBrainz {
						break
					}
					select {
					case <-deezerDone:
						if n2, _ := h.FallbackStep(ctx, src); n2 == 0 {
							goto done
						}
					case <-time.After(5 * time.Second):
					}
				}
			}
		done:
			mu.Lock()
			elapsed[src] = time.Since(s0)
			mu.Unlock()
			if src == metadata.SourceDeezer {
				close(deezerDone)
			}
			t.Logf("%s finished after %s", src, time.Since(s0).Round(time.Second))
		}()
	}
	wg.Wait()
	t.Logf("all stages: %s", time.Since(start).Round(time.Second))

	// Aggregates.
	reqs := map[string]int64{metadata.SourceOEmbed: fb.OEmbed.Requests(), metadata.SourceDeezer: fb.Deezer.Requests(),
		metadata.SourceMusicBrainz: fb.MusicBrainz.Requests()}
	for _, src := range hydrate.FallbackSources {
		for _, part := range []string{"top", "tail"} {
			var matched, unmatched, failed int64
			for id, p := range keep {
				if p != part {
					continue
				}
				var st string
				d.Read.QueryRow(`SELECT status FROM fallback_state WHERE source = ? AND entity_type = 'track' AND id = ?`, src, id).Scan(&st)
				switch st {
				case "matched":
					matched++
				case "no_match":
					unmatched++
				default:
					failed++
				}
			}
			n := matched + unmatched + failed
			t.Logf("%-12s %-4s matched %4d/%d (%.1f%%), no match %d, failed/unsettled %d", src, part, matched, n,
				100*float64(matched)/float64(max(n, 1)), unmatched, failed)
		}
		el := elapsed[src]
		t.Logf("%-12s %d requests in %s: %.2f req/s, %.2f tracks/s", src, reqs[src], el.Round(time.Second),
			float64(reqs[src])/el.Seconds(), float64(len(keep))/el.Seconds())
		// Confidence histogram of matched entities, by type.
		rs, _ := d.Read.Query(`SELECT entity_type, CAST(confidence * 20 AS INTEGER) / 20.0 AS b, COUNT(*)
			FROM fallback_state WHERE source = ? AND status = 'matched' GROUP BY 1, 2 ORDER BY 1, 2`, src)
		var lines []string
		for rs.Next() {
			var typ string
			var b float64
			var c int
			rs.Scan(&typ, &b, &c)
			lines = append(lines, fmt.Sprintf("%s %.2f:%d", typ, b, c))
		}
		rs.Close()
		t.Logf("%-12s confidence (bucket start:count) %s", src, strings.Join(lines, "  "))
		var am, an, alm, aln int64
		d.Read.QueryRow(`SELECT COALESCE(SUM(entity_type = 'artist' AND status = 'matched'), 0), COALESCE(SUM(entity_type = 'artist' AND status = 'no_match'), 0),
			COALESCE(SUM(entity_type = 'album' AND status = 'matched'), 0), COALESCE(SUM(entity_type = 'album' AND status = 'no_match'), 0)
			FROM fallback_state WHERE source = ?`, src).Scan(&am, &an, &alm, &aln)
		t.Logf("%-12s artists matched %d / no match %d; albums matched %d / no match %d", src, am, an, alm, aln)
	}
	var withDur, withGenres, withArtistImg, withCover int64
	d.Read.QueryRow(`SELECT COUNT(*) FROM tracks WHERE source = 'fallback' AND duration_ms IS NOT NULL`).Scan(&withDur)
	d.Read.QueryRow(`SELECT COUNT(DISTINCT artist_id) FROM artist_genres WHERE artist_id LIKE 'ar_%'`).Scan(&withGenres)
	d.Read.QueryRow(`SELECT COUNT(*) FROM fallback_artists WHERE image_url IS NOT NULL`).Scan(&withArtistImg)
	d.Read.QueryRow(`SELECT COUNT(*) FROM fallback_albums WHERE image_url IS NOT NULL`).Scan(&withCover)
	t.Logf("result: %d tracks with a duration, %d artists with genres, %d with a picture, %d albums with a cover",
		withDur, withGenres, withArtistImg, withCover)

	if f := os.Getenv("MEASURE_SPOTCHECK"); f != "" {
		rs, _ := d.Read.Query(`SELECT f.id, f.confidence, f.external_id FROM fallback_state f
			WHERE f.source = 'deezer' AND f.entity_type = 'track' AND f.status = 'matched' ORDER BY random() LIMIT 30`)
		var b strings.Builder
		for rs.Next() {
			var id, ext string
			var c float64
			rs.Scan(&id, &c, &ext)
			var tn, an, al string
			var exp, dur int64
			d.Read.QueryRow(`SELECT track_name, artist_name, album_name FROM raw_export_rows WHERE uri = ? AND track_name IS NOT NULL
				ORDER BY ts_ms DESC LIMIT 1`, "spotify:track:"+id).Scan(&tn, &an, &al)
			d.Read.QueryRow(`SELECT COALESCE(duration_ms, 0) FROM tracks WHERE id = ?`, id).Scan(&dur)
			d.Read.QueryRow(`SELECT COALESCE(MAX(ms_played), 0) FROM raw_export_rows WHERE uri = ?`, "spotify:track:"+id).Scan(&exp)
			fmt.Fprintf(&b, "%.2f\t%s | %s | %s\tdeezer=%s dur=%d maxplayed=%d\n", c, tn, an, al, ext, dur, exp)
		}
		rs.Close()
		os.WriteFile(f, []byte(b.String()), 0o600)
	}
}

func envInt(k string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n > 0 {
		return n
	}
	return def
}

// TestMeasureRematch measures a matcher change (metadata.MatcherVersion)
// on a data directory TestMeasureFallbacks ran Deezer on with the older
// matcher: Rematch re-queues Deezer's unmatched tracks, Deezer alone runs
// them again, and it prints how many now match. MEASURE_SPOTCHECK gets 30
// random new matches for a manual check, as above.
func TestMeasureRematch(t *testing.T) {
	dir := os.Getenv("MEASURE_DATA_DIR")
	if dir == "" {
		t.Skip("MEASURE_DATA_DIR not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	dz := metadata.NewDeezer("", metadata.UserAgent("measure", ""), nil, nil)
	h := hydrate.New(hydrate.Options{DB: d, Fallbacks: hydrate.Fallbacks{Deezer: dz, Always: true}})
	d.Write.Exec(`CREATE TABLE IF NOT EXISTS measure_before AS SELECT id, EXISTS (SELECT 1 FROM raw_export_rows r
		WHERE r.uri = 'spotify:track:' || f.id AND r.reason_end = 'trackdone' AND r.ms_played >= 30000) AS known
		FROM fallback_state f WHERE source = 'deezer' AND entity_type = 'track' AND status = 'no_match'`)
	var before, weak int64
	d.Write.QueryRow(`SELECT COUNT(*), COALESCE(SUM(NOT known), 0) FROM measure_before`).Scan(&before, &weak)
	n, err := h.Rematch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("old matcher: %d unmatched (%d without a stream that played to the end); re-queued %d", before, weak, n)
	start := time.Now()
	for {
		n, err := h.FallbackStep(ctx, metadata.SourceDeezer)
		if err != nil {
			t.Logf("deezer: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}
		if n == 0 {
			break
		}
	}
	var now, nowWeak, still int64
	d.Write.QueryRow(`SELECT COALESCE(SUM(f.status = 'matched'), 0), COALESCE(SUM(f.status = 'matched' AND NOT b.known), 0),
		COALESCE(SUM(f.status = 'no_match'), 0) FROM measure_before b
		JOIN fallback_state f ON f.source = 'deezer' AND f.entity_type = 'track' AND f.id = b.id`).Scan(&now, &nowWeak, &still)
	t.Logf("new matcher: %d of %d now match (%d of them without a completed stream), %d still unmatched; %d requests in %s",
		now, before, nowWeak, still, dz.Requests(), time.Since(start).Round(time.Second))
	if f := os.Getenv("MEASURE_SPOTCHECK"); f != "" {
		rs, _ := d.Write.Query(`SELECT f.id, f.confidence, f.external_id FROM measure_before b
			JOIN fallback_state f ON f.source = 'deezer' AND f.entity_type = 'track' AND f.id = b.id
			WHERE f.status = 'matched' ORDER BY random() LIMIT 30`)
		type row struct {
			id, ext string
			c       float64
		}
		var rows []row
		for rs.Next() {
			var r row
			rs.Scan(&r.id, &r.c, &r.ext)
			rows = append(rows, r)
		}
		rs.Close()
		var b strings.Builder
		for _, r := range rows {
			var tn, an, al string
			var dur, exp int64
			d.Read.QueryRow(`SELECT track_name, artist_name, album_name FROM raw_export_rows WHERE uri = ? AND track_name IS NOT NULL
				ORDER BY ts_ms DESC LIMIT 1`, "spotify:track:"+r.id).Scan(&tn, &an, &al)
			d.Read.QueryRow(`SELECT COALESCE(duration_ms, 0) FROM tracks WHERE id = ?`, r.id).Scan(&dur)
			d.Read.QueryRow(`SELECT COALESCE(MAX(ms_played), 0) FROM raw_export_rows WHERE uri = ?`, "spotify:track:"+r.id).Scan(&exp)
			fmt.Fprintf(&b, "%.2f\t%s | %s | %s\tdeezer=%s dur=%d maxplayed=%d\n", r.c, tn, an, al, r.ext, dur, exp)
		}
		os.WriteFile(f, []byte(b.String()), 0o600)
	}
}
