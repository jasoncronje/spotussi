package live_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/db"
	"spotussi/internal/live"
	"spotussi/internal/refdata"
)

// BenchmarkLive measures the M5/M6 budgets (plan.md): full rebuild
// < 10 s, dashboard endpoints p95 < 25 ms, History p95 < 50 ms. It runs
// on the full reference dataset (~500k rows), or on the database in
// $SPOTUSSI_BENCH_DATA_DIR (an instance's DATA_DIR with a real import;
// never committed, DF4):
//
//	go test -run '^$' -bench Live -benchtime 1x ./internal/live
//
// Each endpoint reports p50/p95 over 50 requests (ms). Only timings are
// reported; no row contents are printed.
//
// CI (M8) sets SPOTUSSI_BENCH_BUDGET_FACTOR to fail the run when a
// measure exceeds its budget times that factor (shared runners are slower
// and noisier than a workstation). An endpoint over budget is measured
// once more before it fails, so one GC pause or noisy neighbour doesn't.
// SPOTUSSI_BENCH_SUMMARY names a file that gets a Markdown table of the
// results appended (the CI job summary).
func BenchmarkLive(b *testing.B) {
	factor := 0.0
	if v := os.Getenv("SPOTUSSI_BENCH_BUDGET_FACTOR"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			b.Fatalf("SPOTUSSI_BENCH_BUDGET_FACTOR must be a positive number, got %q", v)
		}
		factor = f
	}
	var summary []string
	check := func(tb testing.TB, name string, got, budget time.Duration) {
		limit := budgetLimit(budget, factor)
		verdict := "ok"
		if got > limit {
			verdict = "over"
		}
		summary = append(summary, fmt.Sprintf("| %s | %s | %s | %s | %s |", name, fmtDur(got), fmtDur(budget), fmtDur(limit), verdict))
		fmt.Printf("budget %-10s %10s  (budget %s, limit %s) %s\n", name, fmtDur(got), fmtDur(budget), fmtDur(limit), verdict)
		if factor > 0 && got > limit {
			tb.Errorf("%s: %s exceeds %s (budget %s × %g)", name, fmtDur(got), fmtDur(limit), fmtDur(budget), factor)
		}
	}
	defer func() {
		path := os.Getenv("SPOTUSSI_BENCH_SUMMARY")
		if path == "" || len(summary) == 0 {
			return
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			b.Error(err)
			return
		}
		defer f.Close()
		fmt.Fprintf(f, "### Performance budgets (%s, %s/%s, %d CPUs)\n\n| measure | result | budget | limit | |\n|---|---|---|---|---|\n%s\n\n",
			runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), strings.Join(summary, "\n"))
	}()
	ctx := context.Background()
	var d *db.DB
	var err error
	now := time.Now()
	if dir := os.Getenv("SPOTUSSI_BENCH_DATA_DIR"); dir != "" {
		d, err = db.Open(ctx, dir)
	} else {
		now = time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
		ds := refdata.Generate(refdata.Config{Size: refdata.Full, Seed: 1, End: now})
		d, err = db.Open(ctx, b.TempDir())
		if err == nil {
			err = writeRefdata(ctx, d, ds, 0, len(ds.Plays))
		}
	}
	if err != nil {
		b.Fatal(err)
	}
	defer d.Close()
	be := live.New(live.Options{DB: d, DefaultTimezone: "Europe/London", Now: func() time.Time { return now }})
	h := api.NewHandler(api.Options{Version: "b", Mode: "live", Backend: be})

	b.Run("rebuild", func(b *testing.B) {
		fastest := time.Duration(0)
		for b.Loop() {
			start := time.Now()
			if err := be.Engine().Rebuild(ctx); err != nil {
				b.Fatal(err)
			}
			if d := time.Since(start); fastest == 0 || d < fastest {
				fastest = d
			}
		}
		check(b, "rebuild", fastest, 10*time.Second)
	})
	if be.Engine().Status().LastRebuildAt == 0 { // -bench filtered the rebuild out
		if err := be.Engine().Rebuild(ctx); err != nil {
			b.Fatal(err)
		}
	}
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	if path := os.Getenv("SPOTUSSI_BENCH_HEAP"); path != "" { // heap profile with the snapshot live
		f, err := os.Create(path)
		if err != nil {
			b.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
	}
	fmt.Printf("after rebuild: live heap %.1f MB, heap in use %.1f MB, sys %.1f MB\n", float64(ms.HeapAlloc)/1e6,
		float64(ms.HeapInuse)/1e6, float64(ms.Sys)/1e6)

	// IDs for scoped requests, from the data itself.
	first := func(path, field string) string {
		out := benchGet(b, h, path)
		items, _ := out["items"].([]any)
		if len(items) == 0 {
			return ""
		}
		v, _ := dig(items[0], strings.Split(field, ".")...).(string)
		return v
	}
	artist := first("/top/artists?range=all", "artist.id")
	album := first("/top/albums?range=all", "album.id")
	track := first("/top/tracks?range=all", "track.id")
	genre := first("/top/genres?range=all", "genre.id")
	session := first("/sessions?range=all", "id")
	playlist := first("/library/playlists?sort=name", "playlist.id")
	cursor := ""
	page := benchGet(b, h, "/history?range=all&limit=50")
	for range 50 { // a cursor 50 pages deep
		c, _ := page["next_cursor"].(string)
		if c == "" {
			break
		}
		cursor = c
		page = benchGet(b, h, "/history?range=all&limit=50&cursor="+url.QueryEscape(c))
	}

	dashboard := []string{
		"/overview/summary?range=all", "/overview/summary?range=this_year", "/overview/summary?range=last_4w",
		"/timeline?range=all", "/timeline?range=all&bucket=day", "/timeline?range=this_year&compare=true",
		"/clock?range=all", "/top/tracks?range=all", "/top/tracks?range=all&metric=time", "/top/tracks?range=last_4w",
		"/top/artists?range=all", "/top/albums?range=all", "/top/genres?range=all",
		"/stats/hourly-top?range=all", "/stats/artists-over-time?range=all", "/stats/genres-over-time?range=all",
		"/stats/release-years?range=all", "/stats/artists-per-track?range=all", "/contexts?range=all",
		"/devices?range=all", "/sessions?range=all", "/search?q=the",
		"/artists/" + artist + "?range=all", "/albums/" + album + "?range=all", "/tracks/" + track + "?range=all",
		"/genres/" + genre + "?range=all", "/sessions/" + session, "/top/tracks?range=all&artist_id=" + artist,
		"/timeline?range=all&genre_id=" + url.QueryEscape(genre), "/system/status",
		"/library/summary", "/library/timeline?range=all&bucket=month", "/library/tracks?played=never_played",
		"/library/tracks?sort=plays", "/library/albums?sort=played_share", "/library/playlists",
		"/library/playlists/" + playlist,
	}
	history := []string{
		"/history?range=all&limit=50", "/history?range=all&limit=50&cursor=" + url.QueryEscape(cursor),
		"/history?range=last_4w", "/history?range=this_year", "/history?from=2019-01-01&to=2019-12-31",
		"/history?range=all&counted=false", "/history?range=all&skipped=true", "/history?range=all&shuffle=false",
		"/history?range=all&platform=android", "/history?range=all&track_id=" + track,
		"/history?range=all&artist_id=" + artist, "/history?range=all&genre_id=" + url.QueryEscape(genre),
	}
	// SPOTUSSI_BENCH_ONLY limits the endpoints to paths containing it
	// (profiling one endpoint).
	only := os.Getenv("SPOTUSSI_BENCH_ONLY")
	reps := 50
	if only != "" {
		reps = 500
	}
	report := func(name string, paths []string, budget time.Duration) {
		paths = slices.DeleteFunc(slices.Clone(paths), func(p string) bool {
			// Scoped requests need an ID from the data (an unhydrated
			// catalog has no genres).
			empty := strings.Contains(p, "/?") || strings.HasSuffix(p, "=") || strings.Contains(p, "=&")
			return empty || !strings.Contains(p, only)
		})
		if len(paths) == 0 {
			return
		}
		b.Run(name, func(b *testing.B) {
			var worst time.Duration
			worstPath := ""
			measure := func(p string) (p50, p95 time.Duration) {
				var d []time.Duration
				req := httptest.NewRequest("GET", "/api/v1"+p, nil)
				for range reps { // the server's time only: no client decoding
					rec := httptest.NewRecorder()
					start := time.Now()
					h.ServeHTTP(rec, req)
					d = append(d, time.Since(start))
					if rec.Code != 200 {
						b.Fatalf("%s: %d", redact(p), rec.Code)
					}
				}
				slices.Sort(d)
				return d[len(d)/2], d[len(d)*95/100]
			}
			limit := budgetLimit(budget, factor)
			for b.Loop() {
				for _, p := range paths {
					p50, p95 := measure(p)
					if p95 > limit {
						fmt.Printf("%-70s p95 %6.2f ms over the limit; measuring again\n", redact(p), float64(p95.Microseconds())/1000)
						p50, p95 = measure(p)
					}
					if p95 > worst {
						worst, worstPath = p95, p
					}
					fmt.Printf("%-70s p50 %6.2f ms  p95 %6.2f ms\n", redact(p), float64(p50.Microseconds())/1000, float64(p95.Microseconds())/1000)
				}
			}
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			fmt.Printf("%s: %d GCs, %.1f MB allocated per request\n", name, after.NumGC-ms.NumGC,
				float64(after.TotalAlloc-ms.TotalAlloc)/1e6/float64(len(paths)*reps))
			b.ReportMetric(float64(worst.Microseconds())/1000, "worst-p95-ms")
			if worst > budget {
				b.Logf("over budget (%v): %s at %v", budget, redact(worstPath), worst)
			}
			check(b, name+" p95", worst, budget)
		})
	}
	report("dashboard", dashboard, 25*time.Millisecond)
	report("history", history, 50*time.Millisecond)
}

// budgetLimit is budget × factor, or budget itself without a factor.
func budgetLimit(budget time.Duration, factor float64) time.Duration {
	if factor <= 0 {
		return budget
	}
	return time.Duration(float64(budget) * factor)
}

func fmtDur(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%.2f s", d.Seconds())
	}
	return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000)
}

// redact keeps entity IDs (real Spotify IDs on a real import) out of the
// report.
func redact(p string) string {
	path, q, _ := strings.Cut(p, "?")
	parts := strings.Split(path, "/")
	if len(parts) == 3 && parts[1] != "top" && parts[1] != "stats" && parts[1] != "overview" {
		parts[2] = "{id}"
	}
	vals, _ := url.ParseQuery(q)
	for k := range vals {
		if strings.HasSuffix(k, "_id") || k == "cursor" {
			vals.Set(k, "…")
		}
	}
	if len(vals) == 0 {
		return strings.Join(parts, "/")
	}
	s, _ := url.QueryUnescape(vals.Encode())
	return strings.Join(parts, "/") + "?" + s
}

func benchGet(b *testing.B, h http.Handler, path string) map[string]any {
	b.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1"+path, nil))
	if rec.Code != 200 {
		b.Fatalf("%s: %d %.200s", redact(path), rec.Code, rec.Body)
	}
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}
