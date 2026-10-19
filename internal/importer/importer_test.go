package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"spotussi/internal/db"
)

// Synthetic fixtures only: no real export data in tests (DF4). The rows
// mimic the two field sets Spotify has used: old rows (username,
// ip_addr_decrypted, user_agent_decrypted; no audiobook or offline
// fields) and current rows (ip_addr, audiobook_*, offline*).

func openDB(t testing.TB) *db.DB {
	t.Helper()
	d, err := db.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

var base = time.Date(2019, 3, 1, 12, 0, 0, 0, time.UTC)

// newRow is a current-format track row ending i minutes after base.
func newRow(i int, track string, msPlayed int) map[string]any {
	return map[string]any{
		"ts": base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339), "platform": "ios", "ms_played": msPlayed,
		"conn_country": "ZZ", "ip_addr": "192.0.2.1",
		"master_metadata_track_name": "Song " + track, "master_metadata_album_artist_name": "Artist " + track,
		"master_metadata_album_album_name": "Album " + track, "spotify_track_uri": "spotify:track:" + track,
		"episode_name": nil, "episode_show_name": nil, "spotify_episode_uri": nil,
		"audiobook_title": nil, "audiobook_uri": nil, "audiobook_chapter_uri": nil, "audiobook_chapter_title": nil,
		"reason_start": "trackdone", "reason_end": "trackdone", "shuffle": false, "skipped": false,
		"offline": false, "offline_timestamp": nil, "incognito_mode": false,
	}
}

// oldRow is a 2014-era track row: no audiobook or offline fields, a
// null skipped, and the old IP and user-agent field names.
func oldRow(i int, track string) map[string]any {
	return map[string]any{
		"ts": base.Add(-time.Duration(i) * time.Hour).Format(time.RFC3339), "username": "someone", "platform": "Windows 7",
		"ms_played": 200_000, "conn_country": "ZZ", "ip_addr_decrypted": "198.51.100.7", "user_agent_decrypted": "unknown",
		"master_metadata_track_name": "Old " + track, "master_metadata_album_artist_name": "Old Artist",
		"master_metadata_album_album_name": "Old Album", "spotify_track_uri": "spotify:track:" + track,
		"episode_name": nil, "episode_show_name": nil, "spotify_episode_uri": nil,
		"reason_start": "clickrow", "reason_end": "endplay", "shuffle": true, "skipped": nil, "incognito_mode": false,
	}
}

func episodeRow(i int) map[string]any {
	r := newRow(i, "x", 600_000)
	for _, k := range []string{"master_metadata_track_name", "master_metadata_album_artist_name", "master_metadata_album_album_name", "spotify_track_uri"} {
		r[k] = nil
	}
	r["episode_name"], r["episode_show_name"], r["spotify_episode_uri"] = "Episode", "Show", "spotify:episode:ep1"
	return r
}

func audiobookRow(i int) map[string]any {
	r := newRow(i, "x", 900_000)
	for _, k := range []string{"master_metadata_track_name", "master_metadata_album_artist_name", "master_metadata_album_album_name", "spotify_track_uri"} {
		r[k] = nil
	}
	r["audiobook_title"], r["audiobook_uri"] = "Book", "spotify:show:book1"
	r["audiobook_chapter_uri"], r["audiobook_chapter_title"] = "spotify:episode:ch1", "Chapter 1"
	return r
}

// nullRow has no URI at all (local file or unknown content).
func nullRow(i int, name string) map[string]any {
	r := newRow(i, "x", 1000)
	r["spotify_track_uri"] = nil
	r["master_metadata_track_name"] = name
	return r
}

func encode(t testing.TB, rows []map[string]any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(rows, "", "  ") // the export is pretty-printed
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeZip(t testing.TB, files map[string][]byte) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "my_spotify_data.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func count(t testing.TB, d *db.DB, where string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := d.Read.QueryRow(`SELECT COUNT(*) FROM raw_export_rows WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestImportZipAndReimport(t *testing.T) {
	ctx := context.Background()
	d := openDB(t)
	audio := []map[string]any{newRow(1, "a", 200_000), newRow(2, "b", 5_000), episodeRow(3), audiobookRow(4),
		nullRow(5, "Local song"), nullRow(6, "Other local song"), newRow(1, "a", 200_000)} // last: duplicate
	old := []map[string]any{oldRow(1, "c"), oldRow(2, "d")}
	video := []map[string]any{episodeRow(10)}
	video[0]["spotify_episode_uri"] = "spotify:episode:vid1"
	p := writeZip(t, map[string][]byte{
		"Spotify Extended Streaming History/Streaming_History_Audio_2019.json":            encode(t, audio),
		"Spotify Extended Streaming History/Streaming_History_Audio_2013-2014.json":       encode(t, old),
		"Spotify Extended Streaming History/Streaming_History_Video_2019.json":            encode(t, video),
		"Spotify Extended Streaming History/ReadMeFirst_ExtendedStreamingHistory.pdf":     []byte("%PDF-1.4"),
		"__MACOSX/Spotify Extended Streaming History/._Streaming_History_Audio_2019.json": []byte("junk"),
	})

	var progress []Stats
	j, st, err := ImportPath(ctx, d, p, func(s Stats) { progress = append(progress, s) })
	if err != nil {
		t.Fatal(err)
	}
	if j.State != StateDone || j.Files != 3 || j.FilesDone != 3 || j.RowsDone != 10 || j.RowsNew != 9 || j.RowsDuplicate != 1 ||
		j.RowsTotal == nil || *j.RowsTotal != 10 || j.BytesTotal == nil || j.BytesDone != *j.BytesTotal || j.Origin != "cli" ||
		j.Kind != KindZip || j.StartedAt == nil || j.FinishedAt == nil || j.ErrorCode != "" {
		t.Fatalf("job: %+v", j)
	}
	if len(progress) == 0 {
		t.Error("no progress reported")
	}
	// Rows without any URI (local files) are "unknown".
	for ct, want := range map[string]int64{ContentTrack: 4, ContentEpisode: 2, ContentAudiobook: 1, ContentUnknown: 2} {
		if got := count(t, d, "content_type = ?", ct); got != want || st.NewByType[ct] != want {
			t.Errorf("%s rows: %d (stats %d), want %d", ct, got, st.NewByType[ct], want)
		}
	}
	if n := count(t, d, "video = 1 AND uri = 'spotify:episode:vid1'"); n != 1 {
		t.Errorf("video rows: %d", n)
	}
	// Extracted columns: end time, nullable flags, the verbatim JSON.
	var tsMs, msPlayed int64
	var skipped, offline any
	var raw, platform string
	if err := d.Read.QueryRow(`SELECT ts_ms, ms_played, skipped, offline, platform, json FROM raw_export_rows
		WHERE uri = 'spotify:track:c'`).Scan(&tsMs, &msPlayed, &skipped, &offline, &platform, &raw); err != nil {
		t.Fatal(err)
	}
	if tsMs != base.Add(-time.Hour).UnixMilli() || msPlayed != 200_000 || skipped != nil || offline != nil || platform != "Windows 7" ||
		!strings.Contains(raw, `"ip_addr_decrypted"`) || !strings.HasPrefix(raw, "{\n") {
		t.Fatalf("old row: %d %d %v %v %q", tsMs, msPlayed, skipped, offline, platform)
	}
	if n := count(t, d, "uri = 'spotify:episode:ch1' AND content_type = 'audiobook'"); n != 1 {
		t.Errorf("audiobook chapter uri: %d", n)
	}

	// Re-importing is a no-op.
	j2, _, err := ImportPath(ctx, d, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if j2.RowsNew != 0 || j2.RowsDuplicate != 10 || j2.State != StateDone || count(t, d, "1") != 9 {
		t.Fatalf("re-import: %+v", j2)
	}
	// So is a later export that reformats fields (key order, spacing) and
	// adds new ones; its new rows are imported.
	re := []map[string]any{newRow(2, "b", 5_000), newRow(100, "z", 1)}
	re[0]["some_new_field"] = true
	compact, _ := json.Marshal(re)
	jp := filepath.Join(t.TempDir(), "Streaming_History_Audio_2019_1.json")
	os.WriteFile(jp, compact, 0o600)
	j3, _, err := ImportPath(ctx, d, jp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if j3.RowsNew != 1 || j3.RowsDuplicate != 1 || j3.Kind != KindJSON {
		t.Fatalf("overlapping export: %+v", j3)
	}

	jobs, err := List(ctx, d, 10)
	if err != nil || len(jobs) != 3 || jobs[0].ID != j3.ID {
		t.Fatalf("list: %v %v", jobs, err)
	}
	if _, err := Get(ctx, d, 999); err != ErrNotFound {
		t.Fatalf("missing job: %v", err)
	}
	// Raw rows are append-only.
	if _, err := d.Write.Exec(`DELETE FROM raw_export_rows`); err == nil {
		t.Fatal("raw_export_rows accepted a delete")
	}
}

func TestImportDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "Spotify Extended Streaming History")
	os.MkdirAll(sub, 0o700)
	os.WriteFile(filepath.Join(sub, "Streaming_History_Audio_2019.json"), encode(t, []map[string]any{newRow(1, "a", 1)}), 0o600)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
	d := openDB(t)
	j, _, err := ImportPath(context.Background(), d, dir, nil)
	if err != nil || j.Kind != KindDir || j.RowsNew != 1 {
		t.Fatalf("dir: %+v %v", j, err)
	}
}

func TestImportFailures(t *testing.T) {
	ctx := context.Background()
	good := encode(t, []map[string]any{newRow(1, "a", 1), newRow(2, "b", 1)})
	basicRows, _ := json.Marshal([]map[string]any{{"endTime": "2024-01-01 10:00", "artistName": "A", "trackName": "T", "msPlayed": 1}})
	cases := []struct {
		name  string
		path  func(t *testing.T) string
		code  string
		new   int64
		state string
	}{
		{"no history files", func(t *testing.T) string {
			return writeZip(t, map[string][]byte{"ReadMe.pdf": []byte("%PDF")})
		}, CodeNoHistoryFiles, 0, StateFailed},
		{"basic export zip", func(t *testing.T) string {
			return writeZip(t, map[string][]byte{"Spotify Account Data/StreamingHistory_music_0.json": basicRows})
		}, CodeBasicExport, 0, StateFailed},
		{"basic rows in a renamed file", func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "history.json")
			os.WriteFile(p, basicRows, 0o600)
			return p
		}, CodeBasicExport, 0, StateFailed},
		{"truncated file keeps earlier files", func(t *testing.T) string {
			return writeZip(t, map[string][]byte{
				"a/Streaming_History_Audio_2018.json": good,
				"a/Streaming_History_Audio_2019.json": good[:len(good)-40],
			})
		}, CodeMalformedFile, 2, StateFailed},
		{"not an array", func(t *testing.T) string {
			return writeZip(t, map[string][]byte{"Streaming_History_Audio_2019.json": []byte(`{"ts": 1}`)})
		}, CodeMalformedFile, 0, StateFailed},
		{"bad row keeps earlier rows", func(t *testing.T) string {
			rows := []map[string]any{newRow(1, "a", 1), newRow(2, "b", 1)}
			rows[1]["ts"] = "yesterday"
			return writeZip(t, map[string][]byte{"Streaming_History_Audio_2019.json": encode(t, rows)})
		}, CodeMalformedFile, 1, StateFailed},
		{"wrong field type", func(t *testing.T) string {
			rows := []map[string]any{newRow(1, "a", 1)}
			rows[0]["ms_played"] = "lots"
			return writeZip(t, map[string][]byte{"Streaming_History_Audio_2019.json": encode(t, rows)})
		}, CodeMalformedFile, 0, StateFailed},
		{"corrupt zip", func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "x.zip")
			os.WriteFile(p, []byte("PK\x03\x04garbage"), 0o600)
			return p
		}, CodeMalformedFile, 0, StateFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := openDB(t)
			j, _, err := ImportPath(ctx, d, c.path(t), nil)
			if err == nil || j == nil {
				t.Fatalf("expected failure, got %+v", j)
			}
			if j.State != c.state || j.ErrorCode != c.code || j.RowsNew != c.new || j.Error == "" || j.RowsTotal != nil {
				t.Fatalf("job: %+v", j)
			}
			if strings.Contains(j.Error, "192.0.2.1") || strings.Contains(j.Error, "Song") {
				t.Fatalf("error leaks row contents: %q", j.Error)
			}
			if got := count(t, d, "1"); got != c.new {
				t.Fatalf("rows kept: %d, want %d", got, c.new)
			}
		})
	}
	// Neither zip nor JSON.
	p := filepath.Join(t.TempDir(), "x.pdf")
	os.WriteFile(p, []byte("%PDF-1.4"), 0o600)
	if _, _, err := ImportPath(ctx, openDB(t), p, nil); err == nil || !strings.Contains(err.Error(), "not a zip") {
		t.Fatalf("pdf: %v", err)
	}
}

func TestSniff(t *testing.T) {
	for in, want := range map[string]Kind{"PK\x03\x04rest": KindZip, "PK\x05\x06": KindZip, "[": KindJSON,
		"\xef\xbb\xbf\n  [ {": KindJSON, " {": KindJSON} {
		if k, ok := Sniff([]byte(in)); !ok || k != want {
			t.Errorf("%q: %v %v", in, k, ok)
		}
	}
	for _, in := range []string{"", "%PDF", "PK", "x["} {
		if _, ok := Sniff([]byte(in)); ok {
			t.Errorf("%q sniffed", in)
		}
	}
}

func TestRowHash(t *testing.T) {
	a := rowHash(1, "spotify:track:a", 10, "x", "y", "")
	if a != rowHash(1, "spotify:track:a", 10, "other", "names", "") {
		t.Error("names must not matter when the URI is known")
	}
	if a == rowHash(1, "spotify:track:a", 11, "x", "y", "") || a == rowHash(2, "spotify:track:a", 10, "x", "y", "") {
		t.Error("ts and ms_played must matter")
	}
	if rowHash(1, "", 10, "x", "y", "") == rowHash(1, "", 10, "z", "y", "") {
		t.Error("names must matter when the URI is null")
	}
	if rowHash(1, "", 10, "ab", "c", "") == rowHash(1, "", 10, "a", "bc", "") {
		t.Error("fields must not run together")
	}
}

func TestServiceQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := openDB(t)
	dir := filepath.Join(t.TempDir(), "imports")
	var finished atomic.Int32
	s, err := NewService(ctx, Options{DB: d, Dir: dir, OnFinished: func() { finished.Add(1) }})
	if err != nil {
		t.Fatal(err)
	}
	upload := func(name string, b []byte) string {
		f, err := os.CreateTemp(s.Dir(), UploadPrefix+"*")
		if err != nil {
			t.Fatal(err)
		}
		f.Write(b)
		f.Close()
		return f.Name()
	}
	zipBytes, _ := os.ReadFile(writeZip(t, map[string][]byte{"Streaming_History_Audio_2019.json": encode(t, []map[string]any{newRow(1, "a", 1)})}))
	p1 := upload("a.zip", zipBytes)
	j1, err := s.Submit(ctx, p1, "a.zip", KindZip)
	if err != nil || j1.State != StateQueued || j1.Origin != "upload" {
		t.Fatalf("submit: %+v %v", j1, err)
	}
	p2 := upload("b.json", []byte(`[`))
	j2, _ := s.Submit(ctx, p2, "b.json", KindJSON)

	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	waitFor := func(id int64) *Job {
		t.Helper()
		for range 200 {
			j, err := s.Job(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			if j.State == StateDone || j.State == StateFailed {
				return j
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("job %d did not finish", id)
		return nil
	}
	if j := waitFor(j1.ID); j.State != StateDone || j.RowsNew != 1 {
		t.Fatalf("job 1: %+v", j)
	}
	if j := waitFor(j2.ID); j.State != StateFailed || j.ErrorCode != CodeMalformedFile {
		t.Fatalf("job 2: %+v", j)
	}
	if n := finished.Load(); n != 1 {
		t.Errorf("OnFinished called %d times, want 1 (only the job that added rows)", n)
	}
	for _, p := range []string{p1, p2} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("upload %s not removed", p)
		}
	}
	jobs, _ := s.Jobs(ctx, 10)
	if len(jobs) != 2 || jobs[0].ID != j2.ID {
		t.Fatalf("jobs: %+v", jobs)
	}
	cancel()
	<-done

	// A restart fails jobs that were running and resumes queued ones
	// whose upload is still there.
	p3 := upload("c.zip", zipBytes)
	j3, _ := s.Submit(context.Background(), p3, "c.zip", KindZip)
	d.Write.Exec(`UPDATE imports SET state = 'running' WHERE id = ?`, j1.ID)
	stray := upload("stray", []byte("x"))
	s2, err := NewService(context.Background(), Options{DB: d, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if j, _ := s2.Job(context.Background(), j1.ID); j.State != StateFailed || j.ErrorCode != CodeInterrupted {
		t.Fatalf("interrupted: %+v", j)
	}
	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Error("stray upload kept")
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	go s2.Run(ctx2)
	defer cancel2()
	s = s2
	if j := waitFor(j3.ID); j.State != StateDone || j.RowsDuplicate != 1 {
		t.Fatalf("resumed: %+v", j)
	}
}

// synthetic builds an export-shaped zip of n rows over files of
// rowsPerFile, with realistic field variety.
func synthetic(t testing.TB, n, rowsPerFile int) string {
	t.Helper()
	files := map[string][]byte{}
	var rows []map[string]any
	flush := func() {
		files[fmt.Sprintf("Spotify Extended Streaming History/Streaming_History_Audio_%04d.json", len(files))] = encode(t, rows)
		rows = rows[:0]
	}
	for i := range n {
		r := newRow(i*4, fmt.Sprintf("%022d", i%20000), 30_000+i%200_000)
		r["skipped"] = i%7 == 0
		rows = append(rows, r)
		if len(rows) == rowsPerFile {
			flush()
		}
	}
	if len(rows) > 0 {
		flush()
	}
	return writeZip(t, files)
}

// BenchmarkImport reports import throughput and peak heap. It imports a
// synthetic 100k-row export, or the export at $SPOTUSSI_IMPORT_FILE
// (never committed; DF4) into a fresh database per iteration:
//
//	SPOTUSSI_IMPORT_FILE=~/Downloads/my_spotify_data.zip go test -run '^$' -bench Import -benchtime 1x ./internal/importer
//
// For process RSS, time the CLI instead: /usr/bin/time -l spotussi import FILE.
func BenchmarkImport(b *testing.B) {
	p := os.Getenv("SPOTUSSI_IMPORT_FILE")
	if p == "" {
		p = synthetic(b, 100_000, 15_000)
	}
	var rows, bytesRead int64
	var peak uint64
	for b.Loop() {
		b.StopTimer()
		d := openDB(b)
		runtime.GC()
		stop := make(chan struct{})
		sampled := make(chan uint64)
		go func() { // sample the live heap while importing
			var m runtime.MemStats
			var hi uint64
			for {
				runtime.ReadMemStats(&m)
				hi = max(hi, m.HeapInuse)
				select {
				case <-stop:
					sampled <- hi
					return
				case <-time.After(20 * time.Millisecond):
				}
			}
		}()
		b.StartTimer()
		j, st, err := ImportPath(context.Background(), d, p, nil)
		b.StopTimer()
		close(stop)
		peak = max(peak, <-sampled)
		if err != nil {
			b.Fatal(err)
		}
		rows += j.RowsDone
		bytesRead += st.BytesTotal
		b.StartTimer()
	}
	secs := b.Elapsed().Seconds()
	b.ReportMetric(float64(rows)/secs, "rows/s")
	b.ReportMetric(float64(bytesRead)/secs/1e6, "MB/s")
	b.ReportMetric(float64(peak)/(1<<20), "peak-heap-MiB")
}
