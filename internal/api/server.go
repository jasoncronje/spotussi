// Package api is the HTTP layer: /api/v1 handlers, the embedded SPA, ETags
// and middleware. Handlers parse and validate parameters and delegate to a
// Backend; they contain no query logic.
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"spotussi/internal/auth"
	"spotussi/internal/spotify"
	"spotussi/internal/trust"
)

// Pinger reports storage health (the database in live mode).
type Pinger interface {
	Ping(ctx context.Context) error
}

// Options configures the HTTP handler.
type Options struct {
	Version string
	// Mode is "live" or "mock" (reported by /health).
	Mode string
	// Backend answers data endpoints; nil means they return 501 until the
	// query layer exists (M6).
	Backend Backend
	// DB is pinged by /health when set. If it can take snapshots (*db.DB),
	// it also serves GET /system/backup.
	DB Pinger
	// UI is the built SPA (may be nil or empty in dev builds).
	UI fs.FS
	// Extra mounts additional handlers by mux pattern (e.g. mock images).
	Extra map[string]http.Handler

	// Auth enforces instance access control on /api/v1 (D13) and serves
	// the /auth endpoints. Nil leaves the API open (handler tests).
	Auth *auth.Service
	// Spotify serves the connection endpoints and the OAuth callback (D12).
	Spotify *spotify.Client
	// BaseURL overrides the externally visible URL (BASE_URL).
	BaseURL string
	// Proxies are the trusted reverse proxies (forwarded headers).
	Proxies trust.Proxies
	// LoopbackPort picks the port of the plain-HTTP redirect URI (D21).
	LoopbackPort spotify.LoopbackPort
	// ImportDir receives export uploads (under DATA_DIR); empty disables
	// POST /imports (501).
	ImportDir string
	// ImportMaxBytes caps an upload (IMPORT_MAX_BYTES; 0 means 2 GiB).
	ImportMaxBytes int64
}

// NewHandler builds the root handler.
func NewHandler(o Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		h := Health{Status: "ok", Version: o.Version, Mode: o.Mode}
		if o.DB != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := o.DB.Ping(ctx); err != nil {
				slog.Warn("health: database ping failed", "err", err)
				h.Status = "degraded"
				writeJSON(w, r, http.StatusServiceUnavailable, h)
				return
			}
		}
		writeJSON(w, r, http.StatusOK, h)
	})
	mux.HandleFunc("GET /api/v1/system/status", systemStatus(o))
	if src, ok := o.DB.(backupSource); ok {
		registerBackup(mux, src)
	}
	registerDocs(mux)
	if o.Auth != nil {
		registerAccess(mux, o.Auth)
	}
	if o.Spotify != nil {
		registerSpotify(mux, o.Spotify, o.BaseURL, o.Proxies, o.LoopbackPort)
	}
	if o.Backend != nil {
		registerRoutes(mux, o.Backend)
		maxBytes := o.ImportMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2 << 30
		}
		registerImports(mux, o.Backend, o.ImportDir, maxBytes)
		mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, http.StatusNotFound, "not_found", "no such endpoint")
		})
	} else {
		mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, http.StatusNotImplemented, "not_implemented",
				"this endpoint is not implemented yet; run `spotussi serve --mock` for the synthetic dataset")
		})
	}
	for pattern, h := range o.Extra {
		mux.Handle(pattern, h)
	}
	mux.Handle("/", newSPAHandler(o.UI))
	var h http.Handler = mux
	if o.Auth != nil {
		h = requireAuth(o.Auth, h)
	}
	return recoverer(securityHeaders(crossOrigin(warnUntrustedProxy(o.Proxies, o.BaseURL, h))))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic", "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
				writeError(w, r, http.StatusInternalServerError, "internal", "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
