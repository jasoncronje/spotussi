package metadata

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Spotify oEmbed (open.spotify.com/oembed): public, no auth, so it can't
// count against a developer account's Web API quota. Exact by URI: the
// track's title and its album cover, or an artist's name and image.
//
// Measured Oct 2026: 10 requests/s sustained without errors, 25/s drew
// 429s (without Retry-After) after about 3 s. The default rate is 4/s.

// DefaultOEmbedURL is the oEmbed endpoint's origin.
const DefaultOEmbedURL = "https://open.spotify.com"

// OEmbedRate is the default sustained request rate.
const OEmbedRate = 4.0

// OEmbed is the Spotify oEmbed client.
type OEmbed struct{ base }

// NewOEmbed builds an oEmbed client. baseURL "" is DefaultOEmbedURL; lim
// nil uses OEmbedRate.
func NewOEmbed(baseURL, ua string, hc *http.Client, lim *Limiter) *OEmbed {
	if baseURL == "" {
		baseURL = DefaultOEmbedURL
	}
	if lim == nil {
		lim = NewLimiter(OEmbedRate, 4)
	}
	return &OEmbed{newBase(SourceOEmbed, baseURL, ua, hc, lim, 30*time.Second)}
}

// OEmbedResult is an oEmbed answer.
type OEmbedResult struct {
	Title        string `json:"title"`
	ThumbnailURL string `json:"thumbnail_url"`
	ThumbWidth   int    `json:"thumbnail_width"`
}

// Lookup fetches the oEmbed data of a Spotify URI (spotify:track:ID,
// spotify:artist:ID, ...). An unknown ID is ErrNotFound.
func (o *OEmbed) Lookup(ctx context.Context, uri string) (*OEmbedResult, error) {
	var r OEmbedResult
	err := o.get(ctx, "/oembed?url="+url.QueryEscape(uri), statusLimited(http.StatusTooManyRequests),
		decodeJSON(o.source, &r))
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Spotify image URLs encode the size in the first 16 hex digits of the
// file name: ab67616d (album cover) + 0000b273 (640 px), 00001e02 (300)
// or 00004851 (64); ab676161 (artist) + 0000e5eb (640), 00005174 (320)
// or 0000f178 (160). oEmbed returns the 300/320 px one on a CDN alias.

// SpotifyImages turns an oEmbed thumbnail into the large and small
// images Spotify's API would return (as the catalog stores them: largest,
// and smallest of at least 64 px), on Spotify's canonical image host.
// Unrecognized URLs are returned as both.
func SpotifyImages(thumb string) (large, small string) {
	i := strings.LastIndex(thumb, "/image/")
	if i < 0 || len(thumb) < i+len("/image/")+16 {
		return thumb, thumb
	}
	file := thumb[i+len("/image/"):]
	kind, size, hash := file[:8], file[8:16], file[16:]
	sizes := map[string][2]string{
		"ab67616d": {"0000b273", "00004851"},
		"ab676161": {"0000e5eb", "0000f178"},
	}
	s, ok := sizes[kind]
	if !ok || strings.ContainsAny(hash, "/?#") || hash == "" {
		return thumb, thumb
	}
	known := size == s[0] || size == s[1] || size == "00001e02" || size == "00005174"
	if !known {
		return thumb, thumb
	}
	const host = "https://i.scdn.co/image/"
	return host + kind + s[0] + hash, host + kind + s[1] + hash
}
