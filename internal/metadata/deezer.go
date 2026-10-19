package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Deezer's public API (api.deezer.com): no key, documented limit of 50
// requests per 5 s per IP. Over the limit it answers 200 with
// {"error": {"code": 4, "message": "Quota limit exceeded"}}; code 800 is
// "no data" (unknown ID).
//
// Search syntax (verified Oct 2026): free text plus the track:"…" and
// album:"…" filters work; the artist:"…" filter returns nothing, so the
// artist goes in as free text.

// DefaultDeezerURL is the API's origin.
const DefaultDeezerURL = "https://api.deezer.com"

// DeezerRate is the default sustained rate (the documented limit is 10/s).
const DeezerRate = 6.0

// Deezer is the Deezer API client.
type Deezer struct{ base }

// NewDeezer builds a Deezer client. baseURL "" is DefaultDeezerURL; lim
// nil uses DeezerRate.
func NewDeezer(baseURL, ua string, hc *http.Client, lim *Limiter) *Deezer {
	if baseURL == "" {
		baseURL = DefaultDeezerURL
	}
	if lim == nil {
		lim = NewLimiter(DeezerRate, 5)
	}
	return &Deezer{newBase(SourceDeezer, baseURL, ua, hc, lim, 5*time.Second)}
}

// DeezerArtist is an artist (simplified in search results).
type DeezerArtist struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	PictureMedium string `json:"picture_medium"`
	PictureXL     string `json:"picture_xl"`
	Fans          int64  `json:"nb_fan"`
}

// DeezerAlbum is an album (simplified in search results; Album fills
// the rest).
type DeezerAlbum struct {
	ID          int64        `json:"id"`
	Title       string       `json:"title"`
	CoverMedium string       `json:"cover_medium"`
	CoverXL     string       `json:"cover_xl"`
	RecordType  string       `json:"record_type"` // album, single, ep, compile
	ReleaseDate string       `json:"release_date"`
	Tracks      int          `json:"nb_tracks"`
	Artist      DeezerArtist `json:"artist"`
}

// DeezerTrack is a track search result.
type DeezerTrack struct {
	ID       int64        `json:"id"`
	Title    string       `json:"title"`
	ISRC     string       `json:"isrc"`
	Duration int          `json:"duration"` // seconds
	Explicit bool         `json:"explicit_lyrics"`
	Rank     int64        `json:"rank"`
	Artist   DeezerArtist `json:"artist"`
	Album    DeezerAlbum  `json:"album"`
}

type deezerError struct {
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// decode handles Deezer's in-body errors before decoding v.
func (d *Deezer) decode(v any) func([]byte) error {
	return func(b []byte) error {
		var e deezerError
		if json.Unmarshal(b, &e) == nil && e.Error != nil {
			switch e.Error.Code {
			case 4: // quota
				return errLimited{backoff: 5 * time.Second}
			case 800: // no data
				return ErrNotFound
			}
			return fmt.Errorf("deezer: error %d: %s", e.Error.Code, e.Error.Message)
		}
		return decodeJSON(d.source, v)(b)
	}
}

var deezerLimited = statusLimited(http.StatusTooManyRequests)

func (d *Deezer) search(ctx context.Context, kind, q string, limit int, v any) error {
	path := "/search"
	if kind != "" {
		path += "/" + kind
	}
	return d.get(ctx, path+"?q="+url.QueryEscape(q)+"&limit="+strconv.Itoa(limit), deezerLimited, d.decode(v))
}

// SearchTracks runs a track search.
func (d *Deezer) SearchTracks(ctx context.Context, q string, limit int) ([]DeezerTrack, error) {
	var r struct{ Data []DeezerTrack }
	if err := d.search(ctx, "", q, limit, &r); err != nil {
		return nil, err
	}
	return r.Data, nil
}

// SearchAlbums runs an album search.
func (d *Deezer) SearchAlbums(ctx context.Context, q string, limit int) ([]DeezerAlbum, error) {
	var r struct{ Data []DeezerAlbum }
	if err := d.search(ctx, "album", q, limit, &r); err != nil {
		return nil, err
	}
	return r.Data, nil
}

// SearchArtists runs an artist search.
func (d *Deezer) SearchArtists(ctx context.Context, q string, limit int) ([]DeezerArtist, error) {
	var r struct{ Data []DeezerArtist }
	if err := d.search(ctx, "artist", q, limit, &r); err != nil {
		return nil, err
	}
	return r.Data, nil
}

// Album fetches a full album.
func (d *Deezer) Album(ctx context.Context, id int64) (*DeezerAlbum, error) {
	var a DeezerAlbum
	if err := d.get(ctx, "/album/"+strconv.FormatInt(id, 10), deezerLimited, d.decode(&a)); err != nil {
		return nil, err
	}
	return &a, nil
}

// Quote makes s safe inside a Deezer filter's double quotes.
func Quote(s string) string {
	return `"` + strings.NewReplacer(`"`, " ", `\`, " ").Replace(s) + `"`
}

// DeezerImage reports whether a Deezer image URL is a real picture:
// entities without one get a placeholder with an empty hash
// (…/images/artist//500x500-…).
func DeezerImage(u string) bool {
	return u != "" && !strings.Contains(u, "//250x250") && !strings.Contains(u, "//1000x1000") &&
		!strings.Contains(u, "//500x500") && !strings.Contains(u, "//56x56")
}

// SpotifyAlbumType maps a Deezer record type to Spotify's album_type
// (Spotify files EPs as singles).
func SpotifyAlbumType(recordType string) string {
	switch recordType {
	case "album":
		return "album"
	case "single", "ep":
		return "single"
	case "compile":
		return "compilation"
	}
	return ""
}
