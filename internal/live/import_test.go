package live_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/capture"
	"spotussi/internal/importer"
	"spotussi/internal/live"
)

// Synthetic export rows only (DF4).
func exportRow(track string, end time.Time, msPlayed int64, artist, album, platform string, skipped bool) map[string]any {
	return map[string]any{"ts": end.UTC().Format(time.RFC3339), "platform": platform, "ms_played": msPlayed,
		"conn_country": "ZZ", "ip_addr": "192.0.2.1", "master_metadata_track_name": "Song " + track,
		"master_metadata_album_artist_name": artist, "master_metadata_album_album_name": album,
		"spotify_track_uri": "spotify:track:" + track, "episode_name": nil, "spotify_episode_uri": nil,
		"reason_start": "trackdone", "reason_end": "trackdone", "shuffle": true, "skipped": skipped,
		"offline": false, "incognito_mode": false}
}

func importRows(t *testing.T, h *harness, rows []map[string]any) {
	t.Helper()
	b, _ := json.Marshal(rows)
	p := filepath.Join(t.TempDir(), "Streaming_History_Audio_2026.json")
	os.WriteFile(p, b, 0o600)
	if _, _, err := importer.ImportPath(context.Background(), h.db, p, nil); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryWithExportRows(t *testing.T) {
	fc := &fakeCapture{status: capture.Status{State: capture.StateIdle}}
	h := newHarness(t, openDB(t, t.TempDir()), live.Options{DefaultTimezone: "UTC", Capture: fc,
		Now: func() time.Time { return now }})
	h.pollerPlay("a", -3*time.Hour, -3*time.Hour+200*time.Second, 200_000, false, "Kitchen")
	h.recentPlay("a", -3*time.Hour+200*time.Second+300*time.Millisecond)
	h.pollerPlay("b", -2*time.Hour, -2*time.Hour+20*time.Second, 20_000, true, "Phone")
	h.recentPlay("c", -1*time.Hour)
	episode := exportRow("x", now.Add(-10*time.Minute), 600_000, "", "", "android", false)
	episode["spotify_track_uri"], episode["spotify_episode_uri"] = nil, "spotify:episode:ep"
	old := time.Date(2015, 1, 1, 10, 0, 0, 0, time.UTC)
	importRows(t, h, []map[string]any{
		// The poller's play of a, as the export records it: merged.
		exportRow("a", now.Add(-3*time.Hour+205*time.Second), 199_000, "Artist", "Album", "ios", false),
		exportRow("e1", now.Add(-30*time.Minute), 180_000, "The Band", "Live", "android", false),
		exportRow("e2", now.Add(-20*time.Minute), 10_000, " the BAND", "Other", "android", true),
		episode,
		exportRow("old", old, 200_000, "Old", "Old", "windows", false),
	})
	h.rebuild()

	page := h.call("GET", "/history", "/history", nil, 200)
	if page["total"] != float64(6) {
		t.Fatalf("total: %v", page["total"])
	}
	items := page["items"].([]any)
	e2, e1, a, oldRow := items[0].(map[string]any), items[1].(map[string]any), items[4].(map[string]any), items[5].(map[string]any)
	if e2["played_at"] != float64(ms(-20*time.Minute-10*time.Second)) || e2["ended_at"] != float64(ms(-20*time.Minute)) ||
		e2["counts_as_play"] != false || e2["skipped"] != true || e2["platform"] != "android" || e2["ms_estimated"] != false ||
		dig(e2, "track", "id") != "e2" || dig(e2, "track", "name") != "Song e2" || e2["device"] != nil || e2["context"] != nil {
		t.Fatalf("export row: %v", e2)
	}
	if e1["counts_as_play"] != true || e1["shuffle"] != true || len(e1["sources"].([]any)) != 1 || e1["sources"].([]any)[0] != "export" {
		t.Fatalf("export row e1: %v", e1)
	}
	if srcs := a["sources"].([]any); len(srcs) != 3 || srcs[0] != "export" || a["platform"] != "ios" || a["id"] != "2" ||
		a["ms_played"] != float64(199_000) { // the export's fields win (D8)
		t.Fatalf("merged live row: %v", a)
	}
	if oldRow["played_at"] != float64(old.UnixMilli()-200_000) || dig(page, "range", "from") != "2015-01-01" ||
		dig(page, "range", "first_play_date") != "2015-01-01" {
		t.Fatalf("old row / range: %v %v", oldRow, page["range"])
	}
	// Provisional artists are keyed by normalized name.
	artist := dig(e1, "track", "artists").([]any)[0].(map[string]any)
	album := dig(e1, "track", "album").(map[string]any)
	if artist["id"] != dig(e2, "track", "artists").([]any)[0].(map[string]any)["id"] || artist["name"] != "The Band" ||
		album["id"] == dig(e2, "track", "album", "id") {
		t.Fatalf("provisional entities: %v / %v", e1["track"], e2["track"])
	}

	for q, want := range map[string]int{
		"platform=android": 2, "platform=ios": 1, "counted=true": 4, "counted=false": 2, "skipped=true": 2,
		"skipped=false": 4, "shuffle=true": 5, "device=Kitchen": 1, "track_id=e1": 1, "track_id=a": 1,
		"artist_id=" + artist["id"].(string): 2, "album_id=" + album["id"].(string): 1,
		"from=2015-01-01&to=2015-01-02": 1, "range=today": 5,
	} {
		p := h.call("GET", "/history", "/history?"+q, nil, 200)
		if p["total"] != float64(want) || len(p["items"].([]any)) != want {
			t.Errorf("%s: total %v (%d items), want %d", q, p["total"], len(p["items"].([]any)), want)
		}
	}
	// Unknown entities are 404s, as on the mock.
	h.call("GET", "/history", "/history?artist_id=nope", nil, 404)
	h.call("GET", "/history", "/history?album_id=al_!!", nil, 404)
	h.call("GET", "/history", "/history?genre_id=g", nil, 404)
	// A fixed rule applies to export rows too.
	h.call("PATCH", "/settings", "/settings", map[string]any{"play_rule": "fixed", "play_rule_threshold_ms": 5000}, 200)
	if p := h.call("GET", "/history", "/history?counted=true", nil, 200); p["total"] != float64(6) {
		t.Errorf("fixed rule: %v", p["total"])
	}

	// Paging across sources, at every page size, yields every row once in
	// order.
	var want []string
	for _, it := range items {
		want = append(want, it.(map[string]any)["id"].(string))
	}
	for _, size := range []int{1, 2, 4} {
		var seen []string
		path := "/history?limit=" + itoa(size)
		for range 10 {
			p := h.call("GET", "/history", path, nil, 200)
			seen = append(seen, ids(p)...)
			cur, _ := p["next_cursor"].(string)
			if cur == "" {
				break
			}
			path = "/history?limit=" + itoa(size) + "&cursor=" + cur
		}
		if len(seen) != len(want) {
			t.Fatalf("limit %d: %v, want %v", size, seen, want)
		}
		for i := range want {
			if seen[i] != want[i] {
				t.Fatalf("limit %d: %v, want %v", size, seen, want)
			}
		}
	}
	h.call("GET", "/history", "/history?cursor=1~x!", nil, 400)
	h.call("GET", "/history", "/history?cursor=1", nil, 400)

	// Derived rows: the episode is not music (D11); under the fixed rule
	// every row is a play.
	st := h.call("GET", "/system/status", "/system/status", nil, 200)
	if dig(st, "data", "rows") != float64(6) || dig(st, "data", "plays") != float64(6) ||
		dig(st, "data", "first_play_at") != float64(old.UnixMilli()-200_000) ||
		dig(st, "data", "last_play_at") != float64(ms(-20*time.Minute-10*time.Second)) {
		t.Fatalf("data status: %v", st["data"])
	}
	if imps := st["imports"].([]any); len(imps) != 1 || dig(imps[0], "state") != "done" || dig(imps[0], "rows_new") != float64(5) ||
		dig(imps[0], "origin") != "cli" {
		t.Fatalf("imports: %v", imps)
	}
}

func itoa(n int) string { return string(rune('0' + n)) }

func TestImportUpload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := openDB(t, t.TempDir())
	svc, err := importer.NewService(ctx, importer.Options{DB: d, Dir: filepath.Join(t.TempDir(), "imports")})
	if err != nil {
		t.Fatal(err)
	}
	go svc.Run(ctx)
	h := newHarness(t, d, live.Options{Imports: svc})
	h.h = api.NewHandler(api.Options{Version: "t", Mode: "live", Backend: h.be, ImportDir: svc.Dir(), ImportMaxBytes: 1 << 20})

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("Spotify Extended Streaming History/Streaming_History_Audio_2026.json")
	rows, _ := json.Marshal([]map[string]any{exportRow("a", now, 1000, "A", "B", "ios", false)})
	w.Write(rows)
	zw.Close()

	post := func(q, ct string, body []byte, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/v1/imports?"+q, bytes.NewReader(body))
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()
		h.h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("upload: %d %s", rec.Code, rec.Body)
		}
		h.spec.validate(t, "/imports", "post", rec.Code, rec.Body.Bytes())
		var out map[string]any
		json.Unmarshal(rec.Body.Bytes(), &out)
		return out
	}
	job := post("", "application/octet-stream", buf.Bytes(), 202)
	if job["filename"] != "upload.zip" || job["state"] != "queued" {
		t.Fatalf("job: %v", job)
	}
	var j map[string]any
	for range 200 {
		j = h.call("GET", "/imports/{id}", "/imports/"+job["id"].(string), nil, 200)
		if j["state"] == "done" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if j["state"] != "done" || j["rows_new"] != float64(1) || j["bytes_done"] != j["bytes_total"] || j["files"] != float64(1) {
		t.Fatalf("finished: %v", j)
	}
	// The same rows again, as a bare JSON file named by the client.
	job2 := post("filename=Streaming_History_Audio_2026.json", "", rows, 202)
	for range 200 {
		j = h.call("GET", "/imports/{id}", "/imports/"+job2["id"].(string), nil, 200)
		if j["state"] == "done" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if j["rows_new"] != float64(0) || j["rows_duplicate"] != float64(1) || j["filename"] != "Streaming_History_Audio_2026.json" {
		t.Fatalf("re-import: %v", j)
	}
	if l := h.call("GET", "/imports", "/imports", nil, 200); len(l["items"].([]any)) != 2 {
		t.Fatalf("list: %v", l)
	}
	h.call("GET", "/imports/{id}", "/imports/99", nil, 404)
	h.call("GET", "/imports/{id}", "/imports/x", nil, 404)
	post("", "text/plain", []byte("hello"), 415)
	post("", "application/zip", bytes.Repeat([]byte("x"), 1<<20+1), 413)
	if entries, _ := os.ReadDir(svc.Dir()); len(entries) != 0 {
		t.Fatalf("uploads left behind: %d", len(entries))
	}
}
