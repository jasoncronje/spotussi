package mock

import (
	"encoding/json"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/derive"
)

// Scenario puts the mock into states the UI must handle (M7, M8) without a
// real instance: a catalog that is mostly still hydrating, a Spotify
// connection that needs a reconnect, failing or slow endpoints. It is
// development tooling: set with POST /_mock/scenario, never part of the
// API contract, and the defaults are what the differential test compares.
type Scenario struct {
	// Catalog: "" (the dataset as generated: a small unhydrated tail) or
	// "sparse" (like a fresh import: a quarter from Spotify, half from
	// the fallback sources, a quarter known by name only).
	Catalog string `json:"catalog"`
	// Spotify: "" (connected), "reconnect_required" or "not_configured".
	Spotify string `json:"spotify"`
	// Fail answers every /api/v1 request whose path starts with it with
	// FailStatus (default 500); "*" fails everything.
	Fail       string `json:"fail"`
	FailStatus int    `json:"fail_status"`
	// LatencyMs delays every /api/v1 response.
	LatencyMs int `json:"latency_ms"`
}

// applyCatalog sets where each entity's details come from. Callers hold
// the write lock (or own b exclusively).
func (b *Backend) applyCatalog() {
	ds := b.ds
	sparse := b.scenario.Catalog == "sparse"
	level := func(id string, hydrated bool) derive.Meta {
		if !hydrated {
			return derive.MetaPending
		}
		if !sparse {
			return derive.MetaSpotify
		}
		h := fnv.New32a()
		h.Write([]byte(id))
		switch v := h.Sum32() % 8; {
		case v < 2:
			return derive.MetaSpotify
		case v < 6:
			return derive.MetaFallback
		default:
			return derive.MetaPending
		}
	}
	b.trackMeta = make([]derive.Meta, len(ds.Tracks))
	b.artistMeta = make([]derive.Meta, len(ds.Artists))
	b.albumMeta = make([]derive.Meta, len(ds.Albums))
	albumKnown := make([]bool, len(ds.Albums))
	for i, t := range ds.Tracks {
		b.trackMeta[i] = level(t.ID, t.Hydrated)
		if t.Hydrated {
			albumKnown[t.Album] = true
		}
	}
	for i, a := range ds.Artists {
		b.artistMeta[i] = level(a.ID, a.Hydrated)
	}
	for i, a := range ds.Albums {
		b.albumMeta[i] = level(a.ID, albumKnown[i])
	}
}

// metadata is an entity's provenance; fallback entities name the sources
// that would have filled them.
func metadata(m derive.Meta, sources ...string) api.EntityMetadata {
	switch m {
	case derive.MetaSpotify:
		return api.EntityMetadata{State: api.MetaSpotify, Sources: []string{}}
	case derive.MetaFallback:
		return api.EntityMetadata{State: api.MetaFallback, Sources: sources}
	}
	return api.EntityMetadata{State: api.MetaPending, Sources: []string{}}
}

// ScenarioHandler serves GET (current) and POST (replace; query
// parameters or a JSON body) /_mock/scenario.
func (b *Backend) ScenarioHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var s Scenario
			if r.ContentLength > 0 {
				if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			} else {
				q := r.URL.Query()
				s.Catalog, s.Spotify, s.Fail = q.Get("catalog"), q.Get("spotify"), q.Get("fail")
				s.FailStatus, _ = strconv.Atoi(q.Get("fail_status"))
				s.LatencyMs, _ = strconv.Atoi(q.Get("latency_ms"))
			}
			b.mu.Lock()
			catalogChanged := s.Catalog != b.scenario.Catalog
			b.scenario = s
			if catalogChanged {
				b.applyCatalog()
				b.applySettings()
			}
			b.mu.Unlock()
		}
		b.mu.RLock()
		s := b.scenario
		b.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s) //nolint:errcheck
	})
}

// Wrap applies the scenario's latency and failures to /api/v1 requests,
// and answers /spotify/connection for a scenario's Spotify state (the
// mock has no Spotify account of its own).
func (b *Backend) Wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ok := strings.CutPrefix(r.URL.Path, "/api/v1")
		if !ok {
			h.ServeHTTP(w, r)
			return
		}
		b.mu.RLock()
		s := b.scenario
		b.mu.RUnlock()
		if s.LatencyMs > 0 {
			time.Sleep(time.Duration(s.LatencyMs) * time.Millisecond)
		}
		if s.Fail != "" && !strings.HasPrefix(path, "/auth/") && (s.Fail == "*" || strings.HasPrefix(path, s.Fail)) {
			status := s.FailStatus
			if status == 0 {
				status = http.StatusInternalServerError
			}
			code := map[int]string{401: "unauthorized", 404: "not_found", 503: "unavailable"}[status]
			if code == "" {
				code = "internal"
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{ //nolint:errcheck
				"code": code, "message": "Simulated failure (mock scenario)."}})
			return
		}
		if s.Spotify != "" && path == "/spotify/connection" && r.Method == http.MethodGet {
			c := api.SpotifyConnection{State: s.Spotify, Scopes: []string{}}
			c.Callback.Mode, c.Callback.RedirectURI = "direct", "https://"+r.Host+"/oauth/spotify/callback"
			if s.Spotify == "reconnect_required" {
				c.ClientID, c.AccountID, c.AccountName = ptr("mockclientid"), ptr("listener"), ptr("listener")
				c.ConnectedAt = ptr(b.ds.PollerSince)
				c.LastError = ptr("Spotify revoked access (invalid_grant).")
				c.Scopes = []string{"user-read-playback-state", "user-read-recently-played", "playlist-read-private"}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(c) //nolint:errcheck
			return
		}
		h.ServeHTTP(w, r)
	})
}
