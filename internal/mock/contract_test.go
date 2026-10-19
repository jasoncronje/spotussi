package mock_test

// Contract tests: every operation in api/openapi.yaml is called against the
// mock backend through the real HTTP handler, and every response body is
// validated against the operation's response schema (JSON Schema 2020-12,
// which OpenAPI 3.1 schemas are). additionalProperties:false in the spec
// makes Go wire types that drift from the contract fail here.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"

	"spotussi/internal/api"
	"spotussi/internal/auth"
	"spotussi/internal/config"
	"spotussi/internal/db"
	"spotussi/internal/keyring"
	"spotussi/internal/mock"
	"spotussi/internal/refdata"
	"spotussi/internal/spotify"
	"spotussi/internal/spotify/spotifytest"
)

const specPath = "../../api/openapi.yaml"

type spec struct {
	doc      map[string]any
	compiler *jsonschema.Compiler
	cache    map[string]*jsonschema.Schema
}

func loadSpec(t *testing.T) *spec {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.1") {
		t.Fatalf("spec must be OpenAPI 3.1, got %q", v)
	}
	// Round-trip through JSON so numbers and maps have JSON shapes.
	j, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	jdoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(j))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource("openapi.json", jdoc); err != nil {
		t.Fatal(err)
	}
	return &spec{doc: doc, compiler: c, cache: map[string]*jsonschema.Schema{}}
}

func (s *spec) schema(t *testing.T, ref string) *jsonschema.Schema {
	t.Helper()
	if sch, ok := s.cache[ref]; ok {
		return sch
	}
	sch, err := s.compiler.Compile("openapi.json" + ref)
	if err != nil {
		t.Fatalf("compile %s: %v", ref, err)
	}
	s.cache[ref] = sch
	return sch
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

// responseRef returns the schema $ref for an operation's response status
// (falling back to "default").
func (s *spec) responseRef(path, method string, status int) string {
	op := dig(s.doc, "paths", path, method)
	resp := dig(op, "responses", fmt.Sprint(status))
	if resp == nil {
		resp = dig(op, "responses", "default")
	}
	if ref, ok := dig(resp, "$ref").(string); ok { // components/responses
		resp = dig(s.doc, strings.Split(strings.TrimPrefix(ref, "#/"), "/")...)
	}
	ref, _ := dig(resp, "content", "application/json", "schema", "$ref").(string)
	return ref
}

type fixture struct {
	t       *testing.T
	srv     http.Handler
	spec    *spec
	ids     map[string]string
	covered map[string]bool
	fake    *spotifytest.Fake
	auth    *auth.Service

	// Credentials attached to every request (password mode).
	cookie, csrf, bearer string
}

const (
	testPassword  = "contract-test-password"
	testClientID  = "0123456789abcdef0123456789abcdef"
	sessionCookie = "spotussi_session"
)

// newFixture serves the mock backend behind real password-mode auth and a
// Spotify client wired to a fake Spotify, then runs first-run setup so
// requests carry an owner session.
func newFixture(t *testing.T) *fixture {
	end := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	ds := refdata.Generate(refdata.Config{Size: refdata.Small, Seed: 1, End: end})
	be := mock.NewWithClock(ds, func() time.Time { return end })
	database, err := db.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	keys, _ := keyring.New(bytes.Repeat([]byte{5}, 32))
	fake := spotifytest.New(testClientID)
	t.Cleanup(fake.Close)
	a := auth.New(auth.Options{DB: database, Keyring: keys, Mode: config.AuthPassword,
		Hash: auth.Params{MemoryKiB: 64, Time: 1, Threads: 1}, AnnounceSetupToken: func(string) {}})
	f := &fixture{t: t, spec: loadSpec(t), covered: map[string]bool{}, fake: fake, auth: a,
		srv: api.NewHandler(api.Options{Version: "test", Mode: "mock", Backend: be, DB: database,
			Extra:     map[string]http.Handler{"GET " + mock.ImagePath + "{kind}/{file}": be.ImageHandler()},
			Auth:      a,
			ImportDir: t.TempDir(), ImportMaxBytes: 1 << 20,
			Spotify: spotify.New(spotify.Options{DB: database, Keyring: keys,
				AccountsURL: fake.AccountsURL(), APIURL: fake.APIURL()}),
		})}
	s := f.call("GET", at("/auth/session"), "", nil, 200)
	if s["setup_required"] != true || s["authenticated"] != false {
		t.Fatalf("fresh instance session: %v", s)
	}
	f.call("POST", at("/auth/setup"), "", map[string]any{"password": testPassword, "setup_token": "wrong"}, 401)
	f.signIn("/auth/setup")
	f.ids = f.discoverIDs()
	return f
}

// signIn posts the test password to path and keeps the session.
func (f *fixture) signIn(path string) {
	f.t.Helper()
	in := map[string]any{"password": testPassword}
	if path == "/auth/setup" {
		in["setup_token"] = f.auth.SetupToken()
	}
	rec := f.raw("POST", "/api/v1"+path, in)
	body := f.check("POST", at(path), rec, 200)
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Secure {
				f.t.Fatalf("session cookie attributes: %+v", c)
			}
			f.cookie = c.Value
		}
	}
	f.csrf, _ = body["csrf_token"].(string)
	if f.cookie == "" || f.csrf == "" {
		f.t.Fatalf("%s: no session: %v", path, body)
	}
}

func (f *fixture) raw(method, url string, body any) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	return f.send(httptest.NewRequest(method, url, rd))
}

// upload POSTs a file body to /imports.
func (f *fixture) upload(query, contentType string, body []byte, want int) map[string]any {
	f.t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/imports?"+query, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return f.check("POST", at("/imports"), f.send(req), want)
}

func (f *fixture) send(req *http.Request) *httptest.ResponseRecorder {
	if f.cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	}
	if f.csrf != "" && req.Method != "GET" && req.Method != "HEAD" {
		req.Header.Set("X-CSRF-Token", f.csrf)
	}
	if f.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+f.bearer)
	}
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	return rec
}

// route fills the {params} of a spec path template in order.
type route struct{ tmpl, path string }

func at(tmpl string, args ...string) route {
	path := tmpl
	for _, a := range args {
		i, j := strings.Index(path, "{"), strings.Index(path, "}")
		path = path[:i] + a + path[j+1:]
	}
	return route{tmpl, path}
}

// call performs a request and validates the response against the spec
// operation for rt.tmpl. It returns the decoded body.
func (f *fixture) call(method string, rt route, query string, body any, wantStatus int) map[string]any {
	f.t.Helper()
	url := "/api/v1" + rt.path
	if query != "" {
		url += "?" + query
	}
	return f.check(method, rt, f.raw(method, url, body), wantStatus)
}

// check validates a response against the spec operation for rt.tmpl.
func (f *fixture) check(method string, rt route, rec *httptest.ResponseRecorder, wantStatus int) map[string]any {
	f.t.Helper()
	path, url := rt.tmpl, rt.path
	if rec.Code != wantStatus {
		f.t.Fatalf("%s %s: status %d, want %d: %s", method, url, rec.Code, wantStatus, rec.Body.String())
	}
	if rec.Code == http.StatusNoContent {
		if rec.Body.Len() != 0 || dig(f.spec.doc, "paths", path, strings.ToLower(method), "responses", "204") == nil {
			f.t.Fatalf("%s %s: unexpected 204 (body %q)", method, url, rec.Body.String())
		}
		f.covered[strings.ToUpper(method)+" "+path] = true
		return nil
	}
	ref := f.spec.responseRef(path, strings.ToLower(method), rec.Code)
	if ref == "" {
		f.t.Fatalf("%s %s: spec has no JSON schema for status %d", method, path, rec.Code)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		f.t.Fatalf("%s %s: invalid JSON: %v", method, url, err)
	}
	if err := f.spec.schema(f.t, ref).Validate(inst); err != nil {
		f.t.Fatalf("%s %s: response does not match %s:\n%v", method, url, ref, err)
	}
	if wantStatus < 300 {
		f.covered[strings.ToUpper(method)+" "+path] = true
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (f *fixture) discoverIDs() map[string]string {
	get := func(url string) map[string]any {
		rec := f.raw("GET", url, nil)
		var m map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		return m
	}
	first := func(m map[string]any, keys ...string) string {
		items, _ := m["items"].([]any)
		if len(items) == 0 {
			f.t.Fatalf("no items for %v", keys)
		}
		s, _ := dig(items[0], keys...).(string)
		return s
	}
	ids := map[string]string{
		"artist":  first(get("/api/v1/top/artists?range=all&limit=1"), "artist", "id"),
		"album":   first(get("/api/v1/top/albums?range=all&limit=1"), "album", "id"),
		"track":   first(get("/api/v1/top/tracks?range=all&limit=1"), "track", "id"),
		"session": first(get("/api/v1/sessions?range=all&limit=1"), "id"),
		"genre":   first(get("/api/v1/top/genres?range=all&limit=1"), "genre", "id"),
	}
	for _, it := range get("/api/v1/contexts?range=all&limit=500")["items"].([]any) {
		c := dig(it, "context").(map[string]any)
		if c["spotify_owned"] == true {
			ids["playlist"] = c["id"].(string)
			break
		}
	}
	if ids["playlist"] == "" {
		f.t.Fatal("no Spotify-owned playlist context in dataset")
	}
	return ids
}

func TestEveryOperationMatchesContract(t *testing.T) {
	f := newFixture(t)
	ids := f.ids
	ranges := []string{"", "range=today", "range=this_week", "range=this_month", "range=this_year",
		"range=last_7d", "range=last_4w", "range=last_6m", "range=last_12m", "range=prev_day",
		"range=prev_week", "range=prev_month", "range=prev_year", "range=all", "from=2025-02-01&to=2025-03-15"}
	for _, r := range ranges {
		for _, m := range []string{"plays", "time"} {
			q := strings.TrimPrefix(r+"&metric="+m, "&")
			f.call("GET", at("/overview/summary"), q, nil, 200)
			f.call("GET", at("/top/tracks"), q+"&limit=5", nil, 200)
			f.call("GET", at("/top/artists"), q+"&limit=5", nil, 200)
			f.call("GET", at("/top/albums"), q+"&limit=5", nil, 200)
			f.call("GET", at("/top/genres"), q+"&limit=5", nil, 200)
			f.call("GET", at("/stats/hourly-top"), q, nil, 200)
			f.call("GET", at("/stats/artists-over-time"), q, nil, 200)
			f.call("GET", at("/stats/genres-over-time"), q, nil, 200)
			f.call("GET", at("/stats/release-years"), q, nil, 200)
			f.call("GET", at("/stats/artists-per-track"), q, nil, 200)
			f.call("GET", at("/artists/{id}", ids["artist"]), q, nil, 200)
			f.call("GET", at("/albums/{id}", ids["album"]), q, nil, 200)
			f.call("GET", at("/tracks/{id}", ids["track"]), q, nil, 200)
			f.call("GET", at("/genres/{id}", ids["genre"]), q, nil, 200)
			f.call("GET", at("/contexts"), q, nil, 200)
		}
		f.call("GET", at("/timeline"), r, nil, 200)
		f.call("GET", at("/timeline"), strings.TrimPrefix(r+"&compare=true", "&"), nil, 200)
		f.call("GET", at("/clock"), r, nil, 200)
		f.call("GET", at("/sessions"), r, nil, 200)
		f.call("GET", at("/devices"), r, nil, 200)
		f.call("GET", at("/history"), strings.TrimPrefix(r+"&limit=20", "&"), nil, 200)
	}
	for _, b := range []string{"hour", "day", "week", "month", "year"} {
		f.call("GET", at("/timeline"), "range=last_4w&bucket="+b, nil, 200)
		f.call("GET", at("/stats/genres-over-time"), "range=last_12m&series=3&bucket="+b, nil, 200)
	}
	for _, sc := range []string{"artist_id=" + ids["artist"], "album_id=" + ids["album"], "track_id=" + ids["track"], "genre_id=" + ids["genre"]} {
		f.call("GET", at("/timeline"), "range=all&"+sc, nil, 200)
		f.call("GET", at("/clock"), "range=all&"+sc, nil, 200)
		f.call("GET", at("/contexts"), "range=all&"+sc, nil, 200)
		f.call("GET", at("/history"), "limit=5&"+sc, nil, 200)
	}
	f.call("GET", at("/top/tracks"), "range=all&artist_id="+ids["artist"], nil, 200)
	f.call("GET", at("/top/tracks"), "range=all&album_id="+ids["album"], nil, 200)
	f.call("GET", at("/top/albums"), "range=all&artist_id="+ids["artist"], nil, 200)
	f.call("GET", at("/top/artists"), "range=all&album_id="+ids["album"], nil, 200)
	for _, top := range []string{"/top/tracks", "/top/artists", "/top/albums"} {
		f.call("GET", at(top), "range=last_12m&genre_id="+ids["genre"], nil, 200)
	}
	f.call("GET", at("/timeline"), "range=this_month&compare=true&genre_id="+ids["genre"], nil, 200)

	f.call("GET", at("/health"), "", nil, 200)
	f.call("GET", at("/system/status"), "", nil, 200)
	f.call("GET", at("/now-playing"), "", nil, 200)
	f.call("GET", at("/sessions/{id}", ids["session"]), "", nil, 200)
	f.call("GET", at("/search"), "q=a&limit=3", nil, 200)
	f.call("GET", at("/history"), "skipped=true&shuffle=false&counted=false&limit=3", nil, 200)
	f.call("GET", at("/history"), "device=iPhone&limit=3", nil, 200)
	f.call("GET", at("/history"), "platform=ios&limit=3", nil, 200)

	// Imports: the dataset's own import, then a simulated upload.
	if l := f.call("GET", at("/imports"), "", nil, 200); len(l["items"].([]any)) != 1 {
		t.Fatalf("imports: %v", l)
	}
	f.call("GET", at("/imports/{id}", "1"), "", nil, 200)
	f.call("GET", at("/imports/{id}", "nope"), "", nil, 404)
	job := f.upload("filename=my_spotify_data.zip", "application/zip", []byte("PK\x03\x04 not really a zip"), 202)
	if job["state"] != "queued" || job["filename"] != "my_spotify_data.zip" || job["origin"] != "upload" {
		t.Fatalf("upload: %v", job)
	}
	f.call("GET", at("/imports/{id}", job["id"].(string)), "", nil, 200)
	if e := f.upload("", "application/pdf", []byte("%PDF-1.4"), 415); dig(e, "error", "code") != "unsupported_file" {
		t.Fatalf("pdf upload: %v", e)
	}
	f.upload("", "", nil, 400)
	if e := f.upload("", "application/json", bytes.Repeat([]byte(" "), 1<<20+1), 413); dig(e, "error", "code") != "too_large" {
		t.Fatalf("large upload: %v", e)
	}

	// Database download: a binary body, so checked here rather than
	// against a JSON schema.
	if rec := f.raw("GET", "/api/v1/system/backup", nil); rec.Code != 200 ||
		rec.Header().Get("Content-Type") != "application/vnd.sqlite3" ||
		!bytes.HasPrefix(rec.Body.Bytes(), []byte("SQLite format 3\x00")) {
		t.Fatalf("backup download: %d %s", rec.Code, rec.Header())
	}
	f.covered["GET /system/backup"] = true

	// Auth and API tokens.
	f.call("GET", at("/auth/session"), "", nil, 200)
	f.call("POST", at("/auth/setup"), "", map[string]any{"password": "another-password"}, 409)
	f.call("POST", at("/auth/login"), "", map[string]any{"password": "wrong-password"}, 401)
	f.signIn("/auth/login")
	f.call("GET", at("/auth/tokens"), "", nil, 200)
	created := f.call("POST", at("/auth/tokens"), "", map[string]any{"name": "contract test"}, 201)
	tokenID := dig(created, "token", "id").(string)
	if list := f.call("GET", at("/auth/tokens"), "", nil, 200); len(list["items"].([]any)) != 1 {
		t.Fatalf("token list: %v", list)
	}
	cookie, csrf := f.cookie, f.csrf
	f.cookie, f.csrf = "", ""
	f.call("GET", at("/top/tracks"), "", nil, 401)
	f.bearer = created["secret"].(string)
	f.call("GET", at("/top/tracks"), "limit=1", nil, 200)
	f.call("PATCH", at("/settings"), "", map[string]any{"week_start": "monday"}, 403)
	if e := f.upload("", "application/json", []byte("[]"), 403); dig(e, "error", "code") != "read_only_token" {
		t.Fatalf("token upload: %v", e)
	}
	f.call("GET", at("/auth/tokens"), "", nil, 403)
	f.call("GET", at("/system/backup"), "", nil, 403)
	f.bearer = "spt_invalid"
	f.call("GET", at("/top/tracks"), "", nil, 401)
	f.bearer, f.cookie, f.csrf = "", cookie, csrf
	f.call("DELETE", at("/auth/tokens/{id}", tokenID), "", nil, 204)
	f.call("DELETE", at("/auth/tokens/{id}", tokenID), "", nil, 404)

	// Spotify connection (paste-back against the fake Spotify).
	f.call("GET", at("/spotify/connection"), "", nil, 200)
	f.call("POST", at("/spotify/authorize"), "", nil, 409)
	f.call("PUT", at("/spotify/client-id"), "", map[string]any{"client_id": "not-a-client-id"}, 400)
	conn := f.call("PUT", at("/spotify/client-id"), "", map[string]any{"client_id": testClientID}, 200)
	if conn["state"] != "not_configured" || dig(conn, "callback", "mode") != "paste_back" ||
		dig(conn, "callback", "redirect_uri") != "http://127.0.0.1:8080/oauth/spotify/callback" {
		t.Fatalf("connection before authorizing: %v", conn)
	}
	authz := f.call("POST", at("/spotify/authorize"), "", map[string]any{"return_to": "/setup"}, 200)
	back, err := f.fake.Approve(authz["authorize_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", at("/spotify/paste-back"), "", map[string]any{"url": "https://example.com/nope"}, 400)
	conn = f.call("POST", at("/spotify/paste-back"), "", map[string]any{"url": back}, 200)
	if conn["state"] != "connected" || conn["account_name"] != "Test Listener" {
		t.Fatalf("connection after paste-back: %v", conn)
	}
	f.call("POST", at("/spotify/paste-back"), "", map[string]any{"url": back}, 400) // state is single-use
	f.fake.Set(func(fk *spotifytest.Fake) { fk.MeStatus = 403 })
	authz = f.call("POST", at("/spotify/authorize"), "", nil, 200)
	back, _ = f.fake.Approve(authz["authorize_url"].(string))
	f.call("POST", at("/spotify/paste-back"), "", map[string]any{"url": back}, 502)
	f.call("DELETE", at("/spotify/connection"), "", nil, 200)

	f.call("GET", at("/settings"), "", nil, 200)
	f.call("PATCH", at("/settings"), "", map[string]any{"date_format": "dmy", "week_start": "sunday"}, 200)
	f.call("PATCH", at("/settings"), "", map[string]any{"timezone": "America/New_York", "play_rule": "fixed"}, 200)
	f.call("GET", at("/overview/summary"), "range=this_week", nil, 200)
	f.call("GET", at("/exclusions"), "", nil, 200)
	f.call("PUT", at("/exclusions/{kind}/{id}", "artists", ids["artist"]), "", nil, 200)
	f.call("PUT", at("/exclusions/{kind}/{id}", "tracks", ids["track"]), "", nil, 200)
	f.call("GET", at("/artists/{id}", ids["artist"]), "range=all", nil, 200)
	f.call("DELETE", at("/exclusions/{kind}/{id}", "artists", ids["artist"]), "", nil, 200)
	f.call("PUT", at("/playlists/{id}/name", ids["playlist"]), "", map[string]any{"name": "Daily Mix 9"}, 200)
	f.call("PUT", at("/playlists/{id}/name", ids["playlist"]), "", map[string]any{"name": nil}, 200)

	// Errors use the Error schema.
	f.call("GET", at("/top/tracks"), "range=bogus", nil, 400)
	f.call("GET", at("/top/tracks"), "metric=bogus", nil, 400)
	f.call("GET", at("/top/tracks"), "from=2026-02-01", nil, 400)
	f.call("GET", at("/top/tracks"), "limit=0", nil, 400)
	f.call("GET", at("/timeline"), "range=all&bucket=hour", nil, 400)
	f.call("GET", at("/timeline"), "artist_id=x&track_id=y", nil, 400)
	f.call("GET", at("/artists/{id}", "nope"), "", nil, 404)
	f.call("GET", at("/genres/{id}", "nope"), "", nil, 404)
	f.call("GET", at("/timeline"), "genre_id=nope", nil, 404)
	f.call("GET", at("/timeline"), "artist_id=x&genre_id=y", nil, 400)
	f.call("PATCH", at("/settings"), "", map[string]any{"theme": "dark"}, 400)
	f.call("GET", at("/history"), "cursor=!!", nil, 400)
	f.call("PATCH", at("/settings"), "", map[string]any{"timezone": "Mars/Olympus"}, 400)
	f.call("PATCH", at("/settings"), "", map[string]any{"nope": 1}, 400)

	f.call("POST", at("/auth/logout"), "", nil, 204)
	f.call("GET", at("/system/status"), "", nil, 401)

	// Every operation in the spec must have been exercised successfully.
	var missing []string
	for path, item := range f.spec.doc["paths"].(map[string]any) {
		for method := range item.(map[string]any) {
			if method == "parameters" {
				continue
			}
			if key := strings.ToUpper(method) + " " + path; !f.covered[key] {
				missing = append(missing, key)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("operations not exercised: %v", missing)
	}
}

func TestETagRevalidation(t *testing.T) {
	f := newFixture(t)
	rec := f.raw("GET", "/api/v1/top/artists?range=all", nil)
	etag := rec.Header().Get("ETag")
	if etag == "" || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("missing caching headers: %v", rec.Header())
	}
	req := httptest.NewRequest("GET", "/api/v1/top/artists?range=all", nil)
	req.Header.Set("If-None-Match", etag)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	rec2 := httptest.NewRecorder()
	f.srv.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNotModified || rec2.Body.Len() != 0 {
		t.Fatalf("revalidation: status %d, %d bytes", rec2.Code, rec2.Body.Len())
	}
}

func TestHistoryPagination(t *testing.T) {
	f := newFixture(t)
	seen := map[string]bool{}
	cursor := ""
	var total float64
	for page := 0; page < 5; page++ {
		q := "range=last_7d&limit=50"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		h := f.call("GET", at("/history"), q, nil, 200)
		total = h["total"].(float64)
		var prev float64 = 1 << 62
		for _, it := range h["items"].([]any) {
			p := it.(map[string]any)
			id := p["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate play %s across pages", id)
			}
			seen[id] = true
			if at := p["played_at"].(float64); at > prev {
				t.Fatal("history not newest-first")
			} else {
				prev = at
			}
		}
		c, _ := h["next_cursor"].(string)
		if c == "" {
			break
		}
		cursor = c
	}
	if float64(len(seen)) != total && len(seen) != 250 {
		t.Fatalf("paged %d of %v rows", len(seen), total)
	}
}

func TestMockImages(t *testing.T) {
	f := newFixture(t)
	rec := f.raw("GET", mock.ImagePath+"artist/"+f.ids["artist"]+".svg", nil)
	if rec.Code != 200 || !strings.HasPrefix(rec.Body.String(), "<svg") || rec.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("image: %d %s", rec.Code, rec.Header())
	}
	if rec := f.raw("GET", mock.ImagePath+"artist/nope.svg", nil); rec.Code != 404 {
		t.Fatalf("unknown image: %d", rec.Code)
	}
}

func TestMockImportProgress(t *testing.T) {
	end := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	now := end
	be := mock.NewWithClock(refdata.Generate(refdata.Config{Size: refdata.Tiny, Seed: 1, End: end}), func() time.Time { return now })
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "upload")
	os.WriteFile(p, []byte("x"), 0o600)
	j, err := be.CreateImport(ctx, api.ImportUpload{Path: p, Filename: "my_spotify_data.zip", Kind: "zip", Size: 7_000_000})
	if err != nil || j.State != "queued" {
		t.Fatalf("created: %+v %v", j, err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("upload kept")
	}
	var last int64 = -1
	for _, step := range []time.Duration{2 * time.Second, 5 * time.Second, 30 * time.Second} {
		now = end.Add(step)
		j, _ = be.Import(ctx, j.ID)
		if j.RowsDone <= last || j.RowsNew+j.RowsDuplicate != j.RowsDone {
			t.Fatalf("at %s: %+v", step, j)
		}
		last = j.RowsDone
	}
	if j.State != "done" || j.RowsTotal == nil || *j.RowsTotal != j.RowsDone || j.BytesDone != 7_000_000 {
		t.Fatalf("finished: %+v", j)
	}
	b, _ := be.CreateImport(ctx, api.ImportUpload{Path: p, Filename: "StreamingHistory_music_0.json", Kind: "json", Size: 10})
	now = now.Add(10 * time.Second)
	if b, _ = be.Import(ctx, b.ID); b.State != "failed" || *b.ErrorCode != "basic_export" {
		t.Fatalf("basic export: %+v", b)
	}
	if l, _ := be.Imports(ctx); len(l.Items) != 3 || l.Items[0].ID != b.ID {
		t.Fatalf("list: %+v", l)
	}
}
