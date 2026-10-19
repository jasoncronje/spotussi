// Package metadatatest is a fake of the fallback metadata sources (Spotify
// oEmbed, Deezer, ListenBrainz, MusicBrainz) for tests: one httptest
// server answering all four APIs' paths with responses recorded from the real services in
// Oct 2026 (public catalog data for one well-known track, "Never Gonna
// Give You Up" by Rick Astley; previews and signed URLs stripped).
// Everything else is "not found" in each API's own way.
package metadatatest

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"spotussi/internal/metadata"
)

//go:embed testdata/*.json
var fixtures embed.FS

// Recorded IDs.
const (
	TrackID  = "4cOdK2wGLETKBW3PvgPWqT" // Spotify track (oEmbed)
	ArtistID = "0gxyHStUsqpMadRV0Di1Qt" // Spotify artist (oEmbed)
	ISRC     = "GBARL9300135"
	MBID     = "db92a151-1ac2-438b-bc43-b82e149ddd50"
	// RecordingMBID is the recording ListenBrainz finds for the track.
	RecordingMBID = "8f3471b5-7e6a-48da-86a9-c1c07a0f47ae"
	// Names as an export spells them.
	Track  = "Never Gonna Give You Up"
	Artist = "Rick Astley"
	Album  = "Whenever You Need Somebody"
	// DurationMs is the track's length (Deezer: 213 s).
	DurationMs = 213573
)

// Server is the fake.
type Server struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
	// Fail, when set, may answer a request with a status instead (429,
	// 503, ...); 0 serves it normally.
	Fail func(r *http.Request) int
	// DeezerQuota answers this many Deezer requests with Deezer's in-body
	// quota error first.
	DeezerQuota int
}

// New starts a fake server, closed when the test ends.
func New(t testing.TB) *Server {
	s := &Server{hits: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Hits reports how many requests a source received ("oembed", "deezer",
// "listenbrainz", "musicbrainz").
func (s *Server) Hits(source string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[source]
}

// Clients returns clients of all three sources pointed at the fake, with
// limiters fast enough not to slow tests down.
func (s *Server) Clients() (*metadata.OEmbed, *metadata.Deezer, *metadata.MusicBrainz) {
	ua := metadata.UserAgent("test", "")
	return metadata.NewOEmbed(s.URL, ua, s.Client(), metadata.NewLimiter(1000, 100)),
		metadata.NewDeezer(s.URL, ua, s.Client(), metadata.NewLimiter(1000, 100)),
		metadata.NewMusicBrainz(s.URL, ua, s.Client(), metadata.NewLimiter(1000, 100))
}

// ListenBrainz returns a ListenBrainz client pointed at the fake (both
// hosts), with a fast limiter.
func (s *Server) ListenBrainz() *metadata.ListenBrainz {
	return metadata.NewListenBrainz(s.URL, s.URL, metadata.UserAgent("test", ""), s.Client(), metadata.NewLimiter(1000, 100))
}

func fixture(name string) []byte {
	b, err := fixtures.ReadFile("testdata/" + name)
	if err != nil {
		panic(err)
	}
	return b
}

func sourceOf(path string) string {
	switch {
	case strings.HasPrefix(path, "/oembed"):
		return metadata.SourceOEmbed
	case strings.HasPrefix(path, "/ws/2/"):
		return metadata.SourceMusicBrainz
	case strings.HasSuffix(path, "-lookup/json"), path == "/spotify-id-from-mbid/json", strings.HasPrefix(path, "/1/metadata/"):
		return metadata.SourceListenBrainz
	}
	return metadata.SourceDeezer
}

var albums = map[string]bool{"901415162": true, "909591702": true, "901480252": true, "903794362": true}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	src := sourceOf(r.URL.Path)
	s.mu.Lock()
	s.hits[src]++
	quota := false
	if src == metadata.SourceDeezer && s.DeezerQuota > 0 {
		s.DeezerQuota--
		quota = true
	}
	fail := s.Fail
	s.mu.Unlock()
	if fail != nil {
		if code := fail(r); code != 0 {
			w.WriteHeader(code)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	q := strings.ToLower(r.URL.Query().Get("q") + r.URL.Query().Get("query"))
	notFound := func() {
		switch src {
		case metadata.SourceDeezer:
			_, _ = w.Write([]byte(`{"error":{"type":"DataException","message":"no data","code":800}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Not Found"}`))
		}
	}
	empty := `{"data":[],"total":0}`
	switch p := r.URL.Path; {
	case quota:
		_, _ = w.Write([]byte(`{"error":{"type":"Exception","message":"Quota limit exceeded","code":4}}`))
	case p == "/oembed":
		switch r.URL.Query().Get("url") {
		case "spotify:track:" + TrackID:
			_, _ = w.Write(fixture("oembed_track.json"))
		case "spotify:artist:" + ArtistID:
			_, _ = w.Write(fixture("oembed_artist.json"))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	case p == "/search":
		if strings.Contains(q, "never gonna give you up") {
			_, _ = w.Write(fixture("deezer_search_track.json"))
		} else {
			_, _ = w.Write([]byte(empty))
		}
	case p == "/search/album":
		if strings.Contains(q, "whenever you need somebody") {
			_, _ = w.Write(fixture("deezer_search_album.json"))
		} else {
			_, _ = w.Write([]byte(empty))
		}
	case p == "/search/artist":
		if strings.Contains(q, "rick astley") {
			_, _ = w.Write(fixture("deezer_search_artist.json"))
		} else {
			_, _ = w.Write([]byte(empty))
		}
	case strings.HasPrefix(p, "/album/") && albums[strings.TrimPrefix(p, "/album/")]:
		var a map[string]any
		_ = json.Unmarshal(fixture("deezer_album.json"), &a)
		a["id"] = json.Number(strings.TrimPrefix(p, "/album/"))
		_ = json.NewEncoder(w).Encode(a)
	case p == "/acrr-lookup/json" || p == "/acr-lookup/json":
		s.lbLookup(w, r)
	case p == "/spotify-id-from-mbid/json":
		var items []map[string]string
		_ = json.NewDecoder(r.Body).Decode(&items)
		out := []json.RawMessage{}
		for _, it := range items {
			if it["recording_mbid"] == RecordingMBID {
				var rows []json.RawMessage
				_ = json.Unmarshal(fixture("lb_spotify_ids.json"), &rows)
				out = append(out, rows...)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	case p == "/1/metadata/recording/":
		if strings.Contains(r.URL.Query().Get("recording_mbids"), RecordingMBID) {
			_, _ = w.Write(fixture("lb_metadata_recording.json"))
		} else {
			_, _ = w.Write([]byte(`{}`))
		}
	case p == "/ws/2/isrc/"+ISRC:
		_, _ = w.Write(fixture("mb_isrc.json"))
	case p == "/ws/2/artist/"+MBID:
		_, _ = w.Write(fixture("mb_artist.json"))
	case p == "/ws/2/artist":
		if strings.Contains(q, "rick astley") {
			_, _ = w.Write(fixture("mb_search_artist.json"))
		} else {
			_, _ = w.Write([]byte(`{"artists":[]}`))
		}
	default:
		notFound()
	}
}

// lbLookup answers a labs name lookup: a hit for items naming the
// recorded track by the recorded artist (and album, when given).
func (s *Server) lbLookup(w http.ResponseWriter, r *http.Request) {
	var items []map[string]string
	_ = json.NewDecoder(r.Body).Decode(&items)
	out := []map[string]any{}
	for i, it := range items {
		if metadata.Fold(it["artist_credit_name"]) != metadata.Fold(Artist) ||
			metadata.Fold(it["recording_name"]) != metadata.Fold(Track) {
			continue
		}
		if rel, ok := it["release_name"]; ok && metadata.Fold(rel) != metadata.Fold(Album) {
			continue
		}
		var hit map[string]any
		_ = json.Unmarshal(fixture("lb_lookup_hit.json"), &hit)
		hit["index"] = i
		out = append(out, hit)
	}
	_ = json.NewEncoder(w).Encode(out)
}
