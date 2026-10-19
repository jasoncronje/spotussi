// Package spotify is the only package that talks to Spotify (plan.md):
// the PKCE authorization flow, token refresh and rotation, and typed Web
// API calls. Keeping every request here isolates the rest of the app from
// Spotify's API policy churn.
//
// The connection is single-account (D1) and uses Authorization Code with
// PKCE and the owner's own Client ID, with no client secret (D12).
package spotify

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Spotify endpoints (overridable for tests).
const (
	DefaultAccountsURL = "https://accounts.spotify.com"
	DefaultAPIURL      = "https://api.spotify.com/v1"
	// DashboardURL is where the owner creates their Spotify app.
	DashboardURL = "https://developer.spotify.com/dashboard"
)

// CallbackPath is where Spotify redirects after authorization.
const CallbackPath = "/oauth/spotify/callback"

// LoopbackHost is the only plain-HTTP redirect host Spotify accepts.
const LoopbackHost = "127.0.0.1"

// LoopbackRedirectURI is the redirect URI for plain-HTTP instances (D12,
// D21): http://127.0.0.1:<port><prefix>/oauth/spotify/callback. Spotify
// only accepts HTTPS or loopback-IP redirect URIs, and its dashboard
// rejects loopback URIs without an explicit port (verified 2026-10-17),
// so the port is always included. The one registered URI serves both
// cases: opened at http://127.0.0.1:<port>, Spotify redirects straight
// back; opened on any other plain-HTTP host, the browser lands on a dead
// 127.0.0.1 page and the owner pastes its URL back into the wizard.
func LoopbackRedirectURI(port int, prefix string) string {
	return "http://" + net.JoinHostPort(LoopbackHost, strconv.Itoa(port)) + strings.TrimRight(prefix, "/") + CallbackPath
}

// DefaultLoopbackPort is used when neither the request nor the
// configuration names a port.
const DefaultLoopbackPort = 8080

// LoopbackPort decides the port in the plain-HTTP redirect URI.
type LoopbackPort struct {
	// Default is the port the server listens on (PORT), used when the
	// request's Host has no port. 0 means DefaultLoopbackPort.
	Default int
	// Override, when set, is always used (e.g. Docker publishes the
	// server on another port and the Host header can't be trusted).
	Override int
}

func (lp LoopbackPort) port(hostPort string) int {
	if lp.Override > 0 {
		return lp.Override
	}
	if p, err := strconv.Atoi(hostPort); err == nil && p > 0 && p <= 65535 {
		return p
	}
	if lp.Default > 0 {
		return lp.Default
	}
	return DefaultLoopbackPort
}

// Scopes are the read-only scopes requested (D12, D17).
var Scopes = []string{"user-read-playback-state", "user-read-recently-played", "playlist-read-private"}

// CallbackMode is how the authorization code gets back to the server.
type CallbackMode string

const (
	// CallbackDirect: Spotify redirects the browser to this instance
	// (HTTPS, or plain HTTP on a loopback IP).
	CallbackDirect CallbackMode = "direct"
	// CallbackPasteBack: the owner pastes the loopback URL into the wizard.
	CallbackPasteBack CallbackMode = "paste_back"
)

// Redirect returns the callback mode and the exact redirect URI for an
// instance reachable at origin (BASE_URL, or scheme://host derived from
// the request; may include a path prefix).
//
//   - HTTPS (including https://*.localhost dev hosts): direct callback to
//     <origin>/oauth/spotify/callback.
//   - Plain HTTP: always LoopbackRedirectURI with the port the owner
//     reaches the instance on (from origin, else lp). It is a direct
//     callback when origin is that same http://127.0.0.1:<port>, and
//     paste-back for every other host ("localhost", LAN IPs, [::1]),
//     where the browser lands on the dead 127.0.0.1 page.
func Redirect(origin string, lp LoopbackPort) (CallbackMode, string) {
	origin = strings.TrimRight(origin, "/")
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return CallbackPasteBack, LoopbackRedirectURI(lp.port(""), "")
	}
	if u.Scheme == "https" {
		return CallbackDirect, origin + CallbackPath
	}
	port := lp.port(u.Port())
	uri := LoopbackRedirectURI(port, u.Path)
	if u.Scheme == "http" && u.Hostname() == LoopbackHost && u.Port() == strconv.Itoa(port) {
		return CallbackDirect, uri
	}
	return CallbackPasteBack, uri
}

var clientIDPattern = regexp.MustCompile(`^[0-9A-Za-z]{32}$`)

// ValidClientID reports whether id looks like a Spotify Client ID
// (32 alphanumeric characters, as shown in the app dashboard).
func ValidClientID(id string) bool { return clientIDPattern.MatchString(id) }

// pkce returns a random code verifier (RFC 7636: 43-128 unreserved
// characters) and its S256 challenge.
func pkce() (verifier, challenge string) {
	b := make([]byte, 48)
	rand.Read(b)
	verifier = base64.RawURLEncoding.EncodeToString(b) // 64 chars
	return verifier, ChallengeFor(verifier)
}

// ChallengeFor is the S256 PKCE challenge for verifier.
func ChallengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ErrInvalidCallbackURL means a pasted URL is not a Spotify callback.
var ErrInvalidCallbackURL = errors.New("not a Spotify callback URL: paste the full address from the browser's address bar after approving access")

// CallbackParams are the query parameters Spotify appends to the redirect.
type CallbackParams struct {
	State string
	Code  string
	// Error is Spotify's error code (e.g. access_denied) when the owner
	// did not approve.
	Error string
}

// ParseCallbackURL extracts the callback parameters from a URL the owner
// pasted (paste-back mode). Only the path and query are inspected, so it
// accepts the loopback URL whatever host or port the browser showed.
func ParseCallbackURL(raw string) (CallbackParams, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 4096 {
		return CallbackParams{}, ErrInvalidCallbackURL
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw // address bars sometimes hide the scheme
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasSuffix(strings.TrimRight(u.Path, "/"), CallbackPath) {
		return CallbackParams{}, ErrInvalidCallbackURL
	}
	q := u.Query()
	p := CallbackParams{State: q.Get("state"), Code: q.Get("code"), Error: q.Get("error")}
	if p.State == "" || (p.Code == "" && p.Error == "") {
		return CallbackParams{}, ErrInvalidCallbackURL
	}
	return p, nil
}

// SafeReturnPath reports whether p is a same-origin absolute path (not
// "//host" or "/\host", which browsers treat as another origin).
func SafeReturnPath(p string) bool {
	return len(p) <= 512 && strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") &&
		!strings.HasPrefix(p, "/\\") && !strings.ContainsAny(p, "\r\n")
}

// authorizeURL builds the Spotify consent URL.
func authorizeURL(accountsURL, clientID, redirectURI, state, challenge string) string {
	q := url.Values{
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
		"state":                 {state},
		"scope":                 {strings.Join(Scopes, " ")},
	}
	return fmt.Sprintf("%s/authorize?%s", strings.TrimRight(accountsURL, "/"), q.Encode())
}
