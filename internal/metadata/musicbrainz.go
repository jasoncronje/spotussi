package metadata

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MusicBrainz (musicbrainz.org/ws/2): no key, 1 request per second per
// client, and a meaningful User-Agent is required. Over the limit it
// answers 503.

// DefaultMusicBrainzURL is the web service's origin.
const DefaultMusicBrainzURL = "https://musicbrainz.org"

// MusicBrainzRate is the documented limit.
const MusicBrainzRate = 1.0

// MusicBrainz is the MusicBrainz client.
type MusicBrainz struct{ base }

// NewMusicBrainz builds a MusicBrainz client. baseURL "" is
// DefaultMusicBrainzURL; lim nil uses MusicBrainzRate.
func NewMusicBrainz(baseURL, ua string, hc *http.Client, lim *Limiter) *MusicBrainz {
	if baseURL == "" {
		baseURL = DefaultMusicBrainzURL
	}
	if lim == nil {
		lim = NewLimiter(MusicBrainzRate, 1)
	}
	return &MusicBrainz{newBase(SourceMusicBrainz, baseURL, ua, hc, lim, 5*time.Second)}
}

var mbLimited = statusLimited(http.StatusServiceUnavailable, http.StatusTooManyRequests)

// MBArtistCredit is an artist credited on a recording.
type MBArtistCredit struct {
	Name   string `json:"name"`
	Artist struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artist"`
}

// MBRecording is a recording with its artist credits.
type MBRecording struct {
	ID      string           `json:"id"`
	Title   string           `json:"title"`
	Credits []MBArtistCredit `json:"artist-credit"`
}

// ISRC returns the recordings with an ISRC (none: ErrNotFound).
func (m *MusicBrainz) ISRC(ctx context.Context, isrc string) ([]MBRecording, error) {
	var r struct {
		Recordings []MBRecording `json:"recordings"`
	}
	if err := m.get(ctx, "/ws/2/isrc/"+url.PathEscape(isrc)+"?inc=artist-credits&fmt=json", mbLimited,
		decodeJSON(m.source, &r)); err != nil {
		return nil, err
	}
	return r.Recordings, nil
}

// MBArtist is an artist search hit or lookup.
type MBArtist struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Score          int       `json:"score"` // search only, 0-100
	Disambiguation string    `json:"disambiguation"`
	Genres         []MBGenre `json:"genres"` // lookup with inc=genres
	// Relations are the artist's URL links (lookup with inc=url-rels).
	Relations []struct {
		Type string `json:"type"`
		URL  struct {
			Resource string `json:"resource"`
		} `json:"url"`
	} `json:"relations"`
}

// SpotifyArtistID is the Spotify artist ID of the artist's one
// open.spotify.com/artist/ link ("": none, or several). Stored, not used
// (D37).
func (a *MBArtist) SpotifyArtistID() string {
	id := ""
	for _, r := range a.Relations {
		u, err := url.Parse(r.URL.Resource)
		if err != nil || u.Host != "open.spotify.com" {
			continue
		}
		rest, ok := strings.CutPrefix(u.Path, "/artist/")
		if !ok || !spotifyID(rest) {
			continue
		}
		if id != "" && id != rest {
			return ""
		}
		id = rest
	}
	return id
}

// spotifyID reports whether s looks like a Spotify ID (22 base62 chars).
func spotifyID(s string) bool {
	if len(s) != 22 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// MBGenre is a curated genre with its vote count.
type MBGenre struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// SearchArtists searches artists by exact name.
func (m *MusicBrainz) SearchArtists(ctx context.Context, name string, limit int) ([]MBArtist, error) {
	var r struct {
		Artists []MBArtist `json:"artists"`
	}
	q := url.Values{"query": {"artist:" + luceneQuote(name)}, "fmt": {"json"}, "limit": {itoa(limit)}}
	if err := m.get(ctx, "/ws/2/artist?"+q.Encode(), mbLimited, decodeJSON(m.source, &r)); err != nil {
		return nil, err
	}
	return r.Artists, nil
}

// Artist looks up an artist with its genres and URL links.
func (m *MusicBrainz) Artist(ctx context.Context, mbid string) (*MBArtist, error) {
	var a MBArtist
	if err := m.get(ctx, "/ws/2/artist/"+url.PathEscape(mbid)+"?inc=genres+url-rels&fmt=json", mbLimited,
		decodeJSON(m.source, &a)); err != nil {
		return nil, err
	}
	return &a, nil
}

// TopGenres maps MusicBrainz genres to catalog genres (D27): curated
// genres only, the n most voted, keeping those with at least a third of
// the top genre's votes; names as MusicBrainz spells them, lower-cased.
func TopGenres(gs []MBGenre, n int) []string {
	gs = append([]MBGenre(nil), gs...)
	sort.SliceStable(gs, func(i, j int) bool {
		if gs[i].Count != gs[j].Count {
			return gs[i].Count > gs[j].Count
		}
		return gs[i].Name < gs[j].Name
	})
	var out []string
	seen := map[string]bool{}
	for _, g := range gs {
		name := strings.ToLower(strings.TrimSpace(g.Name))
		if name == "" || seen[name] || g.Count <= 0 {
			continue
		}
		if len(out) > 0 && g.Count*3 < gs[0].Count {
			break
		}
		seen[name] = true
		out = append(out, name)
		if len(out) == n {
			break
		}
	}
	return out
}

func luceneQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

func itoa(n int) string {
	if n <= 0 {
		n = 10
	}
	return strconv.Itoa(n)
}
