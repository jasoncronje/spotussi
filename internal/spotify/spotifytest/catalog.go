package spotifytest

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Catalog endpoints (M4): GET /v1/tracks/{id}, /v1/artists/{id},
// /v1/albums/{id} and /v1/playlists/{id}. Tracks, artists and albums
// answer for any ID with deterministic objects unless set explicitly or
// marked missing; playlists answer only when set (Spotify-made ones,
// ID prefix 37i9dQZ, are 404 like for real apps created after Nov 2024).

// CatalogTrack describes a track the fake serves.
type CatalogTrack struct {
	ID         string
	Name       string
	AlbumID    string   // default "album-<id>"
	ArtistIDs  []string // default ArtistsOf(id)
	DurationMs int64    // default 200000
	// AnswerID, when set, is the ID in the response (Spotify answering
	// for a different track than requested).
	AnswerID string
}

// CatalogPlaylist describes a playlist the fake serves.
type CatalogPlaylist struct {
	ID, Name, OwnerID string
}

type catalog struct {
	tracks    map[string]CatalogTrack
	playlists map[string]CatalogPlaylist
	missing   map[string]bool // paths answering 404
	// ArtistsOf picks the artists of generated tracks (default: one,
	// "artist-<id>").
	artistsOf func(trackID string) []string
	latency   time.Duration
	log       []string // catalog paths requested, in order
}

func (f *Fake) cat() *catalog {
	if f.catalog == nil {
		f.catalog = &catalog{tracks: map[string]CatalogTrack{}, playlists: map[string]CatalogPlaylist{}, missing: map[string]bool{}}
	}
	return f.catalog
}

// SetTrack serves t at /v1/tracks/{t.ID}.
func (f *Fake) SetTrack(t CatalogTrack) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cat().tracks[t.ID] = t
}

// SetPlaylist serves p at /v1/playlists/{p.ID}.
func (f *Fake) SetPlaylist(p CatalogPlaylist) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cat().playlists[p.ID] = p
}

// SetMissing makes path (e.g. "/v1/tracks/abc") answer 404.
func (f *Fake) SetMissing(paths ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range paths {
		f.cat().missing[p] = true
	}
}

// SetArtistsOf sets how generated tracks pick their artists.
func (f *Fake) SetArtistsOf(fn func(trackID string) []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cat().artistsOf = fn
}

// SetLatency delays every catalog response.
func (f *Fake) SetLatency(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cat().latency = d
}

// CatalogLog returns the catalog paths requested so far, in order
// (injected failures excluded).
func (f *Fake) CatalogLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cat().log...)
}

// HitsPrefix counts authorized requests to paths starting with prefix.
func (f *Fake) HitsPrefix(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for p, c := range f.hits {
		if strings.HasPrefix(p, prefix) {
			n += c
		}
	}
	return n
}

func (f *Fake) registerCatalog(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/tracks/{id}", f.catalogHandler(func(c *catalog, id string) (any, bool) {
		t, ok := c.tracks[id]
		if !ok {
			t = CatalogTrack{ID: id}
		}
		return c.trackObject(t), true
	}))
	mux.HandleFunc("GET /v1/artists/{id}", f.catalogHandler(func(c *catalog, id string) (any, bool) {
		return artistObject(id), true
	}))
	mux.HandleFunc("GET /v1/albums/{id}", f.catalogHandler(func(c *catalog, id string) (any, bool) {
		return albumObject(id, c.artists("album-"+id)), true
	}))
	mux.HandleFunc("GET /v1/playlists/{id}", f.catalogHandler(func(c *catalog, id string) (any, bool) {
		p, ok := c.playlists[id]
		if !ok {
			return nil, false
		}
		return map[string]any{"id": p.ID, "name": p.Name, "owner": map[string]any{"id": p.OwnerID, "display_name": p.OwnerID},
			"images": []any{map[string]any{"url": "https://mosaic.scdn.co/640/" + p.ID, "width": 640, "height": 640}}}, true
	}))
}

func (f *Fake) catalogHandler(get func(c *catalog, id string) (any, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		c := f.cat()
		latency := c.latency
		f.mu.Unlock()
		time.Sleep(latency)
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.apiPrelude(w, r) {
			return
		}
		c.log = append(c.log, r.URL.Path)
		id := r.PathValue("id")
		obj, ok := get(c, id)
		if !ok || c.missing[r.URL.Path] {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"status":404,"message":"Resource not found"}}`))
			return
		}
		json.NewEncoder(w).Encode(obj)
	}
}

func (c *catalog) artists(trackID string) []string {
	if c.artistsOf != nil {
		return c.artistsOf(trackID)
	}
	return []string{"artist-" + trackID}
}

func simpleArtists(ids []string) []any {
	out := make([]any, 0, len(ids))
	for _, a := range ids {
		out = append(out, map[string]any{"id": a, "name": "Name of " + a, "uri": "spotify:artist:" + a, "type": "artist"})
	}
	return out
}

func images(base string) []any {
	return []any{
		map[string]any{"url": base + "/640", "width": 640, "height": 640},
		map[string]any{"url": base + "/300", "width": 300, "height": 300},
		map[string]any{"url": base + "/64", "width": 64, "height": 64},
	}
}

func albumObject(id string, artists []string) map[string]any {
	return map[string]any{"id": id, "name": "Album " + id, "uri": "spotify:album:" + id, "album_type": "album",
		"release_date": "2019-05", "release_date_precision": "month", "total_tracks": 12,
		"images": images("https://i.scdn.co/image/" + id), "artists": simpleArtists(artists)}
}

func (c *catalog) trackObject(t CatalogTrack) map[string]any {
	id := t.ID
	if t.AnswerID != "" {
		id = t.AnswerID
	}
	name := t.Name
	if name == "" {
		name = "Track " + t.ID
	}
	album := t.AlbumID
	if album == "" {
		album = "album-" + t.ID
	}
	artists := t.ArtistIDs
	if artists == nil {
		artists = c.artists(t.ID)
	}
	dur := t.DurationMs
	if dur == 0 {
		dur = 200000
	}
	return map[string]any{"id": id, "uri": "spotify:track:" + id, "name": name, "duration_ms": dur, "explicit": true,
		"is_local": false, "type": "track", "disc_number": 1, "track_number": 3,
		"external_ids": map[string]any{"isrc": "USX" + strings.ToUpper(t.ID)},
		"artists":      simpleArtists(artists), "album": albumObject(album, artists[:1])}
}

func artistObject(id string) map[string]any {
	return map[string]any{"id": id, "name": "Name of " + id, "uri": "spotify:artist:" + id, "type": "artist",
		"genres": []any{"genre " + id[len(id)-1:], "indie"}, "images": images("https://i.scdn.co/image/a-" + id)}
}
