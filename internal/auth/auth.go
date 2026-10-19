// Package auth implements instance access control (D13): the owner
// password (argon2id), browser sessions, proxy forward-auth, and read-only
// API tokens. It decides who a request is; internal/api enforces what
// that identity may do.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"spotussi/internal/config"
	"spotussi/internal/db"
	"spotussi/internal/keyring"
	"spotussi/internal/trust"
)

// CookieName is the session cookie.
const CookieName = "spotussi_session"

// Session lifetime: sliding idle expiry, capped by an absolute maximum.
const (
	SessionIdleTTL = 30 * 24 * time.Hour
	SessionMaxAge  = 90 * 24 * time.Hour
	// sessionTouchEvery limits session writes to one per hour per session.
	sessionTouchEvery = time.Hour
	// tokenTouchEvery limits last_used_at writes per API token.
	tokenTouchEvery = time.Minute
)

// TokenPrefix starts every API token, so leaked tokens are recognisable
// (and greppable by secret scanners).
const TokenPrefix = "spt_"

const ownerPasswordKey = "owner_password"

// Errors returned by the service.
var (
	ErrSetupDone        = errors.New("the owner password is already set")
	ErrSetupRequired    = errors.New("no owner password is set yet")
	ErrWrongMode        = errors.New("not available in this auth mode")
	ErrInvalidPassword  = errors.New("invalid password")
	ErrInvalidToken     = errors.New("invalid API token")
	ErrNotFound         = errors.New("not found")
	ErrPasswordPolicy   = fmt.Errorf("password must be %d to %d bytes", MinPasswordLen, MaxPasswordLen)
	ErrInvalidTokenName = errors.New("token name must be 1 to 100 characters")
	ErrSetupToken       = errors.New("wrong or missing setup token: copy it from the server log (e.g. docker logs <container>)")
)

// RateLimitError means too many password attempts; retry after RetryAfter.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("too many attempts; retry in %s", e.RetryAfter)
}

// Method is how a request was authenticated.
type Method string

const (
	MethodSession Method = "session" // password mode cookie
	MethodProxy   Method = "proxy"   // trusted forward-auth header
	MethodNone    Method = "none"    // AUTH_MODE=none: everyone is the owner
	MethodToken   Method = "token"   // Authorization: Bearer API token
)

// Principal is an authenticated identity.
type Principal struct {
	Method Method
	// User is "owner" (session), the proxy user name, or the token name.
	User string
	// CSRF is the token unsafe browser requests must echo in X-CSRF-Token
	// (session and proxy methods; empty otherwise).
	CSRF string
	// ExpiresAt is the session expiry (epoch ms; session method only).
	ExpiresAt int64
	// TokenID is the API token's id (token method only).
	TokenID string

	sessionHash string
}

// Browser reports whether the principal is a browser login (any method
// except an API token).
func (p *Principal) Browser() bool { return p.Method != MethodToken }

// CheckCSRF reports whether token is the principal's CSRF token. Methods
// without a token (none, API tokens) always pass; cross-origin requests
// are still rejected by the HTTP layer's origin check.
func (p *Principal) CheckCSRF(token string) bool {
	if p.CSRF == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(p.CSRF), []byte(token)) == 1
}

type ctxKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the request's principal, or nil.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxKey{}).(*Principal)
	return p
}

// Options configure a Service.
type Options struct {
	DB      *db.DB
	Keyring *keyring.Keyring
	Mode    config.AuthMode
	// ProxyHeader carries the user name in proxy mode (default Remote-User).
	ProxyHeader string
	Proxies     trust.Proxies
	// Now defaults to time.Now.
	Now func() time.Time
	// Hash defaults to DefaultParams (tests use cheaper ones).
	Hash Params
	// AnnounceSetupToken shows the first-run setup token to the operator
	// (D21). It is called once per process, the first time setup is found
	// to be pending. Defaults to a slog warning.
	AnnounceSetupToken func(token string)
}

// Service authenticates requests and manages credentials.
type Service struct {
	db      *db.DB
	keys    *keyring.Keyring
	mode    config.AuthMode
	header  string
	proxies trust.Proxies
	now     func() time.Time
	hash    Params
	limiter *Limiter
	hashing chan struct{} // bounds concurrent argon2 runs (memory)

	// setupToken must accompany first-run setup (D21). It is random per
	// process and only ever shown in the server log, so whoever first
	// reaches a freshly exposed instance cannot claim it without access
	// to the logs.
	setupToken string
	announce   func(string)
	announced  atomic.Bool
}

// New builds a Service.
func New(o Options) *Service {
	s := &Service{db: o.DB, keys: o.Keyring, mode: o.Mode, header: o.ProxyHeader, proxies: o.Proxies,
		now: o.Now, hash: o.Hash, hashing: make(chan struct{}, 2)}
	if s.mode == "" {
		s.mode = config.AuthPassword
	}
	if s.header == "" {
		s.header = config.DefaultProxyAuthHeader
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.hash == (Params{}) {
		s.hash = DefaultParams
	}
	s.limiter = NewLimiter(s.now)
	s.setupToken = newSetupToken()
	s.announce = o.AnnounceSetupToken
	if s.announce == nil {
		s.announce = func(tok string) {
			slog.Warn("first-run setup: enter this setup token in the browser to set the owner password", "setup_token", tok)
		}
	}
	return s
}

// setupAlphabet is Crockford base32 (no I, L, O, U): easy to read from a
// log and type.
const setupAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newSetupToken returns 20 random base32 characters (100 bits) in groups
// of five, e.g. 7KQ2M-XW9PD-0RT4H-ZC3NB.
func newSetupToken() string {
	b := make([]byte, 20)
	rand.Read(b)
	var sb strings.Builder
	for i, c := range b {
		if i > 0 && i%5 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteByte(setupAlphabet[int(c)%len(setupAlphabet)]) // 256 % 32 == 0: unbiased
	}
	return sb.String()
}

// normalizeSetupToken ignores case, spaces and dashes, so a token copied
// with stray formatting still matches.
func normalizeSetupToken(t string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '-' || r == ' ' || r == '\t':
			return -1
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		}
		return r
	}, t)
}

// SetupToken is this process's first-run setup token (for tests and the
// startup banner; it is never sent over HTTP).
func (s *Service) SetupToken() string { return s.setupToken }

// Mode is the configured auth mode.
func (s *Service) Mode() config.AuthMode { return s.mode }

// Proxies are the trusted reverse proxies.
func (s *Service) Proxies() trust.Proxies { return s.proxies }

// ---------- password ----------

// SetupRequired reports whether the instance is in password mode with no
// owner password yet.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	if s.mode != config.AuthPassword {
		return false, nil
	}
	_, err := s.passwordHash(ctx)
	if errors.Is(err, ErrSetupRequired) {
		// Announce once per process: at startup (serve calls this), or
		// later if `spotussi reset-password` ran while the server was up.
		if s.announced.CompareAndSwap(false, true) {
			s.announce(s.setupToken)
		}
		return true, nil
	}
	return false, err
}

func (s *Service) passwordHash(ctx context.Context) (string, error) {
	var h []byte
	err := s.db.Read.QueryRowContext(ctx, `SELECT value FROM secrets WHERE name = ?`, ownerPasswordKey).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSetupRequired
	}
	return string(h), err
}

func (s *Service) runHash(ctx context.Context, fn func()) error {
	select {
	case s.hashing <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.hashing }()
	fn()
	return nil
}

func (s *Service) allow(ip netip.Addr) error {
	if ok, wait := s.limiter.Allow(ip); !ok {
		return &RateLimitError{RetryAfter: wait}
	}
	return nil
}

// Setup sets the first owner password and starts a session. token must be
// this process's setup token (ErrSetupToken otherwise; attempts count
// against the login rate limit). It fails with ErrSetupDone once a
// password exists (the check and insert are atomic).
func (s *Service) Setup(ctx context.Context, ip netip.Addr, password, token string) (*Session, error) {
	if s.mode != config.AuthPassword {
		return nil, ErrWrongMode
	}
	if len(password) < MinPasswordLen || len(password) > MaxPasswordLen {
		return nil, ErrPasswordPolicy
	}
	if req, err := s.SetupRequired(ctx); err != nil {
		return nil, err
	} else if !req {
		return nil, ErrSetupDone
	}
	if err := s.allow(ip); err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(normalizeSetupToken(token)), []byte(normalizeSetupToken(s.setupToken))) != 1 {
		return nil, ErrSetupToken
	}
	var h string
	if err := s.runHash(ctx, func() { h = hashPassword(password, s.hash) }); err != nil {
		return nil, err
	}
	res, err := s.db.Write.ExecContext(ctx, `INSERT INTO secrets (name, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (name) DO NOTHING`, ownerPasswordKey, []byte(h), s.now().UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("store password: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrSetupDone // lost a race with a concurrent setup
	}
	return s.newSession(ctx)
}

// Login checks the owner password and starts a session.
func (s *Service) Login(ctx context.Context, ip netip.Addr, password string) (*Session, error) {
	if s.mode != config.AuthPassword {
		return nil, ErrWrongMode
	}
	if err := s.allow(ip); err != nil {
		return nil, err
	}
	h, err := s.passwordHash(ctx)
	if err != nil {
		return nil, err
	}
	if len(password) > MaxPasswordLen {
		return nil, ErrInvalidPassword
	}
	var ok bool
	var verr error
	if err := s.runHash(ctx, func() { ok, verr = verifyPassword(h, password) }); err != nil {
		return nil, err
	}
	if verr != nil {
		return nil, verr
	}
	if !ok {
		return nil, ErrInvalidPassword
	}
	return s.newSession(ctx)
}

// ResetPassword removes the owner password and every session, so the next
// visitor can run first-run setup again (CLI recovery path).
func (s *Service) ResetPassword(ctx context.Context) error {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	if _, err := tx.ExecContext(ctx, `DELETE FROM secrets WHERE name = ?`, ownerPasswordKey); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.announced.Store(false) // show the token again when setup is next needed
	return nil
}

// ---------- sessions ----------

// Session is a newly created browser session.
type Session struct {
	// Token is the cookie value (only ever sent to the browser).
	Token     string
	CSRF      string
	ExpiresAt time.Time
}

func randomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (s *Service) newSession(ctx context.Context) (*Session, error) {
	now := s.now()
	sess := &Session{Token: randomToken(32), CSRF: randomToken(32), ExpiresAt: now.Add(SessionIdleTTL)}
	// Opportunistic cleanup of expired sessions.
	if _, err := s.db.Write.ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at <= ?`, now.UnixMilli()); err != nil {
		return nil, err
	}
	_, err := s.db.Write.ExecContext(ctx, `INSERT INTO auth_sessions
		(token_hash, csrf_token, created_at, last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		sha(sess.Token), sess.CSRF, now.UnixMilli(), now.UnixMilli(), sess.ExpiresAt.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return sess, nil
}

// Logout ends the principal's session (no-op for other methods).
func (s *Service) Logout(ctx context.Context, p *Principal) error {
	if p == nil || p.sessionHash == "" {
		return nil
	}
	_, err := s.db.Write.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash = ?`, p.sessionHash)
	return err
}

func (s *Service) sessionPrincipal(ctx context.Context, token string) (*Principal, error) {
	if token == "" || len(token) > 100 {
		return nil, nil
	}
	h := sha(token)
	now := s.now().UnixMilli()
	var csrf string
	var created, seen, expires int64
	err := s.db.Read.QueryRowContext(ctx, `SELECT csrf_token, created_at, last_seen_at, expires_at
		FROM auth_sessions WHERE token_hash = ? AND expires_at > ?`, h, now).Scan(&csrf, &created, &seen, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if now-seen >= sessionTouchEvery.Milliseconds() {
		expires = min(now+SessionIdleTTL.Milliseconds(), created+SessionMaxAge.Milliseconds())
		if _, err := s.db.Write.ExecContext(ctx, `UPDATE auth_sessions SET last_seen_at = ?, expires_at = ?
			WHERE token_hash = ?`, now, expires, h); err != nil {
			return nil, err
		}
	}
	return &Principal{Method: MethodSession, User: "owner", CSRF: csrf, ExpiresAt: expires, sessionHash: h}, nil
}

// ---------- request authentication ----------

// Authenticate identifies the request. It returns (nil, nil) for an
// anonymous request and ErrInvalidToken for a bad bearer token. A bearer
// token takes precedence over every other method; tokens work in all modes.
func (s *Service) Authenticate(r *http.Request) (*Principal, error) {
	ctx := r.Context()
	if h := r.Header.Get("Authorization"); h != "" {
		scheme, cred, _ := strings.Cut(h, " ")
		if !strings.EqualFold(scheme, "Bearer") {
			return nil, ErrInvalidToken
		}
		return s.tokenPrincipal(ctx, strings.TrimSpace(cred))
	}
	switch s.mode {
	case config.AuthNone:
		return &Principal{Method: MethodNone}, nil
	case config.AuthProxy:
		if !s.proxies.FromProxy(r) {
			return nil, nil
		}
		user := strings.TrimSpace(r.Header.Get(s.header))
		if user == "" || len(user) > 256 || !utf8.ValidString(user) {
			return nil, nil
		}
		csrf := base64.RawURLEncoding.EncodeToString(s.keys.MAC("csrf/proxy", []byte(user)))
		return &Principal{Method: MethodProxy, User: user, CSRF: csrf}, nil
	default:
		c, err := r.Cookie(CookieName)
		if err != nil {
			return nil, nil
		}
		return s.sessionPrincipal(ctx, c.Value)
	}
}

// ---------- API tokens ----------

// Token is an API token's metadata (never its secret).
type Token struct {
	ID         string
	Name       string
	Prefix     string
	CreatedAt  int64
	LastUsedAt *int64
}

// CreateToken makes a new read-only API token and returns its secret,
// which is not stored and cannot be shown again.
func (s *Service) CreateToken(ctx context.Context, name string) (*Token, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return nil, "", ErrInvalidTokenName
	}
	secret := TokenPrefix + randomToken(32)
	t := &Token{ID: randomToken(9), Name: name, Prefix: secret[:len(TokenPrefix)+6], CreatedAt: s.now().UnixMilli()}
	_, err := s.db.Write.ExecContext(ctx, `INSERT INTO api_tokens (id, name, token_hash, prefix, created_at)
		VALUES (?, ?, ?, ?, ?)`, t.ID, t.Name, sha(secret), t.Prefix, t.CreatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("create token: %w", err)
	}
	return t, secret, nil
}

// ListTokens returns every token, newest first.
func (s *Service) ListTokens(ctx context.Context) ([]Token, error) {
	rows, err := s.db.Read.QueryContext(ctx, `SELECT id, name, prefix, created_at, last_used_at
		FROM api_tokens ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken deletes a token; it stops working immediately.
func (s *Service) RevokeToken(ctx context.Context, id string) error {
	res, err := s.db.Write.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) tokenPrincipal(ctx context.Context, secret string) (*Principal, error) {
	if !strings.HasPrefix(secret, TokenPrefix) || len(secret) > 100 {
		return nil, ErrInvalidToken
	}
	h := sha(secret)
	var id, name string
	var used sql.NullInt64
	err := s.db.Read.QueryRowContext(ctx, `SELECT id, name, last_used_at FROM api_tokens WHERE token_hash = ?`, h).
		Scan(&id, &name, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	now := s.now().UnixMilli()
	if !used.Valid || now-used.Int64 >= tokenTouchEvery.Milliseconds() {
		if _, err := s.db.Write.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, now, id); err != nil {
			return nil, err
		}
	}
	return &Principal{Method: MethodToken, User: name, TokenID: id}, nil
}
