package auth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"spotussi/internal/config"
	"spotussi/internal/db"
	"spotussi/internal/keyring"
	"spotussi/internal/trust"
)

var fast = Params{MemoryKiB: 64, Time: 1, Threads: 1}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newService(t *testing.T, mode config.AuthMode, proxies string) (*Service, *clock) {
	t.Helper()
	d, err := db.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	k, _ := keyring.New(bytes.Repeat([]byte{1}, 32))
	p, err := trust.Parse(proxies)
	if err != nil {
		t.Fatal(err)
	}
	c := &clock{t: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	return New(Options{DB: d, Keyring: k, Mode: mode, Proxies: p, Now: c.now, Hash: fast}), c
}

var ip = netip.MustParseAddr("192.0.2.1")

func withCookie(token string) *http.Request {
	r := httptest.NewRequest("GET", "/api/v1/x", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	return r
}

func TestPasswordHashing(t *testing.T) {
	h := hashPassword("correct horse", DefaultParams)
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected hash format %q", h)
	}
	if ok, err := verifyPassword(h, "correct horse"); !ok || err != nil {
		t.Fatalf("verify: %v %v", ok, err)
	}
	if ok, _ := verifyPassword(h, "wrong horse"); ok {
		t.Fatal("wrong password verified")
	}
	if h2 := hashPassword("correct horse", DefaultParams); h2 == h {
		t.Fatal("salt not random")
	}
	for _, bad := range []string{"", "$argon2i$v=19$m=1,t=1,p=1$AA$AA", "$argon2id$v=19$m=x$AA$AA", "$argon2id$v=19$m=1,t=1,p=1$!!$AA"} {
		if _, err := verifyPassword(bad, "x"); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestSetupLoginLogout(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t, config.AuthPassword, "")
	if req, _ := s.SetupRequired(ctx); !req {
		t.Fatal("fresh instance must require setup")
	}
	if _, err := s.Login(ctx, ip, "whatever1"); !errors.Is(err, ErrSetupRequired) {
		t.Fatalf("login before setup: %v", err)
	}
	if _, err := s.Setup(ctx, ip, "short", s.SetupToken()); !errors.Is(err, ErrPasswordPolicy) {
		t.Fatalf("short password: %v", err)
	}
	// The setup token is required (D21); formatting is forgiven.
	for _, bad := range []string{"", "nope", strings.Repeat("0", 20)} {
		if _, err := s.Setup(ctx, netip.MustParseAddr("192.0.2.50"), "hunter2hunter2", bad); !errors.Is(err, ErrSetupToken) {
			t.Fatalf("setup token %q: %v", bad, err)
		}
	}
	tok := s.SetupToken()
	if len(tok) != 23 || strings.Count(tok, "-") != 3 {
		t.Fatalf("setup token format %q", tok)
	}
	if req, _ := s.SetupRequired(ctx); !req {
		t.Fatal("failed setup attempts must leave setup pending")
	}
	sess, err := s.Setup(ctx, ip, "hunter2hunter2", " "+strings.ToLower(strings.ReplaceAll(tok, "-", ""))+" ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Setup(ctx, ip, "another-password", s.SetupToken()); !errors.Is(err, ErrSetupDone) {
		t.Fatalf("second setup: %v", err)
	}
	p, err := s.Authenticate(withCookie(sess.Token))
	if err != nil || p == nil || p.Method != MethodSession || p.CSRF != sess.CSRF {
		t.Fatalf("session auth: %+v %v", p, err)
	}
	if !p.CheckCSRF(sess.CSRF) || p.CheckCSRF("nope") || p.CheckCSRF("") {
		t.Fatal("csrf check")
	}
	if _, err := s.Login(ctx, ip, "wrong-password"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	sess2, err := s.Login(ctx, ip, "hunter2hunter2")
	if err != nil || sess2.Token == sess.Token {
		t.Fatalf("login: %v", err)
	}
	if err := s.Logout(ctx, p); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Authenticate(withCookie(sess.Token)); p != nil {
		t.Fatal("logged-out session still valid")
	}
	if p, _ := s.Authenticate(withCookie(sess2.Token)); p == nil {
		t.Fatal("other session must survive logout")
	}
	if p, _ := s.Authenticate(withCookie("forged")); p != nil {
		t.Fatal("forged cookie accepted")
	}
	if err := s.ResetPassword(ctx); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Authenticate(withCookie(sess2.Token)); p != nil {
		t.Fatal("reset must end sessions")
	}
	if req, _ := s.SetupRequired(ctx); !req {
		t.Fatal("reset must re-enable setup")
	}
}

func TestConcurrentSetupOnlyOneWins(t *testing.T) {
	s, _ := newService(t, config.AuthPassword, "")
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Setup(context.Background(), netip.AddrFrom4([4]byte{10, 0, 0, byte(i)}), "password-number-x", s.SetupToken()); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d setups succeeded", wins)
	}
}

func TestSessionExpiry(t *testing.T) {
	ctx := context.Background()
	s, c := newService(t, config.AuthPassword, "")
	sess, err := s.Setup(ctx, ip, "hunter2hunter2", s.SetupToken())
	if err != nil {
		t.Fatal(err)
	}
	// Idle expiry: unused for longer than the idle TTL.
	c.add(SessionIdleTTL + time.Second)
	if p, _ := s.Authenticate(withCookie(sess.Token)); p != nil {
		t.Fatal("idle session should have expired")
	}

	// Sliding renewal keeps an active session alive up to the max age.
	sess, _ = s.Login(ctx, ip, "hunter2hunter2")
	for elapsed := time.Duration(0); elapsed < SessionMaxAge-SessionIdleTTL; elapsed += 20 * 24 * time.Hour {
		c.add(20 * 24 * time.Hour)
		if p, _ := s.Authenticate(withCookie(sess.Token)); p == nil {
			t.Fatalf("active session expired after %s", elapsed)
		}
	}
	c.add(SessionMaxAge) // well past the absolute cap
	if p, _ := s.Authenticate(withCookie(sess.Token)); p != nil {
		t.Fatal("session outlived its absolute max age")
	}
}

func TestLoginRateLimit(t *testing.T) {
	ctx := context.Background()
	s, c := newService(t, config.AuthPassword, "")
	if _, err := s.Setup(ctx, ip, "hunter2hunter2", s.SetupToken()); err != nil {
		t.Fatal(err)
	}
	other := netip.MustParseAddr("192.0.2.99")
	var rl *RateLimitError
	n := 0
	for ; n < 20; n++ {
		if _, err := s.Login(ctx, other, "bad-guess"); errors.As(err, &rl) {
			break
		}
	}
	if n != 5 || rl.RetryAfter <= 0 {
		t.Fatalf("limited after %d attempts (retry %v)", n, rl)
	}
	// Even the right password is refused while limited.
	if _, err := s.Login(ctx, other, "hunter2hunter2"); !errors.As(err, &rl) {
		t.Fatalf("expected rate limit, got %v", err)
	}
	// Other clients are unaffected.
	if _, err := s.Login(ctx, netip.MustParseAddr("192.0.2.100"), "hunter2hunter2"); err != nil {
		t.Fatal(err)
	}
	c.add(rl.RetryAfter)
	if _, err := s.Login(ctx, other, "hunter2hunter2"); err != nil {
		t.Fatalf("after waiting: %v", err)
	}
}

func TestGlobalRateLimit(t *testing.T) {
	l := NewLimiter((&clock{t: time.Unix(0, 0)}).now)
	allowed := 0
	for i := range 100 {
		if ok, _ := l.Allow(netip.AddrFrom4([4]byte{10, 0, byte(i / 256), byte(i)})); ok {
			allowed++
		}
	}
	if allowed != 20 {
		t.Fatalf("global burst allowed %d", allowed)
	}
}

func TestModes(t *testing.T) {
	ctx := context.Background()
	none, _ := newService(t, config.AuthNone, "")
	if p, err := none.Authenticate(httptest.NewRequest("GET", "/", nil)); err != nil || p == nil || p.Method != MethodNone || p.CSRF != "" {
		t.Fatalf("none: %+v %v", p, err)
	}
	if _, err := none.Setup(ctx, ip, "hunter2hunter2", none.SetupToken()); !errors.Is(err, ErrWrongMode) {
		t.Fatalf("setup in none mode: %v", err)
	}
	if req, _ := none.SetupRequired(ctx); req {
		t.Fatal("none mode never needs setup")
	}

	proxy, _ := newService(t, config.AuthProxy, "10.0.0.0/8")
	req := func(remote, user string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if user != "" {
			r.Header.Set("Remote-User", user)
		}
		return r
	}
	p, err := proxy.Authenticate(req("10.0.0.5:1234", "alice"))
	if err != nil || p == nil || p.Method != MethodProxy || p.User != "alice" || p.CSRF == "" {
		t.Fatalf("trusted proxy: %+v %v", p, err)
	}
	if p2, _ := proxy.Authenticate(req("10.0.0.6:1", "alice")); p2.CSRF != p.CSRF {
		t.Fatal("proxy CSRF token must be stable per user")
	}
	if p, _ := proxy.Authenticate(req("203.0.113.5:1234", "alice")); p != nil {
		t.Fatal("header from untrusted peer must be ignored")
	}
	if p, _ := proxy.Authenticate(req("10.0.0.5:1234", "")); p != nil {
		t.Fatal("trusted peer without header is anonymous")
	}

	pw, _ := newService(t, config.AuthPassword, "")
	if p, err := pw.Authenticate(httptest.NewRequest("GET", "/", nil)); p != nil || err != nil {
		t.Fatalf("anonymous: %+v %v", p, err)
	}
}

func TestAPITokens(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []config.AuthMode{config.AuthPassword, config.AuthProxy, config.AuthNone} {
		s, c := newService(t, mode, "10.0.0.0/8")
		if _, _, err := s.CreateToken(ctx, "  "); !errors.Is(err, ErrInvalidTokenName) {
			t.Fatalf("empty name: %v", err)
		}
		tok, secret, err := s.CreateToken(ctx, "claude desktop")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(secret, TokenPrefix) || !strings.HasPrefix(secret, tok.Prefix) || len(secret) < 40 {
			t.Fatalf("secret %q prefix %q", secret, tok.Prefix)
		}
		var stored int
		s.db.Read.QueryRow(`SELECT count(*) FROM api_tokens WHERE token_hash = ?`, secret).Scan(&stored)
		if stored != 0 {
			t.Fatal("token stored in plaintext")
		}
		bearer := func(v string) *http.Request {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", v)
			r.AddCookie(&http.Cookie{Name: CookieName, Value: "ignored"})
			return r
		}
		p, err := s.Authenticate(bearer("Bearer " + secret))
		if err != nil || p.Method != MethodToken || p.TokenID != tok.ID || p.Browser() {
			t.Fatalf("%s: bearer: %+v %v", mode, p, err)
		}
		for _, bad := range []string{"Bearer spt_nope", "Bearer " + secret + "x", "Basic abc", "Bearer"} {
			if _, err := s.Authenticate(bearer(bad)); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("%s: %q: %v", mode, bad, err)
			}
		}
		c.add(time.Hour)
		list, err := s.ListTokens(ctx)
		if err != nil || len(list) != 1 || list[0].LastUsedAt == nil || list[0].Name != "claude desktop" {
			t.Fatalf("list: %+v %v", list, err)
		}
		if err := s.RevokeToken(ctx, tok.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.RevokeToken(ctx, tok.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("double revoke: %v", err)
		}
		if _, err := s.Authenticate(bearer("Bearer " + secret)); !errors.Is(err, ErrInvalidToken) {
			t.Fatal("revoked token still works")
		}
	}
}
