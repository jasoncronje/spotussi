package live_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"

	"spotussi/internal/api"
	"spotussi/internal/db"
	"spotussi/internal/live"
)

// Live responses are validated against api/openapi.yaml like the mock's
// contract tests, so the provisional live endpoints cannot drift from
// the contract the UI is built against.

type spec struct {
	doc      map[string]any
	compiler *jsonschema.Compiler
}

func loadSpec(t *testing.T) *spec {
	t.Helper()
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	j, _ := json.Marshal(doc)
	jdoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(j))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource("openapi.json", jdoc); err != nil {
		t.Fatal(err)
	}
	return &spec{doc: doc, compiler: c}
}

func dig(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func (s *spec) validate(t *testing.T, path, method string, status int, body []byte) {
	t.Helper()
	op := dig(s.doc, "paths", path, method)
	resp := dig(op, "responses", fmt.Sprint(status))
	if resp == nil {
		resp = dig(op, "responses", "default")
	}
	if ref, ok := dig(resp, "$ref").(string); ok {
		resp = dig(s.doc, strings.Split(strings.TrimPrefix(ref, "#/"), "/")...)
	}
	ref, _ := dig(resp, "content", "application/json", "schema", "$ref").(string)
	if ref == "" {
		t.Fatalf("%s %s: no schema for %d", method, path, status)
	}
	sch, err := s.compiler.Compile("openapi.json" + ref)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s %s: invalid JSON: %v", method, path, err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Fatalf("%s %s: response does not match %s:\n%v\n%s", method, path, ref, err, body)
	}
}

type harness struct {
	t    *testing.T
	db   *db.DB
	spec *spec
	h    http.Handler
	be   *live.Backend
}

func openDB(t *testing.T, dir string) *db.DB {
	t.Helper()
	d, err := db.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func newHarness(t *testing.T, d *db.DB, o live.Options) *harness {
	t.Helper()
	o.DB = d
	if o.Now == nil {
		o.Now = func() time.Time { return time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC) }
	}
	be := live.New(o)
	h := &harness{t: t, db: d, spec: loadSpec(t), be: be,
		h: api.NewHandler(api.Options{Version: "t", Mode: "live", Backend: be})}
	h.rebuild()
	return h
}

// newRunningHarness runs the derivation engine as main does (load or
// rebuild at startup, then follow changes) and waits for its first
// snapshot.
func newRunningHarness(t *testing.T, d *db.DB, o live.Options) *harness {
	t.Helper()
	o.DB = d
	be := live.New(o)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { be.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	wait, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	if _, err := be.Engine().Snapshot(wait); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, db: d, spec: loadSpec(t), be: be,
		h: api.NewHandler(api.Options{Version: "t", Mode: "live", Backend: be})}
}

// rebuild derives plays from the raw tables now (the engine's Run loop
// is not started in tests).
func (h *harness) rebuild() {
	h.t.Helper()
	if err := h.be.Engine().Rebuild(context.Background()); err != nil {
		h.t.Fatal(err)
	}
}

// call performs a request, checks the status and validates the body
// against the spec operation tmpl.
func (h *harness) call(method, tmpl, path string, body any, want int) map[string]any {
	h.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1"+path, rd))
	if rec.Code != want {
		h.t.Fatalf("%s %s: status %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	h.spec.validate(h.t, tmpl, strings.ToLower(method), rec.Code, rec.Body.Bytes())
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func TestSettingsPersist(t *testing.T) {
	dir := t.TempDir()
	h := newHarness(t, openDB(t, dir), live.Options{DefaultTimezone: "Europe/Amsterdam"})
	s := h.call("GET", "/settings", "/settings", nil, 200)
	if s["timezone"] != "Europe/Amsterdam" || s["play_rule"] != "fixed" || s["date_format"] != "auto" ||
		s["week_start"] != "monday" || s["session_gap_minutes"] != float64(15) {
		t.Fatalf("defaults: %v", s)
	}
	s = h.call("PATCH", "/settings", "/settings", map[string]any{"timezone": "America/New_York",
		"play_rule": "fixed", "play_rule_threshold_ms": 45000, "week_start": "sunday", "date_format": "dmy",
		"session_gap_minutes": 30}, 200)
	if s["timezone"] != "America/New_York" || s["date_format"] != "dmy" || s["play_rule_threshold_ms"] != float64(45000) {
		t.Fatalf("patched: %v", s)
	}
	for _, bad := range []map[string]any{
		{"timezone": "Mars/Olympus"}, {"timezone": "Local"}, {"timezone": ""}, {"theme": "dark"},
		{"play_rule": "x"}, {"play_rule_threshold_ms": 10}, {"week_start": "friday"},
		{"date_format": "x"}, {"session_gap_minutes": 0}, {"unknown": 1},
	} {
		h.call("PATCH", "/settings", "/settings", bad, 400)
	}
	// A rejected patch changes nothing, even its valid fields.
	h.call("PATCH", "/settings", "/settings", map[string]any{"week_start": "monday", "timezone": "Nope/Nope"}, 400)

	// A new process (and a changed host zone) keeps the stored values.
	h2 := newHarness(t, h.db, live.Options{DefaultTimezone: "Asia/Tokyo"})
	s = h2.call("GET", "/settings", "/settings", nil, 200)
	want := map[string]any{"timezone": "America/New_York", "play_rule": "fixed",
		"play_rule_threshold_ms": float64(45000), "week_start": "sunday", "date_format": "dmy", "session_gap_minutes": float64(30)}
	for k, v := range want {
		if s[k] != v {
			t.Errorf("%s = %v, want %v", k, s[k], v)
		}
	}
	// Corrupt stored values fall back to defaults instead of failing
	// (settings are cached after the first read, so a new process).
	h.db.Write.Exec(`UPDATE settings SET value = '"Gone/Zone"' WHERE key = 'timezone'`)
	h.db.Write.Exec(`UPDATE settings SET value = 'not json' WHERE key = 'date_format'`)
	h2 = newHarness(t, h.db, live.Options{DefaultTimezone: "Asia/Tokyo"})
	s = h2.call("GET", "/settings", "/settings", nil, 200)
	if s["timezone"] != "Asia/Tokyo" || s["date_format"] != "auto" || s["week_start"] != "sunday" {
		t.Fatalf("fallbacks: %v", s)
	}
	// An invalid default zone falls back to UTC.
	if s := newHarness(t, openDB(t, t.TempDir()), live.Options{DefaultTimezone: "Local"}).
		call("GET", "/settings", "/settings", nil, 200); s["timezone"] != "UTC" {
		t.Fatalf("default zone: %v", s)
	}
}

// Every dashboard endpoint answers on an empty database (no 501s since
// M6); the differential test covers their content.
func TestEmptyDatabase(t *testing.T) {
	h := newHarness(t, openDB(t, t.TempDir()), live.Options{})
	for _, p := range []string{"/top/tracks", "/top/artists", "/top/genres", "/overview/summary", "/timeline",
		"/clock", "/stats/hourly-top", "/stats/artists-over-time", "/stats/genres-over-time", "/stats/release-years",
		"/stats/artists-per-track", "/sessions", "/exclusions", "/devices", "/contexts", "/search?q=a", "/history"} {
		tmpl, _, _ := strings.Cut(p, "?")
		h.call("GET", tmpl, p, nil, 200)
	}
	h.call("GET", "/artists/{id}", "/artists/nope", nil, 404)
	h.call("GET", "/top/tracks", "/nope", nil, 404)
}

func TestContainsLower(t *testing.T) {
	for _, c := range []struct {
		s, q string
		want bool
	}{
		{"The Beatles", "the", true}, {"Other", "the", true}, {"Bach", "the", false},
		{"ABC", "", true}, {"", "a", false}, {"Björk", "björk", true}, {"BJÖRK", "björk", true}, {"Ab", "abc", false},
	} {
		if got := live.ContainsLower(c.s, c.q); got != c.want {
			t.Errorf("containsLower(%q, %q) = %v", c.s, c.q, got)
		}
	}
}
