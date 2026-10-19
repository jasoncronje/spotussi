package live_test

import (
	"encoding/json"
	"testing"
	"time"

	"spotussi/internal/capture"
	"spotussi/internal/live"
	"spotussi/internal/spotify"
	"spotussi/internal/spotify/spotifytest"
)

var now = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

type fakeCapture struct {
	status  capture.Status
	st      *spotify.PlayerState
	started int64
}

func (f *fakeCapture) Status() capture.Status { return f.status }
func (f *fakeCapture) NowPlaying() (*spotify.PlayerState, int64, int64) {
	return f.st, now.UnixMilli() - 1000, f.started
}

func ms(d time.Duration) int64 { return now.Add(d).UnixMilli() }

func (h *harness) pollerPlay(track string, start, end time.Duration, played int64, skipped bool, device string) {
	h.t.Helper()
	raw := spotifytest.PlayerSpec{TrackID: track, TrackName: "Song " + track, DurationMs: 200_000, Playing: true}.JSON()
	if _, err := h.db.Write.Exec(`INSERT INTO raw_poller_plays (track_id, track_uri, duration_ms, started_ms, ended_ms,
		ms_played, skipped, end_reason, shuffle, repeat, device_name, device_type, context_uri, private_session, seeks, pauses,
		device_switches, observations, first_seen_ms, last_seen_ms, json, created_at)
		VALUES (?, ?, 200000, ?, ?, ?, ?, 'track_change', 1, 'off', ?, 'Speaker', 'spotify:playlist:37i9dQZF1DXcBWIGoYBM5M', 0, 0, 0, 0, 5, ?, ?, ?, ?)`,
		track, "spotify:track:"+track, ms(start), ms(end), played, skipped, device, ms(start), ms(end), string(raw), ms(end)); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) recentPlay(track string, playedAt time.Duration) {
	h.t.Helper()
	item := map[string]any{"played_at": now.Add(playedAt).Format(time.RFC3339Nano), "context": nil,
		"track": map[string]any{"id": track, "uri": "spotify:track:" + track, "name": "Song " + track, "duration_ms": 200_000,
			"artists": []any{map[string]any{"id": "artist-" + track, "name": "Artist"}},
			"album":   map[string]any{"id": "album-" + track, "name": "Album", "images": []any{}}}}
	raw, _ := json.Marshal(item)
	if _, err := h.db.Write.Exec(`INSERT INTO raw_recent_plays (played_at_ms, track_id, duration_ms, context_uri, json, fetched_at)
		VALUES (?, ?, 200000, NULL, ?, ?)`, ms(playedAt), track, string(raw), ms(0)); err != nil {
		h.t.Fatal(err)
	}
}

func ids(page map[string]any) []string {
	var out []string
	for _, it := range page["items"].([]any) {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func TestHistory(t *testing.T) {
	fc := &fakeCapture{status: capture.Status{State: capture.StateIdle}}
	h := newHarness(t, openDB(t, t.TempDir()), live.Options{DefaultTimezone: "UTC", Capture: fc,
		Now: func() time.Time { return now }})

	empty := h.call("GET", "/history", "/history", nil, 200)
	if empty["total"] != float64(0) || len(empty["items"].([]any)) != 0 {
		t.Fatalf("empty: %v", empty)
	}
	// Three hours ago: a full play the backstop confirmed; two hours ago:
	// a skip; one hour ago: a play only recently-played knows about.
	h.pollerPlay("a", -3*time.Hour, -3*time.Hour+200*time.Second, 200_000, false, "Kitchen")
	h.recentPlay("a", -3*time.Hour+200*time.Second+300*time.Millisecond)
	h.pollerPlay("b", -2*time.Hour, -2*time.Hour+20*time.Second, 20_000, true, "Phone")
	h.recentPlay("c", -1*time.Hour)
	h.rebuild()

	page := h.call("GET", "/history", "/history", nil, 200)
	items := page["items"].([]any)
	if page["total"] != float64(3) || len(items) != 3 || page["next_cursor"] != nil {
		t.Fatalf("page: %v", page)
	}
	c, b, a := items[0].(map[string]any), items[1].(map[string]any), items[2].(map[string]any)
	// played_at is taken as the end (no calibration yet): c started a
	// track length before it.
	if c["played_at"] != float64(ms(-time.Hour-200*time.Second)) || c["ms_estimated"] != true ||
		c["counts_as_play"] != true || dig(c, "track", "name") != "Song c" || c["sources"].([]any)[0] != "recent" {
		t.Fatalf("recent-only row: %v", c)
	}
	if b["skipped"] != true || b["counts_as_play"] != false || dig(b, "device", "name") != "Phone" ||
		len(b["sources"].([]any)) != 1 {
		t.Fatalf("skip row: %v", b)
	}
	if srcs := a["sources"].([]any); len(srcs) != 2 || srcs[1] != "recent" || a["counts_as_play"] != true ||
		dig(a, "context", "spotify_owned") != true || dig(a, "context", "type") != "playlist" ||
		dig(a, "context", "name_source") != "unresolved" || dig(a, "track", "album", "id") != "album-a" {
		t.Fatalf("confirmed row: %v", a)
	}

	// Cursor pagination, one row per page.
	var seen []string
	path := "/history?limit=1"
	for range 4 {
		p := h.call("GET", "/history", path, nil, 200)
		seen = append(seen, ids(p)...)
		cur, _ := p["next_cursor"].(string)
		if cur == "" {
			break
		}
		path = "/history?limit=1&cursor=" + cur
	}
	if len(seen) != 3 || seen[0] != c["id"] || seen[2] != a["id"] {
		t.Fatalf("paged: %v", seen)
	}
	h.call("GET", "/history", "/history?cursor=zz", nil, 400)

	// Filters.
	for q, want := range map[string]int{
		"skipped=true": 1, "counted=true": 2, "counted=false": 1, "shuffle=true": 2, "device=Kitchen": 1,
		"platform=Android": 0, "track_id=c": 1, "artist_id=artist-a": 1, "range=today": 3, "from=2026-10-10&to=2026-10-16": 0,
	} {
		p := h.call("GET", "/history", "/history?"+q, nil, 200)
		if p["total"] != float64(want) {
			t.Errorf("%s: total %v, want %d", q, p["total"], want)
		}
	}

	// A measured start anchor moves recent-only rows.
	fc.status.Calibration = &capture.Calibration{Verdict: capture.AnchorStart}
	h.rebuild()
	if p := h.call("GET", "/history", "/history?track_id=c", nil, 200); dig(p["items"].([]any)[0], "played_at") != float64(ms(-time.Hour)) {
		t.Fatalf("start-anchored: %v", p)
	}

	// System status counts derived rows: the skip is not a play.
	st := h.call("GET", "/system/status", "/system/status", nil, 200)
	if dig(st, "data", "rows") != float64(3) || dig(st, "data", "plays") != float64(2) ||
		dig(st, "data", "first_play_at") != float64(ms(-3*time.Hour)) {
		t.Fatalf("data status: %v", st["data"])
	}
}

func TestNowPlayingAndCaptureStatus(t *testing.T) {
	fc := &fakeCapture{status: capture.Status{State: capture.StateStopped}}
	lim := spotify.NewLimiter(spotify.LimiterConfig{}, nil)
	h := newHarness(t, openDB(t, t.TempDir()), live.Options{Capture: fc, LimiterStats: lim.Stats})

	np := h.call("GET", "/now-playing", "/now-playing", nil, 200)
	if np["is_playing"] != false || np["track"] != nil || np["observed_at"] != nil {
		t.Fatalf("nothing yet: %v", np)
	}
	var ps spotify.PlayerState
	raw := spotifytest.PlayerSpec{TrackID: "t1", TrackName: "One", DurationMs: 200_000, ProgressMs: 30_000, Playing: true,
		Shuffle: true, ContextURI: "spotify:album:al", DeviceName: "Kitchen", DeviceType: "Speaker"}.JSON()
	if err := json.Unmarshal(raw, &ps); err != nil {
		t.Fatal(err)
	}
	fc.st, fc.started = &ps, ms(-31*time.Second)
	np = h.call("GET", "/now-playing", "/now-playing", nil, 200)
	if np["is_playing"] != true || dig(np, "track", "name") != "One" || np["progress_ms"] != float64(30_000) ||
		np["started_at"] != float64(ms(-31*time.Second)) || dig(np, "device", "name") != "Kitchen" ||
		dig(np, "context", "type") != "album" || np["shuffle"] != true || dig(np, "track", "image_url") == nil {
		t.Fatalf("playing: %v", np)
	}

	st := h.call("GET", "/system/status", "/system/status", nil, 200)
	if dig(st, "capture", "state") != "stopped" || dig(st, "capture", "rate_limit", "burst") != float64(10) ||
		dig(st, "capture", "calibration") != nil || dig(st, "capture", "backstop", "items_stored") != float64(0) {
		t.Fatalf("stopped status: %v", st["capture"])
	}
	fc.status = capture.Status{State: capture.StatePlaying, PollInterval: 12 * time.Second, LastPollAt: now,
		NextPollAt: now.Add(12 * time.Second), LastPlayAt: now.Add(-time.Minute), RecentLastRunAt: now, RecentInserted: 7,
		Calibration: &capture.Calibration{Samples: 30, Discriminating: 25, Verdict: capture.AnchorEnd,
			End: capture.OffsetStats{MedianMs: 400, P10Ms: 100, P90Ms: 900, HitRate: 0.96}, UpdatedAt: now}}
	st = h.call("GET", "/system/status", "/system/status", nil, 200)
	if dig(st, "capture", "poll_interval_ms") != float64(12_000) || dig(st, "capture", "last_poll_at") != float64(ms(0)) ||
		dig(st, "capture", "calibration", "verdict") != "end" || dig(st, "capture", "calibration", "end_offset", "hit_rate") != 0.96 ||
		dig(st, "capture", "backstop", "items_stored") != float64(7) || dig(st, "capture", "last_error") != nil {
		t.Fatalf("playing status: %v", st["capture"])
	}
	// The error path.
	fc.status = capture.Status{State: capture.StateStopped, LastError: "Spotify access expired or was revoked: reconnect Spotify",
		LastErrorAt: now}
	st = h.call("GET", "/system/status", "/system/status", nil, 200)
	if dig(st, "capture", "last_error") == nil || dig(st, "capture", "last_error_at") != float64(ms(0)) {
		t.Fatalf("error status: %v", st["capture"])
	}
}
