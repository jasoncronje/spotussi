// Package spotifytest is a fake Spotify accounts service and Web API for
// tests: /authorize, /api/token (authorization_code with PKCE
// verification, refresh_token with optional rotation), /v1/me, a
// scriptable /v1/me/player (204 when nothing plays) and
// /v1/me/player/recently-played, the catalog lookups (tracks, artists,
// albums, playlists), plus injected failures (e.g. 429 with
// Retry-After) per path.
package spotifytest

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Fake is a running fake. Configure fields before use or under Lock.
type Fake struct {
	Server *httptest.Server

	mu sync.Mutex
	// ClientID is the only client accepted.
	ClientID string
	// RotateRefresh issues a new refresh token on every refresh.
	RotateRefresh bool
	// RefreshDelay slows refresh responses (single-flight tests).
	RefreshDelay time.Duration
	// MeStatus, when non-zero, makes /v1/me fail with this status.
	MeStatus int
	// Deny makes /authorize redirect with error=access_denied.
	Deny bool

	codes   map[string]grant  // authorization code -> grant
	refresh map[string]bool   // valid refresh tokens
	access  map[string]bool   // valid access tokens
	scopes  map[string]string // access token -> scopes
	granted map[string]string // refresh token -> scopes
	used    map[string]bool   // redeemed codes
	Seen    map[string]url.Values

	Refreshes atomic.Int32

	// player is the scripted GET /v1/me/player answer (nil: 204).
	player *PlayerSpec
	// recent are the plays recently-played knows about.
	recent []RecentPlay
	// fail queues injected failures per path, consumed one per request.
	fail map[string][]Failure
	// quota maps path prefixes to the Retry-After of a spent daily quota:
	// every request under them answers 429 QUOTA_EXCEEDED.
	quota map[string]string
	hits  map[string]int
	// catalog serves the catalog endpoints (catalog.go).
	catalog *catalog
	// library serves the library endpoints (library.go).
	library *Library
}

// Failure is an injected Web API error response.
type Failure struct {
	Status     int
	RetryAfter string // Retry-After header, if any
	Body       string // default: a Web API error object
}

type grant struct {
	clientID, redirectURI, challenge, scope string
}

// New starts a fake accepting clientID.
func New(clientID string) *Fake {
	f := &Fake{ClientID: clientID, codes: map[string]grant{}, refresh: map[string]bool{},
		access: map[string]bool{}, scopes: map[string]string{}, granted: map[string]string{}, used: map[string]bool{}, Seen: map[string]url.Values{},
		fail: map[string][]Failure{}, hits: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /authorize", f.authorize)
	mux.HandleFunc("POST /api/token", f.token)
	mux.HandleFunc("GET /v1/me", f.me)
	mux.HandleFunc("GET /v1/me/player", f.playerHandler)
	mux.HandleFunc("GET /v1/me/player/recently-played", f.recentHandler)
	f.registerCatalog(mux)
	f.registerLibrary(mux)
	f.Server = httptest.NewServer(mux)
	return f
}

// Close stops the server.
func (f *Fake) Close() { f.Server.Close() }

// AccountsURL is the fake accounts base URL.
func (f *Fake) AccountsURL() string { return f.Server.URL }

// APIURL is the fake Web API base URL.
func (f *Fake) APIURL() string { return f.Server.URL + "/v1" }

func random() string {
	b := make([]byte, 18)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Approve simulates the owner approving the consent screen for an
// authorize URL and returns the URL Spotify would redirect the browser to.
func (f *Fake) Approve(authorizeURL string) (string, error) {
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(authorizeURL)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		return "", &url.Error{Op: "authorize", URL: authorizeURL, Err: http.ErrNotSupported}
	}
	return resp.Header.Get("Location"), nil
}

func (f *Fake) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Seen["authorize"] = q
	if q.Get("client_id") != f.ClientID || q.Get("response_type") != "code" ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("redirect_uri") == "" {
		http.Error(w, "INVALID_CLIENT", http.StatusBadRequest)
		return
	}
	ru, _ := url.Parse(q.Get("redirect_uri"))
	out := url.Values{"state": {q.Get("state")}}
	if f.Deny {
		out.Set("error", "access_denied")
	} else {
		code := random()
		f.codes[code] = grant{clientID: q.Get("client_id"), redirectURI: q.Get("redirect_uri"),
			challenge: q.Get("code_challenge"), scope: q.Get("scope")}
		out.Set("code", code)
	}
	ru.RawQuery = out.Encode()
	http.Redirect(w, r, ru.String(), http.StatusFound)
}

func oauthErr(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": code})
}

func (f *Fake) token(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.ParseForm() != nil {
		oauthErr(w, 400, "invalid_request")
		return
	}
	form := r.PostForm
	if form.Get("grant_type") == "refresh_token" {
		f.Refreshes.Add(1)
		f.mu.Lock()
		delay := f.RefreshDelay
		f.mu.Unlock()
		time.Sleep(delay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Seen["token:"+form.Get("grant_type")] = form
	if form.Get("client_id") != f.ClientID {
		oauthErr(w, 400, "invalid_client")
		return
	}
	var scope, refresh string
	switch form.Get("grant_type") {
	case "authorization_code":
		g, ok := f.codes[form.Get("code")]
		if !ok || f.used[form.Get("code")] {
			oauthErr(w, 400, "invalid_grant")
			return
		}
		f.used[form.Get("code")] = true
		sum := sha256.Sum256([]byte(form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			oauthErr(w, 400, "invalid_grant") // PKCE mismatch
			return
		}
		if form.Get("redirect_uri") != g.redirectURI {
			oauthErr(w, 400, "invalid_grant")
			return
		}
		refresh, scope = random(), g.scope
		f.refresh[refresh], f.granted[refresh] = true, scope
	case "refresh_token":
		old := form.Get("refresh_token")
		if !f.refresh[old] {
			oauthErr(w, 400, "invalid_grant")
			return
		}
		scope = f.granted[old]
		if f.RotateRefresh {
			delete(f.refresh, old)
			refresh = random()
			f.refresh[refresh], f.granted[refresh] = true, scope
		}
	default:
		oauthErr(w, 400, "unsupported_grant_type")
		return
	}
	access := random()
	f.access[access] = true
	f.scopes[access] = scope
	resp := map[string]any{"access_token": access, "token_type": "Bearer", "scope": scope, "expires_in": 3600}
	if refresh != "" {
		resp["refresh_token"] = refresh
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (f *Fake) me(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	w.Header().Set("Content-Type", "application/json")
	if !f.access[tok] {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"status":401,"message":"The access token expired"}}`))
		return
	}
	if f.MeStatus != 0 {
		w.WriteHeader(f.MeStatus)
		w.Write([]byte(`{"error":{"status":403,"message":"User not registered in the Developer Dashboard"}}`))
		return
	}
	w.Write([]byte(`{"id":"listener42","display_name":"Test Listener"}`))
}

// ExpireAccessTokens invalidates every issued access token.
func (f *Fake) ExpireAccessTokens() {
	f.mu.Lock()
	f.access = map[string]bool{}
	f.mu.Unlock()
}

// RevokeRefreshTokens invalidates every refresh token (the owner removed
// the app, or the token expired).
func (f *Fake) RevokeRefreshTokens() {
	f.mu.Lock()
	f.refresh = map[string]bool{}
	f.mu.Unlock()
}

// ValidRefreshToken reports whether tok is currently valid.
func (f *Fake) ValidRefreshToken(tok string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refresh[tok]
}

// Set runs fn under the fake's lock, to change configuration safely.
func (f *Fake) Set(fn func(f *Fake)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}
