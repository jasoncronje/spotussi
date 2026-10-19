package spotify_test

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/keyring"
	"spotussi/internal/spotify"
	"spotussi/internal/spotify/spotifytest"
)

const clientID = "0123456789abcdef0123456789abcdef"

const loopbackURI = "http://127.0.0.1:8080/oauth/spotify/callback"

type env struct {
	t    *testing.T
	db   *db.DB
	keys *keyring.Keyring
	fake *spotifytest.Fake
	now  time.Time
	mu   sync.Mutex
}

func newEnv(t *testing.T) *env {
	d, err := db.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	k, _ := keyring.New(bytes.Repeat([]byte{3}, 32))
	f := spotifytest.New(clientID)
	t.Cleanup(f.Close)
	return &env{t: t, db: d, keys: k, fake: f, now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
}

func (e *env) clock() time.Time { e.mu.Lock(); defer e.mu.Unlock(); return e.now }
func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

func (e *env) client() *spotify.Client {
	return spotify.New(spotify.Options{DB: e.db, Keyring: e.keys, AccountsURL: e.fake.AccountsURL(),
		APIURL: e.fake.APIURL(), Now: e.clock})
}

// connect runs the whole paste-back flow and returns the client.
func (e *env) connect(c *spotify.Client) {
	e.t.Helper()
	ctx := context.Background()
	if err := c.SetClientID(ctx, clientID); err != nil {
		e.t.Fatal(err)
	}
	req, err := c.BeginAuth(ctx, loopbackURI, "/")
	if err != nil {
		e.t.Fatal(err)
	}
	back, err := e.fake.Approve(req.URL)
	if err != nil {
		e.t.Fatal(err)
	}
	p, err := spotify.ParseCallbackURL(back)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := c.Complete(ctx, p); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) storedRefreshToken() string {
	var sealed []byte
	if err := e.db.Read.QueryRow(`SELECT refresh_token FROM spotify_connection`).Scan(&sealed); err != nil {
		e.t.Fatal(err)
	}
	pt, err := e.keys.Open("spotify/refresh_token", sealed)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(pt)
}

func TestRedirect(t *testing.T) {
	for origin, want := range map[string][2]string{
		"https://stats.example.com":       {"direct", "https://stats.example.com/oauth/spotify/callback"},
		"https://stats.example.com:8443/": {"direct", "https://stats.example.com:8443/oauth/spotify/callback"},
		"https://example.com/spotussi":    {"direct", "https://example.com/spotussi/oauth/spotify/callback"},
		"https://spotussi.localhost":      {"direct", "https://spotussi.localhost/oauth/spotify/callback"},
		// Plain HTTP: always http://127.0.0.1:<port>/... with an explicit
		// port (Spotify rejects port-less loopback URIs); direct only when
		// opened on exactly that origin (D21).
		"http://127.0.0.1:8080":    {"direct", "http://127.0.0.1:8080/oauth/spotify/callback"},
		"http://127.0.0.1:9000/x/": {"direct", "http://127.0.0.1:9000/x/oauth/spotify/callback"},
		"http://127.0.0.1":         {"paste_back", "http://127.0.0.1:3000/oauth/spotify/callback"}, // port from PORT
		"http://192.168.1.10:8080": {"paste_back", "http://127.0.0.1:8080/oauth/spotify/callback"},
		"http://stats.example.com": {"paste_back", "http://127.0.0.1:3000/oauth/spotify/callback"},
		"http://localhost:8080":    {"paste_back", "http://127.0.0.1:8080/oauth/spotify/callback"},
		"http://[::1]:8080":        {"paste_back", "http://127.0.0.1:8080/oauth/spotify/callback"},
		"http://127.0.0.2:8080":    {"paste_back", "http://127.0.0.1:8080/oauth/spotify/callback"},
		"not a url":                {"paste_back", "http://127.0.0.1:3000/oauth/spotify/callback"},
	} {
		mode, uri := spotify.Redirect(origin, spotify.LoopbackPort{Default: 3000})
		if string(mode) != want[0] || uri != want[1] {
			t.Errorf("%s: got %s %s", origin, mode, uri)
		}
	}
	// Override wins; a browser on another port then can't receive it directly.
	lp := spotify.LoopbackPort{Default: 3000, Override: 18080}
	if mode, uri := spotify.Redirect("http://127.0.0.1:18080", lp); mode != spotify.CallbackDirect || uri != "http://127.0.0.1:18080/oauth/spotify/callback" {
		t.Errorf("override direct: %s %s", mode, uri)
	}
	if mode, uri := spotify.Redirect("http://127.0.0.1:8080", lp); mode != spotify.CallbackPasteBack || uri != "http://127.0.0.1:18080/oauth/spotify/callback" {
		t.Errorf("override paste-back: %s %s", mode, uri)
	}
	if _, uri := spotify.Redirect("http://192.168.1.10", spotify.LoopbackPort{}); uri != "http://127.0.0.1:8080/oauth/spotify/callback" {
		t.Errorf("no port anywhere: %s", uri)
	}
}

func TestParseCallbackURL(t *testing.T) {
	ok := map[string]spotify.CallbackParams{
		"http://127.0.0.1/oauth/spotify/callback?code=abc&state=xyz":          {Code: "abc", State: "xyz"},
		"  127.0.0.1/oauth/spotify/callback?state=xyz&code=abc  ":             {Code: "abc", State: "xyz"},
		"http://127.0.0.1:80/oauth/spotify/callback/?code=a&state=s":          {Code: "a", State: "s"},
		"http://127.0.0.1/oauth/spotify/callback?error=access_denied&state=s": {Error: "access_denied", State: "s"},
	}
	for raw, want := range ok {
		got, err := spotify.ParseCallbackURL(raw)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "hello", "http://127.0.0.1/other?code=a&state=s",
		"http://127.0.0.1/oauth/spotify/callback?code=a", "http://127.0.0.1/oauth/spotify/callback?state=s"} {
		if _, err := spotify.ParseCallbackURL(raw); !errors.Is(err, spotify.ErrInvalidCallbackURL) {
			t.Errorf("%q: expected ErrInvalidCallbackURL, got %v", raw, err)
		}
	}
}

func TestSafeReturnPath(t *testing.T) {
	for p, want := range map[string]bool{"/": true, "/setup?step=2": true, "//evil.com": false,
		"/\\evil.com": false, "https://evil.com": false, "": false, "relative": false} {
		if spotify.SafeReturnPath(p) != want {
			t.Errorf("%q", p)
		}
	}
}

func TestAuthorizeURLAndPKCE(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	if _, err := c.BeginAuth(ctx, loopbackURI, "/"); !errors.Is(err, spotify.ErrNoClientID) {
		t.Fatalf("without client ID: %v", err)
	}
	for _, bad := range []string{"", "short", strings.Repeat("g", 31), clientID + "0", "0123456789abcdef0123456789abcde!"} {
		if err := c.SetClientID(ctx, bad); !errors.Is(err, spotify.ErrInvalidClientID) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if err := c.SetClientID(ctx, " "+clientID+" "); err != nil {
		t.Fatal(err)
	}
	if id, _ := c.ClientID(ctx); id != clientID {
		t.Fatalf("stored client id %q", id)
	}
	if _, err := c.BeginAuth(ctx, loopbackURI, "//evil.com"); !errors.Is(err, spotify.ErrInvalidReturnPath) {
		t.Fatalf("unsafe return path: %v", err)
	}
	req, err := c.BeginAuth(ctx, "https://stats.example.com/oauth/spotify/callback", "/setup")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(req.URL)
	q := u.Query()
	if u.Path != "/authorize" || q.Get("client_id") != clientID || q.Get("response_type") != "code" ||
		q.Get("redirect_uri") != "https://stats.example.com/oauth/spotify/callback" ||
		q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 || len(q.Get("state")) < 40 ||
		q.Get("scope") != "user-read-playback-state user-read-recently-played playlist-read-private user-library-read" {
		t.Fatalf("authorize URL: %s", req.URL)
	}
	// The stored verifier hashes to the challenge sent to Spotify (S256).
	var verifier string
	e.db.Read.QueryRow(`SELECT verifier FROM spotify_auth_requests WHERE state = ?`, q.Get("state")).Scan(&verifier)
	if len(verifier) < 43 || len(verifier) > 128 || spotify.ChallengeFor(verifier) != q.Get("code_challenge") {
		t.Fatalf("verifier %q does not match challenge", verifier)
	}
	// RFC 7636 Appendix B test vector.
	if got := spotify.ChallengeFor("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("S256 challenge %q", got)
	}
}

func TestConnectViaPasteBackAndStatus(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	if st, _ := c.Status(ctx); st.State != spotify.StateNotConfigured {
		t.Fatalf("initial state %s", st.State)
	}
	e.connect(c)
	st, err := c.Status(ctx)
	if err != nil || st.State != spotify.StateConnected || st.AccountID != "listener42" || st.AccountName != "Test Listener" ||
		st.ClientID != clientID || len(st.Scopes) != 4 || st.ConnectedAt != e.clock().UnixMilli() {
		t.Fatalf("status: %+v %v", st, err)
	}
	// The refresh token is encrypted at rest and is a live token at Spotify.
	var raw []byte
	e.db.Read.QueryRow(`SELECT refresh_token FROM spotify_connection`).Scan(&raw)
	rt := e.storedRefreshToken()
	if bytes.Contains(raw, []byte(rt)) || !e.fake.ValidRefreshToken(rt) {
		t.Fatal("refresh token not encrypted, or not the one Spotify issued")
	}
	// The access token from the exchange is cached: no refresh needed.
	if u, err := c.Me(ctx); err != nil || u.ID != "listener42" || e.fake.Refreshes.Load() != 0 {
		t.Fatalf("me: %+v %v refreshes=%d", u, err, e.fake.Refreshes.Load())
	}
	if err := c.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ := c.Status(ctx); st.State != spotify.StateNotConfigured || st.ClientID != clientID {
		t.Fatalf("after disconnect: %+v", st)
	}
	if _, err := c.AccessToken(ctx); !errors.Is(err, spotify.ErrNotConnected) {
		t.Fatalf("access token after disconnect: %v", err)
	}
}

func TestStateValidation(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	c.SetClientID(ctx, clientID)

	if _, err := c.Complete(ctx, spotify.CallbackParams{State: "forged", Code: "x"}); !errors.Is(err, spotify.ErrInvalidState) {
		t.Fatalf("unknown state: %v", err)
	}

	// A state can be used once.
	req, _ := c.BeginAuth(ctx, loopbackURI, "/back")
	back, _ := e.fake.Approve(req.URL)
	p, _ := spotify.ParseCallbackURL(back)
	if ret, err := c.Complete(ctx, p); err != nil || ret != "/back" {
		t.Fatalf("complete: %q %v", ret, err)
	}
	if _, err := c.Complete(ctx, p); !errors.Is(err, spotify.ErrInvalidState) {
		t.Fatalf("replayed state: %v", err)
	}

	// Expired state.
	req, _ = c.BeginAuth(ctx, loopbackURI, "/")
	back, _ = e.fake.Approve(req.URL)
	p, _ = spotify.ParseCallbackURL(back)
	e.advance(16 * time.Minute)
	if _, err := c.Complete(ctx, p); !errors.Is(err, spotify.ErrInvalidState) {
		t.Fatalf("expired state: %v", err)
	}

	// Denied consent.
	e.fake.Set(func(f *spotifytest.Fake) { f.Deny = true })
	req, _ = c.BeginAuth(ctx, loopbackURI, "/")
	back, _ = e.fake.Approve(req.URL)
	p, _ = spotify.ParseCallbackURL(back)
	var de *spotify.DeniedError
	if _, err := c.Complete(ctx, p); !errors.As(err, &de) || de.Code != "access_denied" {
		t.Fatalf("denied: %v", err)
	}
	e.fake.Set(func(f *spotifytest.Fake) { f.Deny = false })

	// A wrong PKCE verifier is rejected by Spotify (proves the verifier is
	// what the exchange sends).
	req, _ = c.BeginAuth(ctx, loopbackURI, "/")
	back, _ = e.fake.Approve(req.URL)
	p, _ = spotify.ParseCallbackURL(back)
	e.db.Write.Exec(`UPDATE spotify_auth_requests SET verifier = 'tampered-verifier-tampered-verifier-tampered' WHERE state = ?`, p.State)
	var ae *spotify.APIError
	if _, err := c.Complete(ctx, p); !errors.As(err, &ae) || ae.Code != "invalid_grant" {
		t.Fatalf("tampered verifier: %v", err)
	}
}

func TestMeFailureDoesNotConnect(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	e.fake.Set(func(f *spotifytest.Fake) { f.MeStatus = 403 })
	ctx := context.Background()
	c.SetClientID(ctx, clientID)
	req, _ := c.BeginAuth(ctx, loopbackURI, "/")
	back, _ := e.fake.Approve(req.URL)
	p, _ := spotify.ParseCallbackURL(back)
	_, err := c.Complete(ctx, p)
	if err == nil || !strings.Contains(err.Error(), "User not registered") {
		t.Fatalf("expected /me error, got %v", err)
	}
	if st, _ := c.Status(ctx); st.State != spotify.StateNotConfigured {
		t.Fatalf("state %s", st.State)
	}
}

func TestRefreshRotationPersists(t *testing.T) {
	e := newEnv(t)
	e.fake.Set(func(f *spotifytest.Fake) { f.RotateRefresh = true })
	c := e.client()
	ctx := context.Background()
	e.connect(c)
	first := e.storedRefreshToken()

	// Spotify rejects the cached access token: Do refreshes and retries.
	e.fake.ExpireAccessTokens()
	if u, err := c.Me(ctx); err != nil || u.ID != "listener42" {
		t.Fatalf("me after expiry: %v", err)
	}
	second := e.storedRefreshToken()
	if second == first || e.fake.ValidRefreshToken(first) || !e.fake.ValidRefreshToken(second) {
		t.Fatal("rotated refresh token was not persisted")
	}

	// Natural expiry: the access token is refreshed ahead of expires_in.
	e.advance(59*time.Minute + time.Second)
	if _, err := c.AccessToken(ctx); err != nil {
		t.Fatal(err)
	}
	third := e.storedRefreshToken()
	if third == second || e.fake.Refreshes.Load() != 2 {
		t.Fatalf("expected a second rotation, refreshes=%d", e.fake.Refreshes.Load())
	}

	// A new process (empty cache) refreshes with the persisted token.
	if _, err := e.client().Me(ctx); err != nil {
		t.Fatalf("after restart: %v", err)
	}
	var refreshedAt int64
	e.db.Read.QueryRow(`SELECT refreshed_at FROM spotify_connection`).Scan(&refreshedAt)
	if refreshedAt != e.clock().UnixMilli() {
		t.Fatal("refreshed_at not updated")
	}
}

func TestRefreshIsSingleFlight(t *testing.T) {
	e := newEnv(t)
	e.fake.Set(func(f *spotifytest.Fake) { f.RotateRefresh = true })
	e.connect(e.client())
	e.fake.Set(func(f *spotifytest.Fake) { f.RefreshDelay = 50 * time.Millisecond })

	c := e.client() // empty cache: every caller needs a refresh
	var wg sync.WaitGroup
	tokens := make([]string, 16)
	errs := make([]error, 16)
	for i := range tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tokens[i], errs[i] = c.AccessToken(context.Background())
		}()
	}
	wg.Wait()
	for i := range tokens {
		if errs[i] != nil || tokens[i] != tokens[0] {
			t.Fatalf("caller %d: %q %v", i, tokens[i], errs[i])
		}
	}
	if n := e.fake.Refreshes.Load(); n != 1 {
		t.Fatalf("%d refreshes, want 1", n)
	}
	// Waiters can give up.
	e.fake.Set(func(f *spotifytest.Fake) { f.RefreshDelay = 200 * time.Millisecond })
	c2 := e.client()
	go c2.AccessToken(context.Background())
	time.Sleep(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c2.AccessToken(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter: %v", err)
	}
}

func TestRevokedRefreshTokenNeedsReconnect(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	e.connect(c)
	e.fake.RevokeRefreshTokens()
	e.fake.ExpireAccessTokens()
	if _, err := c.Me(ctx); !errors.Is(err, spotify.ErrReconnectRequired) {
		t.Fatalf("expected reconnect, got %v", err)
	}
	st, _ := c.Status(ctx)
	if st.State != spotify.StateReconnectRequired || !strings.Contains(st.LastError, "invalid_grant") {
		t.Fatalf("status %+v", st)
	}
	n := e.fake.Refreshes.Load()
	if _, err := c.AccessToken(ctx); !errors.Is(err, spotify.ErrReconnectRequired) || e.fake.Refreshes.Load() != n {
		t.Fatal("must not keep hammering Spotify once reconnect is required")
	}
	// Reconnecting clears the flag.
	e.connect(c)
	if st, _ := c.Status(ctx); st.State != spotify.StateConnected || st.LastError != "" {
		t.Fatalf("after reconnect %+v", st)
	}
}

func TestLostKeyNeedsReconnect(t *testing.T) {
	e := newEnv(t)
	e.connect(e.client())
	e.keys, _ = keyring.New(bytes.Repeat([]byte{9}, 32))
	if _, err := e.client().AccessToken(context.Background()); !errors.Is(err, spotify.ErrReconnectRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestChangingClientIDDropsConnection(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	ctx := context.Background()
	e.connect(c)
	if err := c.SetClientID(ctx, clientID); err != nil { // same ID: kept
		t.Fatal(err)
	}
	if st, _ := c.Status(ctx); st.State != spotify.StateConnected {
		t.Fatal("same client ID must keep the connection")
	}
	if err := c.SetClientID(ctx, strings.Repeat("f", 32)); err != nil {
		t.Fatal(err)
	}
	if st, _ := c.Status(ctx); st.State != spotify.StateNotConfigured {
		t.Fatal("new client ID must drop the connection")
	}
}
