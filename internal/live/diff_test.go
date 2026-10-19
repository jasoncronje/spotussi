package live_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/live"
	"spotussi/internal/mock"
	"spotussi/internal/refdata"
)

// TestDifferential is the M6 correctness check. The reference dataset is
// written into a live database through the raw tables and catalog
// (loadRefdata). Derivation then rebuilds plays from scratch, and every
// endpoint answers on both backends. The mock reads the dataset directly
// and is the oracle (plan.md, M5/M6): responses must match exactly,
// apart from opaque history cursors. Live responses are also validated
// against the spec.
func TestDifferential(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the small reference dataset")
	}
	end := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	ds := refdata.Generate(refdata.Config{Size: diffSize, Seed: 1, End: end})
	clock := func() time.Time { return end }
	mh := api.NewHandler(api.Options{Version: "t", Mode: "mock", Backend: mock.NewWithClock(ds, clock)})

	d := openDB(t, t.TempDir())
	loadRefdata(t, d, ds)
	h := newHarness(t, d, live.Options{DefaultTimezone: ds.Timezone, Now: clock})
	dc := &diffChecker{t: t, h: h, mock: mh}

	// IDs to scope by, from the oracle.
	first := func(path, field string) string {
		var out map[string]any
		json.Unmarshal(serve(mh, "GET", path, nil).Body.Bytes(), &out)
		it := out["items"].([]any)[0].(map[string]any)
		return dig(it, strings.Split(field, ".")...).(string)
	}
	ids := map[string]string{
		"artist": first("/top/artists?range=all", "artist.id"), "album": first("/top/albums?range=all", "album.id"),
		"track": first("/top/tracks?range=all", "track.id"), "genre": first("/top/genres?range=all", "genre.id"),
		"session": first("/sessions?range=all", "id"),
		// A less played artist (tail, maybe unhydrated) too.
		"artist2": first(fmt.Sprintf("/top/artists?range=all&offset=%d&limit=1", len(ds.Artists)/2), "artist.id"),
		"track2":  first(fmt.Sprintf("/top/tracks?range=all&offset=%d&limit=1", len(ds.Tracks)/2), "track.id"),
	}
	playlist := ""
	for _, c := range ds.Contexts {
		if c.Type == "playlist" && c.SpotifyOwned && c.NameSource == "unresolved" {
			playlist = c.ID
			break
		}
	}

	dashboard := func(ranges []string) {
		for _, r := range ranges {
			for _, m := range []string{"plays", "time"} {
				q := "?" + strings.TrimPrefix(r+"&metric="+m, "&")
				for _, p := range []string{"/overview/summary", "/top/tracks", "/top/artists", "/top/albums", "/top/genres",
					"/stats/hourly-top", "/stats/artists-over-time", "/stats/genres-over-time", "/stats/release-years",
					"/stats/artists-per-track", "/contexts"} {
					dc.get(p, p+q)
				}
				for _, k := range []string{"artist", "album", "track", "genre"} {
					dc.get("/"+k+"s/{id}", "/"+k+"s/"+ids[k]+q)
				}
				dc.get("/artists/{id}", "/artists/"+ids["artist2"]+q)
				dc.get("/tracks/{id}", "/tracks/"+ids["track2"]+q)
			}
			q := "?" + r
			dc.get("/timeline", "/timeline"+q)
			dc.get("/timeline", "/timeline"+q+"&compare=true")
			dc.get("/clock", "/clock"+q)
			dc.get("/sessions", "/sessions"+q)
			dc.get("/devices", "/devices"+q)
			dc.get("/history", "/history"+q+"&limit=100")
		}
	}
	allRanges := []string{"", "range=today", "range=this_week", "range=this_month", "range=this_year",
		"range=last_7d", "range=last_4w", "range=last_6m", "range=last_12m", "range=prev_day",
		"range=prev_week", "range=prev_month", "range=prev_year", "range=all", "from=2025-02-01&to=2025-03-15",
		"from=2022-03-27&to=2022-03-27"} // a DST change in Europe/London
	dashboard(allRanges)
	for _, b := range []string{"hour", "day", "week", "month", "year"} {
		dc.get("/timeline", "/timeline?range=last_4w&bucket="+b)
		dc.get("/stats/genres-over-time", "/stats/genres-over-time?range=last_12m&series=3&bucket="+b)
		dc.get("/stats/artists-over-time", "/stats/artists-over-time?range=all&series=8&bucket="+b)
	}
	scopes := []string{"artist_id=" + ids["artist"], "album_id=" + ids["album"], "track_id=" + ids["track"],
		"genre_id=" + ids["genre"], "artist_id=" + ids["artist2"]}
	for _, sc := range scopes {
		for _, p := range []string{"/timeline", "/clock", "/contexts", "/top/tracks", "/top/albums", "/top/artists",
			"/overview/summary", "/stats/hourly-top"} {
			dc.get(p, p+"?range=all&"+sc)
		}
		dc.get("/history", "/history?limit=50&"+sc)
		dc.get("/timeline", "/timeline?range=this_month&compare=true&"+sc)
	}
	for _, q := range []string{"skipped=true", "skipped=false&shuffle=false", "counted=false", "counted=true&range=last_6m",
		"device=iPhone", "platform=ios", "shuffle=true&device=Kitchen", "range=all&limit=200&platform=android"} {
		dc.get("/history", "/history?"+q)
	}
	dc.get("/history", "/history?range=all&limit=500")
	dc.pages("/history?range=last_4w&limit=37")
	for _, p := range []string{"/top/tracks", "/top/artists", "/contexts", "/sessions"} {
		dc.get(p, p+"?range=all&offset=40&limit=25")
	}
	dc.get("/sessions/{id}", "/sessions/"+ids["session"])
	for _, q := range []string{"a", "the", "LOVE", "zz", "é"} {
		dc.get("/search", "/search?q="+q+"&limit=8")
	}
	dc.get("/exclusions", "/exclusions")
	dc.status()

	// Mutations: both backends see the same writes, then the views that
	// depend on them are compared again.
	dc.do("PUT", "/exclusions/{kind}/{id}", "/exclusions/artists/"+ids["artist"], nil)
	dc.do("PUT", "/exclusions/{kind}/{id}", "/exclusions/tracks/"+ids["track2"], nil)
	dc.do("PUT", "/exclusions/{kind}/{id}", "/exclusions/artists/"+ids["artist"], nil) // idempotent
	dc.do("PUT", "/playlists/{id}/name", "/playlists/"+playlist+"/name", map[string]any{"name": "Morning Mix"})
	dashboard([]string{"range=all", "range=last_12m"})
	dc.get("/artists/{id}", "/artists/"+ids["artist"]+"?range=all")
	dc.status()
	dc.do("PATCH", "/settings", "/settings", map[string]any{"play_rule": "fixed", "play_rule_threshold_ms": 45000,
		"session_gap_minutes": 40, "timezone": "America/New_York", "week_start": "sunday"})
	dashboard([]string{"range=all", "range=this_week", "range=last_7d", "from=2024-11-03&to=2024-11-03"})
	dc.status()
	dc.do("DELETE", "/exclusions/{kind}/{id}", "/exclusions/artists/"+ids["artist"], nil)
	dc.do("PUT", "/playlists/{id}/name", "/playlists/"+playlist+"/name", map[string]any{"name": nil})
	dashboard([]string{"range=all"})
	// The deferred rebuild after settings and exclusion changes lands on
	// the same answers.
	h.rebuild()
	dashboard([]string{"range=all", "range=last_4w"})
	dc.status()

	t.Logf("%d requests compared, %d differing", dc.n, dc.failed)
}

type diffChecker struct {
	t         *testing.T
	h         *harness
	mock      http.Handler
	n, failed int
}

func serve(h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1"+path, rd))
	return rec
}

func (dc *diffChecker) get(tmpl, path string) { dc.do("GET", tmpl, path, nil) }

// do sends the request to both backends and compares status and body.
func (dc *diffChecker) do(method, tmpl, path string, body any) (mockOut, liveOut any) {
	dc.t.Helper()
	dc.n++
	m := serve(dc.mock, method, path, body)
	l := serve(dc.h.h, method, path, body)
	if m.Code != l.Code {
		dc.fail("%s %s: status mock %d, live %d: %s", method, path, m.Code, l.Code, l.Body)
		return nil, nil
	}
	dc.h.spec.validate(dc.t, tmpl, strings.ToLower(method), l.Code, l.Body.Bytes())
	json.Unmarshal(m.Body.Bytes(), &mockOut)
	json.Unmarshal(l.Body.Bytes(), &liveOut)
	normalize(mockOut)
	normalize(liveOut)
	var diffs []string
	compare("", mockOut, liveOut, &diffs)
	if len(diffs) > 0 {
		dc.fail("%s %s: %d differences:\n  %s", method, path, len(diffs), strings.Join(diffs[:min(len(diffs), 8)], "\n  "))
	}
	return mockOut, liveOut
}

func (dc *diffChecker) fail(format string, args ...any) {
	dc.t.Helper()
	dc.failed++
	if dc.failed <= 40 {
		dc.t.Errorf(format, args...)
	}
}

// pages walks history page by page on both backends (cursors are opaque
// and differ, so each backend follows its own).
func (dc *diffChecker) pages(path string) {
	dc.t.Helper()
	mp, lp := path, path
	for range 30 {
		dc.n++
		var m, l map[string]any
		json.Unmarshal(serve(dc.mock, "GET", mp, nil).Body.Bytes(), &m)
		json.Unmarshal(serve(dc.h.h, "GET", lp, nil).Body.Bytes(), &l)
		mc, _ := m["next_cursor"].(string)
		lc, _ := l["next_cursor"].(string)
		normalize(m)
		normalize(l)
		var diffs []string
		compare("", m, l, &diffs)
		if len(diffs) > 0 {
			dc.fail("GET %s (paged): %s", lp, strings.Join(diffs[:min(len(diffs), 8)], "\n  "))
			return
		}
		if mc == "" {
			return
		}
		sep := "&"
		mp, lp = path+sep+"cursor="+mc, path+sep+"cursor="+lc
	}
}

// status compares the parts of system status both backends derive.
func (dc *diffChecker) status() {
	dc.t.Helper()
	dc.n++
	var m, l map[string]any
	json.Unmarshal(serve(dc.mock, "GET", "/system/status", nil).Body.Bytes(), &m)
	json.Unmarshal(serve(dc.h.h, "GET", "/system/status", nil).Body.Bytes(), &l)
	var diffs []string
	compare("data", m["data"], l["data"], &diffs)
	if len(diffs) > 0 {
		dc.fail("system status: %s", strings.Join(diffs, "\n  "))
	}
}

// normalize drops what legitimately differs: opaque cursors (only their
// presence matters) and the order of an artist's genres (catalog order
// vs generation order).
func normalize(v any) {
	switch x := v.(type) {
	case map[string]any:
		if c, ok := x["next_cursor"]; ok && c != nil {
			x["next_cursor"] = "…"
		}
		if gs, ok := x["genres"].([]any); ok && x["id"] != nil {
			sort.Slice(gs, func(i, j int) bool {
				return fmt.Sprint(gs[i].(map[string]any)["id"]) < fmt.Sprint(gs[j].(map[string]any)["id"])
			})
		}
		for _, c := range x {
			normalize(c)
		}
	case []any:
		for _, c := range x {
			normalize(c)
		}
	}
}

func compare(path string, a, b any, out *[]string) {
	if len(*out) > 50 {
		return
	}
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: mock %v, live %v", path, short(a), short(b)))
			return
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		for k := range y {
			if _, ok := x[k]; !ok {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		for _, k := range keys {
			compare(path+"."+k, x[k], y[k], out)
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			*out = append(*out, fmt.Sprintf("%s: mock len %d, live %v", path, len(x), short(b)))
			if ok {
				for i := range min(len(x), len(y)) {
					compare(fmt.Sprintf("%s[%d]", path, i), x[i], y[i], out)
				}
			}
			return
		}
		for i := range x {
			compare(fmt.Sprintf("%s[%d]", path, i), x[i], y[i], out)
		}
	default:
		if !reflect.DeepEqual(a, b) {
			*out = append(*out, fmt.Sprintf("%s: mock %v, live %v", path, short(a), short(b)))
		}
	}
}

func short(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 160 {
		return string(b[:160]) + "…"
	}
	return string(b)
}

// TestDifferentialIncremental checks the incremental path. Live rows of
// the last days arrive in batches after a full rebuild and are merged
// incrementally. The result must match the oracle, as a full rebuild
// does, including play and session IDs.
func TestDifferentialIncremental(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the small reference dataset")
	}
	end := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	ds := refdata.Generate(refdata.Config{Size: diffSize, Seed: 1, End: end})
	clock := func() time.Time { return end }
	mh := api.NewHandler(api.Options{Version: "t", Mode: "mock", Backend: mock.NewWithClock(ds, clock)})
	d := openDB(t, t.TempDir())
	ctx := t.Context()

	// Batches of 1, then the rest of the last two days; the last batch
	// arrives while the server is down.
	cut := sort.Search(len(ds.Plays), func(i int) bool { return ds.Plays[i].StartMs >= end.Add(-48*time.Hour).UnixMilli() })
	bounds := []int{cut, cut + 1, cut + 2, cut + 40, len(ds.Plays) - 3, len(ds.Plays)}
	for i := range bounds {
		bounds[i] = min(bounds[i], len(ds.Plays))
		if i > 0 {
			bounds[i] = max(bounds[i], bounds[i-1])
		}
	}
	if err := writeRefdata(ctx, d, ds, 0, bounds[0]); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, d, live.Options{DefaultTimezone: ds.Timezone, Now: clock})
	for k := 1; k < len(bounds); k++ {
		if err := writeRefdata(ctx, d, ds, bounds[k-1], bounds[k]); err != nil {
			t.Fatal(err)
		}
		if k < len(bounds)-1 {
			if err := h.be.Engine().Merge(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	// The restart loads the tables (the fingerprint ignores live rows),
	// then merges the live rows stored since.
	h2 := newRunningHarness(t, d, live.Options{DefaultTimezone: ds.Timezone, Now: clock})
	if st := h2.be.Engine().Status(); st.LastRebuildAt == 0 {
		t.Errorf("restart rebuilt instead of loading: %+v", st)
	}
	if err := h2.be.Engine().Merge(ctx); err != nil {
		t.Fatal(err)
	}
	dc := &diffChecker{t: t, h: h2, mock: mh}
	for _, r := range []string{"range=all", "range=last_7d", "range=today"} {
		for _, p := range []string{"/overview/summary", "/top/tracks", "/top/artists", "/timeline", "/clock", "/sessions",
			"/devices", "/contexts"} {
			dc.get(p, p+"?"+r)
		}
		dc.get("/history", "/history?limit=300&"+r)
	}
	dc.status()
	// The tail written to the plays table matches a fresh full rebuild.
	tail := func() string {
		rs, err := d.Read.Query(`SELECT group_concat(id || ':' || track_id || ':' || started_ms || ':' || ms_played || ':' ||
			sources || ':' || counts_as_play || ':' || COALESCE(session_id, '') || ':' || local_day, ',')
			FROM (SELECT * FROM plays WHERE started_ms >= ? ORDER BY id)`, end.Add(-72*time.Hour).UnixMilli())
		if err != nil {
			t.Fatal(err)
		}
		defer rs.Close()
		var s string
		for rs.Next() {
			rs.Scan(&s)
		}
		return s
	}
	inc := tail()
	h.rebuild()
	if full := tail(); full != inc || inc == "" {
		t.Errorf("plays table after incremental merges differs from a full rebuild")
	}
	t.Logf("%d requests compared, %d differing", dc.n, dc.failed)
}
