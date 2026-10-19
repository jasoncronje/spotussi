package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"spotussi/internal/auth"
	"spotussi/internal/config"
)

// Access control (D13). Every /api/v1 route needs an identity except the
// few a signed-out browser must reach. API tokens are read-only: GET/HEAD
// only, and never the credential-management routes. Unsafe browser
// requests must carry the session's CSRF token in X-CSRF-Token (password
// and proxy modes), on top of the origin check every unsafe request gets.

// CSRFHeader carries the CSRF token on unsafe browser requests.
const CSRFHeader = "X-CSRF-Token"

const v1 = "/api/v1"

// publicRoutes are reachable without an identity.
var publicRoutes = map[string]bool{
	"GET " + v1 + "/health":       true,
	"HEAD " + v1 + "/health":      true,
	"GET " + v1 + "/auth/session": true,
	"POST " + v1 + "/auth/setup":  true,
	"POST " + v1 + "/auth/login":  true,
}

// httpError is an error with a fixed status and code.
type httpError struct {
	status     int
	code, msg  string
	retryAfter time.Duration
}

func (e *httpError) Error() string { return e.msg }

func errorf(status int, code, format string, args ...any) *httpError {
	return &httpError{status: status, code: code, msg: fmt.Sprintf(format, args...)}
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// requireAuth authenticates /api/ requests and enforces the rules above.
func requireAuth(a *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		public := publicRoutes[r.Method+" "+r.URL.Path]
		p, err := a.Authenticate(r)
		switch {
		case err == nil:
		case public: // a stale token must not lock anyone out of login
			p = nil
		case errors.Is(err, auth.ErrInvalidToken):
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			writeError(w, r, http.StatusUnauthorized, "invalid_token", "invalid or revoked API token")
			return
		default:
			slog.Error("authenticate", "err", err)
			writeError(w, r, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		if p != nil {
			r = r.WithContext(auth.WithPrincipal(r.Context(), p))
		}
		if public {
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case p == nil:
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "sign in or send an API token")
		case p.Method == auth.MethodToken && !safeMethod(r.Method):
			writeError(w, r, http.StatusForbidden, "read_only_token", "API tokens are read-only")
		case p.Method == auth.MethodToken && strings.HasPrefix(r.URL.Path, v1+"/auth/"):
			writeError(w, r, http.StatusForbidden, "forbidden", "API tokens cannot manage credentials")
		case p.Method == auth.MethodToken && r.URL.Path == v1+"/system/backup":
			// The snapshot holds every credential hash and the encrypted
			// Spotify token (D28).
			writeError(w, r, http.StatusForbidden, "forbidden", "API tokens cannot download backups; sign in")
		case !safeMethod(r.Method) && !p.CheckCSRF(r.Header.Get(CSRFHeader)):
			writeError(w, r, http.StatusForbidden, "csrf_failed", "missing or invalid "+CSRFHeader+" header")
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// crossOrigin rejects unsafe cross-origin browser requests (Sec-Fetch-Site
// / Origin checks) with a JSON error.
func crossOrigin(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusForbidden, "cross_origin", "cross-origin request rejected")
	}))
	return cop.Handler(next)
}

// ---------- wire types ----------

type AuthSession struct {
	Mode          string  `json:"mode"`
	Authenticated bool    `json:"authenticated"`
	SetupRequired bool    `json:"setup_required"`
	Method        *string `json:"method"`
	User          *string `json:"user"`
	CSRFToken     *string `json:"csrf_token"`
	ExpiresAt     *int64  `json:"expires_at"`
}

type PasswordInput struct {
	Password string `json:"password"`
}

// SetupInput is the first-run setup request: the new owner password and
// the one-time setup token from the server log (D21).
type SetupInput struct {
	Password   string `json:"password"`
	SetupToken string `json:"setup_token"`
}

type APIToken struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt *int64 `json:"last_used_at"`
}

type APITokenList struct {
	Items []APIToken `json:"items"`
}

type APITokenInput struct {
	Name string `json:"name"`
}

type APITokenCreated struct {
	Token  APIToken `json:"token"`
	Secret string   `json:"secret"`
}

// ---------- handlers ----------

func authError(err error) error {
	var rl *auth.RateLimitError
	switch {
	case errors.As(err, &rl):
		e := errorf(http.StatusTooManyRequests, "rate_limited", "%s", rl.Error())
		e.retryAfter = rl.RetryAfter
		return e
	case errors.Is(err, auth.ErrSetupDone):
		return errorf(http.StatusConflict, "setup_done", "%s", err)
	case errors.Is(err, auth.ErrSetupRequired):
		return errorf(http.StatusConflict, "setup_required", "%s", err)
	case errors.Is(err, auth.ErrWrongMode):
		return errorf(http.StatusConflict, "wrong_auth_mode", "%s", err)
	case errors.Is(err, auth.ErrInvalidPassword):
		return errorf(http.StatusUnauthorized, "invalid_password", "%s", err)
	case errors.Is(err, auth.ErrSetupToken):
		return errorf(http.StatusUnauthorized, "invalid_setup_token", "%s", err)
	case errors.Is(err, auth.ErrPasswordPolicy), errors.Is(err, auth.ErrInvalidTokenName):
		return errorf(http.StatusBadRequest, "bad_request", "%s", err)
	case errors.Is(err, auth.ErrNotFound):
		return errorf(http.StatusNotFound, "not_found", "no such token")
	}
	return err
}

func registerAccess(mux *http.ServeMux, a *auth.Service) {
	session := func(r *http.Request, p *auth.Principal) (*AuthSession, error) {
		setup, err := a.SetupRequired(r.Context())
		if err != nil {
			return nil, err
		}
		s := &AuthSession{Mode: string(a.Mode()), SetupRequired: setup}
		if p != nil {
			m, user := string(p.Method), p.User
			s.Authenticated, s.Method = true, &m
			if user != "" {
				s.User = &user
			}
			if p.CSRF != "" {
				csrf := p.CSRF
				s.CSRFToken = &csrf
			}
			if p.ExpiresAt != 0 {
				exp := p.ExpiresAt
				s.ExpiresAt = &exp
			}
		}
		return s, nil
	}
	start := func(w http.ResponseWriter, r *http.Request, in any, fn func(context.Context, netip.Addr) (*auth.Session, error)) {
		if err := decodeBody(r, in); err != nil {
			writeResult[AuthSession](w, r, nil, err)
			return
		}
		sess, err := fn(r.Context(), a.Proxies().ClientIP(r))
		if err != nil {
			writeResult[AuthSession](w, r, nil, authError(err))
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: auth.CookieName, Value: sess.Token, Path: "/",
			// The browser keeps the cookie for the absolute maximum; the
			// server enforces the sliding idle expiry.
			MaxAge:   int(auth.SessionMaxAge.Seconds()),
			HttpOnly: true, SameSite: http.SameSiteLaxMode,
			Secure: a.Proxies().Scheme(r) == "https",
		})
		m, user, exp := string(auth.MethodSession), "owner", sess.ExpiresAt.UnixMilli()
		writeJSON(w, r, http.StatusOK, &AuthSession{Mode: string(a.Mode()), Authenticated: true,
			Method: &m, User: &user, CSRFToken: &sess.CSRF, ExpiresAt: &exp})
	}

	mux.HandleFunc("GET "+v1+"/auth/session", func(w http.ResponseWriter, r *http.Request) {
		s, err := session(r, auth.FromContext(r.Context()))
		writeResult(w, r, s, err)
	})
	mux.HandleFunc("POST "+v1+"/auth/setup", func(w http.ResponseWriter, r *http.Request) {
		var in SetupInput
		start(w, r, &in, func(ctx context.Context, ip netip.Addr) (*auth.Session, error) {
			return a.Setup(ctx, ip, in.Password, in.SetupToken)
		})
	})
	mux.HandleFunc("POST "+v1+"/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var in PasswordInput
		start(w, r, &in, func(ctx context.Context, ip netip.Addr) (*auth.Session, error) {
			return a.Login(ctx, ip, in.Password)
		})
	})
	mux.HandleFunc("POST "+v1+"/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Logout(r.Context(), auth.FromContext(r.Context())); err != nil {
			writeResult[AuthSession](w, r, nil, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.Proxies().Scheme(r) == "https"})
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET "+v1+"/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		list, err := a.ListTokens(r.Context())
		if err != nil {
			writeResult[APITokenList](w, r, nil, err)
			return
		}
		out := &APITokenList{Items: make([]APIToken, len(list))}
		for i, t := range list {
			out.Items[i] = apiToken(t)
		}
		writeJSON(w, r, http.StatusOK, out)
	})
	mux.HandleFunc("POST "+v1+"/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		var in APITokenInput
		if err := decodeBody(r, &in); err != nil {
			writeResult[APITokenCreated](w, r, nil, err)
			return
		}
		t, secret, err := a.CreateToken(r.Context(), in.Name)
		if err != nil {
			writeResult[APITokenCreated](w, r, nil, authError(err))
			return
		}
		writeJSON(w, r, http.StatusCreated, &APITokenCreated{Token: apiToken(*t), Secret: secret})
	})
	mux.HandleFunc("DELETE "+v1+"/auth/tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.RevokeToken(r.Context(), r.PathValue("id")); err != nil {
			writeResult[APIToken](w, r, nil, authError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func apiToken(t auth.Token) APIToken {
	return APIToken{ID: t.ID, Name: t.Name, Prefix: t.Prefix, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt}
}

// authModeName is the auth mode reported in system status.
func authModeName(a *auth.Service) string {
	if a == nil {
		return string(config.AuthNone)
	}
	return string(a.Mode())
}

func writeRetryAfter(w http.ResponseWriter, d time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(max(1, d.Round(time.Second).Seconds()))))
}
