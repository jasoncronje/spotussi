// Package mcp is the built-in MCP server (D16, D28): read-only tools over
// the same Backend that answers /api/v1, so a model sees the numbers the UI
// shows. It speaks Streamable HTTP at /mcp (stateless, JSON responses) and
// authenticates with API tokens (D13). RunProxy bridges stdio clients to it.
package mcp

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"spotussi/internal/api"
	"spotussi/internal/auth"
)

// Path is where the MCP endpoint is mounted.
const Path = "/mcp"

// Options configures the MCP server.
type Options struct {
	// Backend answers every tool (the same one /api/v1 uses).
	Backend api.Backend
	// Auth authenticates requests; nil leaves /mcp open (tests).
	Auth *auth.Service
	// Version and Mode ("live" or "mock") are reported by get_status and
	// in the server's implementation info.
	Version, Mode string
}

const instructions = `spotussi holds one person's Spotify listening history (plays, skips, listening time) and answers the same numbers its web dashboard shows.

- Start with get_status to learn the owner's timezone, how much history there is and whether data is still being imported or enriched.
- Every result carries names and IDs. Pass IDs from one tool to another: search_catalog finds an artist, album or track by name; get_top, get_entity, get_history and get_sessions accept those IDs.
- Time ranges: range is a preset (today, this_week, this_month, this_year, last_7d, last_4w, last_6m, last_12m, prev_day, prev_week, prev_month, prev_year, all) or custom with from and to (inclusive local dates, YYYY-MM-DD). Day boundaries use the owner's timezone.
- metric "plays" ranks by counted plays, "time" by listening time. A play counts when it passes the owner's play rule (see get_status); skips are plays that ended early.
- Minutes are listening minutes; times are local to the owner's timezone (RFC 3339).`

// NewServer builds the MCP server and registers every tool.
func NewServer(o Options) *sdk.Server {
	version := o.Version
	if version == "" {
		version = "dev"
	}
	s := sdk.NewServer(&sdk.Implementation{Name: "spotussi", Title: "spotussi listening stats", Version: version},
		&sdk.ServerOptions{Instructions: instructions})
	registerTools(s, o)
	return s
}

// Handler serves the Streamable HTTP transport behind token auth.
//
// The server is stateless (every POST is self-contained, no session IDs)
// and answers with plain JSON: the tools are read-only request/response
// calls with nothing to stream. The SDK's DNS-rebinding guard (which
// rejects loopback connections with a non-localhost Host) is off because
// every deployment sits behind a reverse proxy on some host name, and a
// rebinding page cannot supply the bearer token /mcp requires.
func Handler(o Options) http.Handler {
	srv := NewServer(o)
	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, &sdk.StreamableHTTPOptions{
		Stateless:                  true,
		JSONResponse:               true,
		DisableLocalhostProtection: true,
		MaxRequestBodyBytes:        1 << 20,
	})
	return requireToken(o.Auth, h)
}

// Routes returns the handlers to mount on the root mux: /mcp, plus JSON 404s
// for the OAuth discovery documents MCP clients probe after a 401 (the SPA
// fallback would otherwise answer them with index.html).
func Routes(o Options) map[string]http.Handler {
	notFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "spotussi has no OAuth server; send an API token as a bearer token")
	})
	return map[string]http.Handler{
		Path:                                       Handler(o),
		"/.well-known/oauth-protected-resource":    notFound,
		"/.well-known/oauth-protected-resource/":   notFound,
		"/.well-known/oauth-authorization-server":  notFound,
		"/.well-known/oauth-authorization-server/": notFound,
		"/.well-known/openid-configuration":        notFound,
	}
}

// requireToken admits API tokens, and everyone in AUTH_MODE=none. Browser
// identities (session cookies, proxy headers) are refused: an MCP client
// is a separate, individually revocable client (D13).
func requireToken(a *auth.Service, next http.Handler) http.Handler {
	if a == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := a.Authenticate(r)
		switch {
		case errors.Is(err, auth.ErrInvalidToken):
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			writeError(w, http.StatusUnauthorized, "invalid_token", "invalid or revoked API token")
			return
		case err != nil:
			slog.Error("mcp: authenticate", "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		case p == nil:
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "unauthorized",
				"send an API token in the Authorization header (Bearer); create one in Settings → API & MCP")
			return
		case p.Method != auth.MethodToken && p.Method != auth.MethodNone:
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "token_required",
				"/mcp needs an API token, not a browser sign-in; create one in Settings → API & MCP")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	body, _ := json.Marshal(api.ErrorBody{Error: api.ErrorDetail{Code: code, Message: msg}})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write(body)
}
