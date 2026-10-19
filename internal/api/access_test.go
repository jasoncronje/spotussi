package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"spotussi/internal/auth"
	"spotussi/internal/config"
	"spotussi/internal/db"
	"spotussi/internal/keyring"
	"spotussi/internal/spotify"
	"spotussi/internal/spotify/spotifytest"
	"spotussi/internal/trust"
)

const testClientID = "0123456789abcdef0123456789abcdef"

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *testClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type server struct {
	t     *testing.T
	h     http.Handler
	clock *testClock
	fake  *spotifytest.Fake
	auth  *auth.Service
}

type serverOpts struct {
	mode    config.AuthMode
	proxies string
	baseURL string
}

func newServer(t *testing.T, o serverOpts) *server {
	t.Helper()
	d, err := db.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	keys, _ := keyring.New(bytes.Repeat([]byte{2}, 32))
	proxies, err := trust.Parse(o.proxies)
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{t: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	fake := spotifytest.New(testClientID)
	t.Cleanup(fake.Close)
	a := auth.New(auth.Options{DB: d, Keyring: keys, Mode: o.mode, Proxies: proxies, Now: clock.now,
		Hash: auth.Params{MemoryKiB: 64, Time: 1, Threads: 1}})
	sp := spotify.New(spotify.Options{DB: d, Keyring: keys, AccountsURL: fake.AccountsURL(), APIURL: fake.APIURL()})
	h := NewHandler(Options{Version: "t", Mode: "live", DB: d, Auth: a, Spotify: sp, BaseURL: o.baseURL, Proxies: proxies})
	return &server{t: t, h: h, clock: clock, fake: fake, auth: a}
}

// req describes one request.
type req struct {
	method, path string
	body         any
	cookie, csrf string
	bearer       string
	remote       string
	header       map[string]string
	tls          bool
	host         string
}

func (s *server) do(r req) *httptest.ResponseRecorder {
	s.t.Helper()
	var rd *bytes.Reader
	if r.body != nil {
		b, _ := json.Marshal(r.body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	hr := httptest.NewRequest(r.method, r.path, rd)
	if r.remote != "" {
		hr.RemoteAddr = r.remote
	}
	if r.host != "" {
		hr.Host = r.host
	}
	if r.tls {
		hr.TLS = &tls.ConnectionState{}
	}
	if r.cookie != "" {
		hr.AddCookie(&http.Cookie{Name: auth.CookieName, Value: r.cookie})
	}
	if r.csrf != "" {
		hr.Header.Set(CSRFHeader, r.csrf)
	}
	if r.bearer != "" {
		hr.Header.Set("Authorization", "Bearer "+r.bearer)
	}
	for k, v := range r.header {
		hr.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, hr)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return m
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	c, _ := decode(t, rec)["error"].(map[string]any)["code"].(string)
	return c
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	if code != "" {
		if got := errCode(t, rec); got != code {
			t.Fatalf("error code %q, want %q", got, code)
		}
	}
}

// login runs setup (first time) and returns the session cookie and CSRF token.
func (s *server) setup(extra req) (cookie, csrf string, rec *httptest.ResponseRecorder) {
	s.t.Helper()
	extra.method, extra.path = "POST", "/api/v1/auth/setup"
	extra.body = map[string]string{"password": "hunter2hunter2", "setup_token": s.auth.SetupToken()}
	rec = s.do(extra)
	expect(s.t, rec, 200, "")
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			cookie = c.Value
		}
	}
	csrf, _ = decode(s.t, rec)["csrf_token"].(string)
	return cookie, csrf, rec
}

func TestPasswordModeAccess(t *testing.T) {
	s := newServer(t, serverOpts{mode: config.AuthPassword})

	// Before setup: only the public routes and the SPA answer.
	expect(t, s.do(req{method: "GET", path: "/api/v1/health"}), 200, "")
	expect(t, s.do(req{method: "GET", path: "/"}), 200, "")
	expect(t, s.do(req{method: "GET", path: "/api/v1/system/status"}), 401, "unauthorized")
	expect(t, s.do(req{method: "GET", path: "/api/v1/top/tracks"}), 401, "unauthorized")
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/login", body: map[string]string{"password": "whatever1"}}), 409, "setup_required")
	sess := decode(t, s.do(req{method: "GET", path: "/api/v1/auth/session"}))
	if sess["mode"] != "password" || sess["setup_required"] != true || sess["authenticated"] != false || sess["csrf_token"] != nil {
		t.Fatalf("anonymous session: %v", sess)
	}

	// First-run setup needs the one-time token from the server log (D21).
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/setup", body: map[string]string{"password": "hunter2hunter2"}}), 401, "invalid_setup_token")
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/setup",
		body: map[string]string{"password": "hunter2hunter2", "setup_token": "AAAAA-AAAAA-AAAAA-AAAAA"}}), 401, "invalid_setup_token")
	cookie, csrf, rec := s.setup(req{})
	c := rec.Result().Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Secure || c.Path != "/" || c.MaxAge != int(auth.SessionMaxAge.Seconds()) {
		t.Fatalf("cookie attributes over HTTP: %+v", c)
	}
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/setup", body: map[string]string{"password": "other-password"}}), 409, "setup_done")

	// Signed in.
	expect(t, s.do(req{method: "GET", path: "/api/v1/top/tracks", cookie: cookie}), 501, "not_implemented")
	st := decode(t, s.do(req{method: "GET", path: "/api/v1/system/status", cookie: cookie}))
	if st["auth"].(map[string]any)["mode"] != "password" || st["spotify"].(map[string]any)["state"] != "not_configured" {
		t.Fatalf("status: %v", st)
	}
	sess = decode(t, s.do(req{method: "GET", path: "/api/v1/auth/session", cookie: cookie}))
	if sess["authenticated"] != true || sess["method"] != "session" || sess["csrf_token"] != csrf || sess["expires_at"] == nil {
		t.Fatalf("session: %v", sess)
	}

	// CSRF: unsafe requests need the session's token.
	tokenBody := map[string]string{"name": "cli"}
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/tokens", body: tokenBody, cookie: cookie}), 403, "csrf_failed")
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/tokens", body: tokenBody, cookie: cookie, csrf: "wrong"}), 403, "csrf_failed")
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/tokens", body: tokenBody, cookie: cookie, csrf: csrf}), 201, "")

	// Cross-site unsafe requests are rejected before auth, even with a valid token.
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/tokens", body: tokenBody, cookie: cookie, csrf: csrf,
		header: map[string]string{"Sec-Fetch-Site": "cross-site"}}), 403, "cross_origin")
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/login", body: map[string]string{"password": "x"},
		header: map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}}), 403, "cross_origin")

	// Logout ends the session and clears the cookie.
	rec = s.do(req{method: "POST", path: "/api/v1/auth/logout", cookie: cookie, csrf: csrf})
	expect(t, rec, 204, "")
	if c := rec.Result().Cookies(); len(c) != 1 || c[0].MaxAge >= 0 {
		t.Fatalf("logout cookie: %+v", c)
	}
	expect(t, s.do(req{method: "GET", path: "/api/v1/system/status", cookie: cookie}), 401, "unauthorized")
}

func TestSessionCookieSecureOverHTTPS(t *testing.T) {
	cases := []struct {
		name   string
		r      req
		secure bool
	}{
		{"direct TLS", req{tls: true}, true},
		{"trusted proxy https", req{remote: "10.0.0.2:1", header: map[string]string{"X-Forwarded-Proto": "https"}}, true},
		{"untrusted proxy header", req{remote: "203.0.113.1:1", header: map[string]string{"X-Forwarded-Proto": "https"}}, false},
	}
	for _, c := range cases {
		s := newServer(t, serverOpts{mode: config.AuthPassword, proxies: "10.0.0.0/8"})
		_, _, rec := s.setup(c.r)
		if got := rec.Result().Cookies()[0].Secure; got != c.secure {
			t.Errorf("%s: Secure=%v", c.name, got)
		}
	}
}

func TestSessionExpiryOverHTTP(t *testing.T) {
	s := newServer(t, serverOpts{mode: config.AuthPassword})
	cookie, _, _ := s.setup(req{})
	s.clock.add(auth.SessionIdleTTL + time.Minute)
	expect(t, s.do(req{method: "GET", path: "/api/v1/system/status", cookie: cookie}), 401, "unauthorized")
	if decode(t, s.do(req{method: "GET", path: "/api/v1/auth/session", cookie: cookie}))["authenticated"] != false {
		t.Fatal("expired session still authenticated")
	}
}

func TestLoginRateLimitOverHTTP(t *testing.T) {
	s := newServer(t, serverOpts{mode: config.AuthPassword, proxies: "10.0.0.0/8"})
	s.setup(req{})
	bad := map[string]string{"password": "wrong-password"}
	// Behind a trusted proxy, the limit is per forwarded client.
	client := func(ip string) req {
		return req{method: "POST", path: "/api/v1/auth/login", body: bad, remote: "10.0.0.2:1",
			header: map[string]string{"X-Forwarded-For": ip}}
	}
	for range 5 { // the per-client burst
		expect(t, s.do(client("198.51.100.7")), 401, "invalid_password")
	}
	rec := s.do(client("198.51.100.7"))
	expect(t, rec, 429, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	expect(t, s.do(client("198.51.100.8")), 401, "invalid_password")
}

func TestAPITokensAreReadOnly(t *testing.T) {
	for _, mode := range []config.AuthMode{config.AuthPassword, config.AuthProxy, config.AuthNone} {
		t.Run(string(mode), func(t *testing.T) {
			s := newServer(t, serverOpts{mode: mode, proxies: "10.0.0.0/8"})
			// Create a token as the owner in this mode.
			owner := req{method: "POST", path: "/api/v1/auth/tokens", body: map[string]string{"name": "mcp"}}
			switch mode {
			case config.AuthPassword:
				owner.cookie, owner.csrf, _ = s.setup(req{})
			case config.AuthProxy:
				owner.remote = "10.0.0.2:1"
				owner.header = map[string]string{"Remote-User": "alice"}
				sess := decode(t, s.do(req{method: "GET", path: "/api/v1/auth/session", remote: owner.remote, header: owner.header}))
				owner.csrf = sess["csrf_token"].(string)
			}
			rec := s.do(owner)
			expect(t, rec, 201, "")
			created := decode(t, rec)
			secret := created["secret"].(string)
			id := created["token"].(map[string]any)["id"].(string)

			expect(t, s.do(req{method: "GET", path: "/api/v1/system/status", bearer: secret}), 200, "")
			expect(t, s.do(req{method: "GET", path: "/api/v1/spotify/connection", bearer: secret}), 200, "")
			expect(t, s.do(req{method: "PUT", path: "/api/v1/spotify/client-id", bearer: secret,
				body: map[string]string{"client_id": testClientID}}), 403, "read_only_token")
			expect(t, s.do(req{method: "DELETE", path: "/api/v1/auth/tokens/" + id, bearer: secret}), 403, "read_only_token")
			expect(t, s.do(req{method: "PATCH", path: "/api/v1/settings", bearer: secret, body: map[string]string{"week_start": "sunday"}}), 403, "read_only_token")
			expect(t, s.do(req{method: "GET", path: "/api/v1/auth/tokens", bearer: secret}), 403, "forbidden")
			expect(t, s.do(req{method: "GET", path: "/api/v1/system/backup", bearer: secret}), 403, "forbidden")
			sess := decode(t, s.do(req{method: "GET", path: "/api/v1/auth/session", bearer: secret}))
			if sess["method"] != "token" || sess["user"] != "mcp" {
				t.Fatalf("token session: %v", sess)
			}

			rec = s.do(req{method: "GET", path: "/api/v1/system/status", bearer: "spt_bogus"})
			expect(t, rec, 401, "invalid_token")
			if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "invalid_token") {
				t.Fatal("missing WWW-Authenticate")
			}
			// A bad token does not block public routes.
			expect(t, s.do(req{method: "GET", path: "/api/v1/health", bearer: "spt_bogus"}), 200, "")

			// Revoke as the owner.
			owner.method, owner.path, owner.body = "DELETE", "/api/v1/auth/tokens/"+id, nil
			expect(t, s.do(owner), 204, "")
			expect(t, s.do(req{method: "GET", path: "/api/v1/system/status", bearer: secret}), 401, "invalid_token")
		})
	}
}

func TestProxyMode(t *testing.T) {
	s := newServer(t, serverOpts{mode: config.AuthProxy, proxies: "10.0.0.0/8"})
	trusted := map[string]string{"Remote-User": "alice"}
	expect(t, s.do(req{method: "GET", path: "/api/v1/system/status", remote: "10.1.1.1:1", header: trusted}), 200, "")
	// The same header from anywhere else is ignored.
	expect(t, s.do(req{method: "GET", path: "/api/v1/system/status", remote: "203.0.113.9:1", header: trusted}), 401, "unauthorized")
	expect(t, s.do(req{method: "GET", path: "/api/v1/system/status", remote: "10.1.1.1:1"}), 401, "unauthorized")

	sess := decode(t, s.do(req{method: "GET", path: "/api/v1/auth/session", remote: "10.1.1.1:1", header: trusted}))
	if sess["mode"] != "proxy" || sess["user"] != "alice" || sess["setup_required"] != false || sess["csrf_token"] == nil {
		t.Fatalf("proxy session: %v", sess)
	}
	body := map[string]string{"client_id": testClientID}
	expect(t, s.do(req{method: "PUT", path: "/api/v1/spotify/client-id", remote: "10.1.1.1:1", header: trusted, body: body}), 403, "csrf_failed")
	expect(t, s.do(req{method: "PUT", path: "/api/v1/spotify/client-id", remote: "10.1.1.1:1", header: trusted, body: body,
		csrf: sess["csrf_token"].(string)}), 200, "")
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/login", body: map[string]string{"password": "whatever1"}}), 409, "wrong_auth_mode")
}

func TestNoneMode(t *testing.T) {
	s := newServer(t, serverOpts{mode: config.AuthNone})
	st := decode(t, s.do(req{method: "GET", path: "/api/v1/system/status"}))
	if st["auth"].(map[string]any)["mode"] != "none" {
		t.Fatalf("status: %v", st)
	}
	// No CSRF token needed, but cross-site requests are still refused.
	expect(t, s.do(req{method: "PUT", path: "/api/v1/spotify/client-id", body: map[string]string{"client_id": testClientID}}), 200, "")
	expect(t, s.do(req{method: "PUT", path: "/api/v1/spotify/client-id", body: map[string]string{"client_id": testClientID},
		header: map[string]string{"Sec-Fetch-Site": "cross-site"}}), 403, "cross_origin")
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/setup", body: map[string]string{"password": "hunter2hunter2"}}), 409, "wrong_auth_mode")
	expect(t, s.do(req{method: "POST", path: "/api/v1/auth/logout"}), 204, "")
}

func TestRedirectURIDerivation(t *testing.T) {
	get := func(s *server, r req) map[string]any {
		r.method, r.path = "GET", "/api/v1/spotify/connection"
		rec := s.do(r)
		expect(t, rec, 200, "")
		return decode(t, rec)["callback"].(map[string]any)
	}
	s := newServer(t, serverOpts{mode: config.AuthNone, proxies: "10.0.0.0/8"})
	fwd := map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "stats.example.com"}
	for name, c := range map[string]struct {
		r          req
		mode, want string
	}{
		"plain http":      {req{}, "paste_back", "http://127.0.0.1:8080/oauth/spotify/callback"},
		"loopback http":   {req{host: "127.0.0.1:8080"}, "direct", "http://127.0.0.1:8080/oauth/spotify/callback"},
		"loopback v6":     {req{host: "[::1]:9999"}, "paste_back", "http://127.0.0.1:9999/oauth/spotify/callback"},
		"localhost http":  {req{host: "localhost:8080"}, "paste_back", "http://127.0.0.1:8080/oauth/spotify/callback"},
		"lan http":        {req{host: "192.168.1.5:8080"}, "paste_back", "http://127.0.0.1:8080/oauth/spotify/callback"},
		"direct tls":      {req{tls: true}, "direct", "https://example.com/oauth/spotify/callback"},
		"trusted proxy":   {req{remote: "10.0.0.3:1", header: fwd}, "direct", "https://stats.example.com/oauth/spotify/callback"},
		"untrusted proxy": {req{remote: "198.51.100.1:1", header: fwd}, "paste_back", "http://127.0.0.1:8080/oauth/spotify/callback"},
	} {
		cb := get(s, c.r)
		if cb["mode"] != c.mode || cb["redirect_uri"] != c.want {
			t.Errorf("%s: %v", name, cb)
		}
	}
	// BASE_URL wins over the request.
	b := newServer(t, serverOpts{mode: config.AuthNone, proxies: "10.0.0.0/8", baseURL: "https://music.example.org"})
	for _, r := range []req{{}, {remote: "10.0.0.3:1", header: fwd}} {
		if cb := get(b, r); cb["mode"] != "direct" || cb["redirect_uri"] != "https://music.example.org/oauth/spotify/callback" {
			t.Errorf("BASE_URL: %v", cb)
		}
	}
	h := newServer(t, serverOpts{mode: config.AuthNone, baseURL: "http://192.168.1.5:8080"})
	if cb := get(h, req{tls: true}); cb["mode"] != "paste_back" {
		t.Errorf("http BASE_URL: %v", cb)
	}
}

func TestSpotifyDirectCallback(t *testing.T) {
	s := newServer(t, serverOpts{mode: config.AuthPassword, baseURL: "https://stats.example.com"})
	cookie, csrf, _ := s.setup(req{tls: true})
	owner := func(method, path string, body any) *httptest.ResponseRecorder {
		return s.do(req{method: method, path: path, body: body, cookie: cookie, csrf: csrf, tls: true})
	}
	expect(t, owner("POST", "/api/v1/spotify/authorize", nil), 409, "client_id_required")
	expect(t, owner("PUT", "/api/v1/spotify/client-id", map[string]string{"client_id": testClientID}), 200, "")
	expect(t, owner("POST", "/api/v1/spotify/authorize", map[string]string{"return_to": "https://evil.com"}), 400, "bad_request")
	rec := owner("POST", "/api/v1/spotify/authorize", map[string]string{"return_to": "/setup?step=spotify"})
	expect(t, rec, 200, "")
	authz := decode(t, rec)
	if authz["callback"].(map[string]any)["mode"] != "direct" {
		t.Fatalf("authorize: %v", authz)
	}
	back, err := s.fake.Approve(authz["authorize_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(back)
	if u.Scheme+"://"+u.Host+u.Path != "https://stats.example.com/oauth/spotify/callback" {
		t.Fatalf("spotify redirected to %s", back)
	}
	// The browser follows the redirect to this instance (no API auth needed;
	// the single-use state authorizes it).
	rec = s.do(req{method: "GET", path: u.RequestURI(), tls: true})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/setup?step=spotify&spotify=connected" {
		t.Fatalf("callback: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	st := decode(t, owner("GET", "/api/v1/system/status", nil))["spotify"].(map[string]any)
	if st["state"] != "connected" || st["account_name"] != "Test Listener" || st["connected_at"] == nil {
		t.Fatalf("status after connect: %v", st)
	}
	// Replaying the callback fails without touching the connection.
	rec = s.do(req{method: "GET", path: u.RequestURI(), tls: true})
	if loc := rec.Header().Get("Location"); loc != "/?spotify=error&spotify_error=invalid_state" {
		t.Fatalf("replay: %s", loc)
	}
	// Denied consent returns to the wizard with the reason.
	s.fake.Set(func(f *spotifytest.Fake) { f.Deny = true })
	authz = decode(t, owner("POST", "/api/v1/spotify/authorize", map[string]string{"return_to": "/setup"}))
	back, _ = s.fake.Approve(authz["authorize_url"].(string))
	u, _ = url.Parse(back)
	if loc := s.do(req{method: "GET", path: u.RequestURI()}).Header().Get("Location"); loc != "/setup?spotify=error&spotify_error=spotify_denied" {
		t.Fatalf("denied: %s", loc)
	}
	// Disconnect.
	if decode(t, owner("DELETE", "/api/v1/spotify/connection", nil))["state"] != "not_configured" {
		t.Fatal("disconnect")
	}
}
