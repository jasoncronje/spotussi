package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ListenBrainz (D37) serves MusicBrainz data through keyless batch
// endpoints: the labs host's name lookups (artist credit + recording
// [+ release] → MBIDs, exact after folding case, accents and
// punctuation) and Spotify track IDs of a recording, and the API's
// recording metadata (credited artists and their tags; tags that are
// MusicBrainz genres carry a genre_mbid). The labs host is experimental,
// with no stability promise. The metadata endpoint allows 30 requests
// per 10 s and takes 100 MBIDs per request (150 gets a 502).

// Default ListenBrainz origins.
const (
	DefaultListenBrainzLabsURL = "https://labs.api.listenbrainz.org"
	DefaultListenBrainzURL     = "https://api.listenbrainz.org"
)

// ListenBrainzRate is the default request rate, well inside the
// metadata endpoint's 30 per 10 s.
const ListenBrainzRate = 2.0

// Batch limits.
const (
	// LBLookupBatch is the most items one labs lookup takes here (1,000
	// answered in under 2 s when measured).
	LBLookupBatch = 1000
	// LBMetadataBatch is the most recording MBIDs one metadata request
	// takes.
	LBMetadataBatch = 100
)

// ListenBrainz is the ListenBrainz client.
type ListenBrainz struct {
	base   // the labs host
	apiURL string
}

// NewListenBrainz builds a ListenBrainz client. labsURL "" is
// DefaultListenBrainzLabsURL, apiURL "" DefaultListenBrainzURL; lim nil
// uses ListenBrainzRate.
func NewListenBrainz(labsURL, apiURL, ua string, hc *http.Client, lim *Limiter) *ListenBrainz {
	if labsURL == "" {
		labsURL = DefaultListenBrainzLabsURL
	}
	if apiURL == "" {
		apiURL = DefaultListenBrainzURL
	}
	if lim == nil {
		lim = NewLimiter(ListenBrainzRate, 2)
	}
	return &ListenBrainz{base: newBase(SourceListenBrainz, labsURL, ua, hc, lim, 10*time.Second),
		apiURL: strings.TrimRight(apiURL, "/")}
}

// lbLimited is a 429 or 503, backing off for Retry-After or the API's
// X-RateLimit-Reset-In (seconds).
func lbLimited(resp *http.Response) (bool, time.Duration) {
	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable {
		return false, 0
	}
	if d := retryAfter(resp.Header.Get("Retry-After")); d > 0 {
		return true, d
	}
	if s, err := strconv.Atoi(resp.Header.Get("X-RateLimit-Reset-In")); err == nil && s > 0 {
		return true, time.Duration(s) * time.Second
	}
	return true, 0
}

// LBLookup is one name lookup. Release "" uses the lookup without it
// (acr-lookup), which can pick the recording of another release.
type LBLookup struct {
	Artist, Recording, Release string
}

// LBHit is a lookup's match.
type LBHit struct {
	Index       int      `json:"index"` // into the lookups
	ArtistName  string   `json:"artist_credit_name"`
	Recording   string   `json:"recording_name"`
	Release     string   `json:"release_name"`
	ArtistMBIDs []string `json:"artist_mbids"`
	RecordingID string   `json:"recording_mbid"`
	ReleaseID   string   `json:"release_mbid"`
}

// Lookup looks recordings up by name, all with a release or all
// without (at most LBLookupBatch). Lookups without a match have no hit.
func (l *ListenBrainz) Lookup(ctx context.Context, items []LBLookup, withRelease bool) ([]LBHit, error) {
	body := make([]map[string]string, len(items))
	path := "/acr-lookup/json"
	if withRelease {
		path = "/acrr-lookup/json"
	}
	for i, it := range items {
		body[i] = map[string]string{"artist_credit_name": it.Artist, "recording_name": it.Recording}
		if withRelease {
			body[i]["release_name"] = it.Release
		}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var hits []LBHit
	if err := l.send(ctx, http.MethodPost, l.baseURL+path, b, lbLimited, decodeJSON(l.source, &hits)); err != nil {
		return nil, err
	}
	out := hits[:0]
	for _, h := range hits {
		if h.Index >= 0 && h.Index < len(items) && h.RecordingID != "" {
			out = append(out, h)
		}
	}
	return out, nil
}

// SpotifyTrackIDs maps recording MBIDs to the Spotify track IDs
// ListenBrainz links them to (at most LBLookupBatch).
func (l *ListenBrainz) SpotifyTrackIDs(ctx context.Context, mbids []string) (map[string][]string, error) {
	body := make([]map[string]string, len(mbids))
	for i, m := range mbids {
		body[i] = map[string]string{"recording_mbid": m}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		RecordingID string   `json:"recording_mbid"`
		IDs         []string `json:"spotify_track_ids"`
	}
	if err := l.send(ctx, http.MethodPost, l.baseURL+"/spotify-id-from-mbid/json", b, lbLimited,
		decodeJSON(l.source, &rows)); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, r := range rows {
		out[r.RecordingID] = append(out[r.RecordingID], r.IDs...)
	}
	return out, nil
}

// LBTag is a tag with its vote count; GenreID is set when the tag is a
// MusicBrainz genre.
type LBTag struct {
	ArtistID string `json:"artist_mbid"` // artist tags only
	Tag      string `json:"tag"`
	Count    int    `json:"count"`
	GenreID  string `json:"genre_mbid"`
}

// LBArtist is an artist credited on a recording.
type LBArtist struct {
	ID   string `json:"artist_mbid"`
	Name string `json:"name"`
}

// LBRecording is a recording's metadata.
type LBRecording struct {
	Artist struct {
		Name    string     `json:"name"` // the whole credit
		Artists []LBArtist `json:"artists"`
	} `json:"artist"`
	Recording struct {
		Name     string `json:"name"`
		LengthMs int64  `json:"length"`
	} `json:"recording"`
	Tag struct {
		Artist []LBTag `json:"artist"`
	} `json:"tag"`
}

// Recordings fetches recordings' metadata with their artists' tags (at
// most LBMetadataBatch), keyed by MBID; unknown MBIDs are left out.
func (l *ListenBrainz) Recordings(ctx context.Context, mbids []string) (map[string]LBRecording, error) {
	out := map[string]LBRecording{}
	if len(mbids) == 0 {
		return out, nil
	}
	esc := make([]string, len(mbids))
	for i, m := range mbids {
		esc[i] = url.QueryEscape(m)
	}
	// Commas stay literal: the gateway's URL limit is what caps the batch.
	q := "?recording_mbids=" + strings.Join(esc, ",") + "&inc=" + url.QueryEscape("artist tag")
	if err := l.send(ctx, http.MethodGet, l.apiURL+"/1/metadata/recording/"+q, nil, lbLimited,
		decodeJSON(l.source, &out)); err != nil {
		return nil, err
	}
	return out, nil
}

// ArtistGenres returns an artist's genre tags on a recording (those with
// a genre_mbid), for TopGenres.
func (r LBRecording) ArtistGenres(artistID string) []MBGenre {
	var out []MBGenre
	for _, t := range r.Tag.Artist {
		if t.ArtistID == artistID && t.GenreID != "" {
			out = append(out, MBGenre{Name: t.Tag, Count: t.Count})
		}
	}
	return out
}
