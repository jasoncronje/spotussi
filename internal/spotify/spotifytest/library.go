package spotifytest

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Library endpoints (D36): GET /v1/me/tracks, /v1/me/albums,
// /v1/me/playlists and /v1/playlists/{id}/items, paged like Spotify
// (limit ≤ 50, offset, total). Saved tracks and albums need the
// user-library-read scope (403 without it); playlist items answer 403
// unless the connected user (listener42) owns the playlist or it is
// collaborative, as for development-mode apps since Feb 2026.

// Me is the fake's connected account ID.
const Me = "listener42"

// Library is what the library endpoints serve, newest first for saved
// tracks and albums (as Spotify orders them).
type Library struct {
	Tracks    []LibraryItem
	Albums    []LibraryAlbum
	Playlists []LibraryPlaylist
}

// LibraryItem is a saved track or a playlist item.
type LibraryItem struct {
	Track   CatalogTrack
	AddedAt time.Time
	AddedBy string // playlist items
	Episode bool   // an episode instead of a track
	Local   bool   // a local file (no ID)
}

// LibraryAlbum is a saved album.
type LibraryAlbum struct {
	ID      string
	AddedAt time.Time
}

// LibraryPlaylist is one of the user's playlists.
type LibraryPlaylist struct {
	ID, Name, OwnerID string
	Collaborative     bool
	Snapshot          string
	Items             []LibraryItem
}

// SetLibrary changes the served library under the fake's lock.
func (f *Fake) SetLibrary(fn func(l *Library)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.library == nil {
		f.library = &Library{}
	}
	fn(f.library)
}

func (f *Fake) lib() *Library {
	if f.library == nil {
		f.library = &Library{}
	}
	return f.library
}

func (f *Fake) registerLibrary(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/me/tracks", f.libraryHandler(true, func(r *http.Request) ([]any, bool) {
		var out []any
		for _, it := range f.lib().Tracks {
			out = append(out, map[string]any{"added_at": stamp(it.AddedAt), "track": f.cat().trackObject(it.Track)})
		}
		return out, true
	}))
	mux.HandleFunc("GET /v1/me/albums", f.libraryHandler(true, func(r *http.Request) ([]any, bool) {
		var out []any
		for _, a := range f.lib().Albums {
			out = append(out, map[string]any{"added_at": stamp(a.AddedAt), "album": albumObject(a.ID, f.cat().artists("album-"+a.ID))})
		}
		return out, true
	}))
	mux.HandleFunc("GET /v1/me/playlists", f.libraryHandler(false, func(r *http.Request) ([]any, bool) {
		var out []any
		for _, p := range f.lib().Playlists {
			out = append(out, map[string]any{"id": p.ID, "name": p.Name, "collaborative": p.Collaborative, "public": false,
				"snapshot_id": p.Snapshot, "owner": map[string]any{"id": p.OwnerID, "display_name": "Name of " + p.OwnerID},
				"images": []any{map[string]any{"url": "https://mosaic.scdn.co/640/" + p.ID, "width": 640, "height": 640}},
				"items":  map[string]any{"total": len(p.Items)}})
		}
		return out, true
	}))
	mux.HandleFunc("GET /v1/playlists/{id}/items", f.libraryHandler(false, func(r *http.Request) ([]any, bool) {
		id := r.PathValue("id")
		i := slices.IndexFunc(f.lib().Playlists, func(p LibraryPlaylist) bool { return p.ID == id })
		if i < 0 {
			return nil, false
		}
		p := f.lib().Playlists[i]
		if p.OwnerID != Me && !p.Collaborative {
			return nil, true // forbidden
		}
		out := []any{}
		for _, it := range p.Items {
			var obj map[string]any
			switch {
			case it.Episode:
				obj = map[string]any{"id": it.Track.ID, "uri": "spotify:episode:" + it.Track.ID, "type": "episode", "name": it.Track.Name}
			case it.Local:
				obj = map[string]any{"id": nil, "uri": "spotify:local:Artist:Album:" + it.Track.Name + ":200", "type": "track",
					"name": it.Track.Name, "is_local": true, "duration_ms": 200000, "artists": []any{map[string]any{"name": "Local Artist"}},
					"album": map[string]any{"name": "Local Album"}}
			default:
				obj = f.cat().trackObject(it.Track)
			}
			out = append(out, map[string]any{"added_at": stamp(it.AddedAt), "added_by": map[string]any{"id": it.AddedBy},
				"is_local": it.Local, "item": obj, "track": obj})
		}
		return out, true
	}))
}

func stamp(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// libraryHandler pages all() like Spotify. needsScope requires
// user-library-read; all returning a nil slice with true means 403, and
// false 404.
func (f *Fake) libraryHandler(needsScope bool, all func(r *http.Request) ([]any, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.apiPrelude(w, r) {
			return
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if needsScope && !slices.Contains(strings.Fields(f.scopes[tok]), "user-library-read") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":{"status":403,"message":"Insufficient client scope"}}`))
			return
		}
		items, ok := all(r)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"status":404,"message":"Resource not found"}}`))
			return
		}
		if items == nil && strings.HasSuffix(r.URL.Path, "/items") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":{"status":403,"message":"Forbidden"}}`))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 {
			limit = 20
		}
		if limit > 50 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"status":400,"message":"Invalid limit"}}`))
			return
		}
		page := []any{}
		if offset < len(items) {
			page = items[offset:min(len(items), offset+limit)]
		}
		var next any
		if offset+limit < len(items) {
			next = "https://api.spotify.com/v1" + strings.TrimPrefix(r.URL.Path, "/v1") + "?offset=" + strconv.Itoa(offset+limit)
		}
		json.NewEncoder(w).Encode(map[string]any{"total": len(items), "limit": limit, "offset": offset, "next": next,
			"items": page})
	}
}
