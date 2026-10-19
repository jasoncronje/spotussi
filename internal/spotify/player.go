package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// Web API objects used by live capture. Only the fields the app reads are
// typed; the verbatim JSON is kept alongside (raw layer, D3).

// Image is an artwork image.
type Image struct {
	URL    string `json:"url"`
	Width  *int   `json:"width"`
	Height *int   `json:"height"`
}

// SimpleArtist is an artist reference inside a track.
type SimpleArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// SimpleAlbum is the album inside a track.
type SimpleAlbum struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	URI                  string         `json:"uri"`
	AlbumType            string         `json:"album_type"`
	ReleaseDate          string         `json:"release_date"`
	ReleaseDatePrecision string         `json:"release_date_precision"`
	TotalTracks          int            `json:"total_tracks"`
	Images               []Image        `json:"images"`
	Artists              []SimpleArtist `json:"artists"`
}

// TrackLink is the original track of a relinked one (linked_from).
type TrackLink struct {
	ID  string `json:"id"`
	URI string `json:"uri"`
}

// Track is a full track object.
type Track struct {
	ID         string         `json:"id"` // empty for local files
	URI        string         `json:"uri"`
	Name       string         `json:"name"`
	DurationMs int64          `json:"duration_ms"`
	Explicit   bool           `json:"explicit"`
	IsLocal    bool           `json:"is_local"`
	Type       string         `json:"type"` // "track"
	Artists    []SimpleArtist `json:"artists"`
	Album      SimpleAlbum    `json:"album"`
	// Catalog fields (M4); absent from some simplified objects.
	DiscNumber  int `json:"disc_number"`
	TrackNumber int `json:"track_number"`
	ExternalIDs struct {
		ISRC string `json:"isrc"`
	} `json:"external_ids"`
	// LinkedFrom is set when Spotify relinked the track for the market
	// (the item is a different, playable copy of the requested track).
	LinkedFrom *TrackLink `json:"linked_from"`
}

// Key identifies the track for capture: its ID, or the URI for local
// files (which have no ID).
func (t *Track) Key() string {
	if t.ID != "" {
		return t.ID
	}
	return t.URI
}

// PlayContext is where playback started from (playlist, album, ...).
type PlayContext struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

// PlayerDevice is the active device.
type PlayerDevice struct {
	ID               *string `json:"id"`
	Name             string  `json:"name"`
	Type             string  `json:"type"`
	IsActive         bool    `json:"is_active"`
	IsPrivateSession bool    `json:"is_private_session"`
	IsRestricted     bool    `json:"is_restricted"`
}

// PlayerState is GET /me/player. Item is nil for episodes (we do not ask
// for them, D11), ads and unknown items; CurrentlyPlayingType says which.
type PlayerState struct {
	Device               *PlayerDevice `json:"device"`
	RepeatState          string        `json:"repeat_state"` // off, track, context
	ShuffleState         bool          `json:"shuffle_state"`
	Context              *PlayContext  `json:"context"`
	Timestamp            int64         `json:"timestamp"`
	ProgressMs           *int64        `json:"progress_ms"`
	IsPlaying            bool          `json:"is_playing"`
	Item                 *Track        `json:"item"`
	CurrentlyPlayingType string        `json:"currently_playing_type"` // track, episode, ad, unknown
}

// Player returns the playback state, or (nil, nil, nil) when nothing is
// playing (204 No Content). raw is the verbatim response body. Episodes
// are deliberately not requested (no additional_types, D11): they come
// back with a null item and count as gaps.
func (c *Client) Player(ctx context.Context) (st *PlayerState, raw []byte, err error) {
	resp, err := c.Do(ctx, http.MethodGet, "/me/player?market=from_token")
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("read /me/player: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusNoContent:
		return nil, nil, nil
	case resp.StatusCode != http.StatusOK:
		return nil, nil, apiError(resp.StatusCode, body)
	case len(body) == 0:
		return nil, nil, nil // some clients see 200 with an empty body
	}
	st = &PlayerState{}
	if err := json.Unmarshal(body, st); err != nil {
		return nil, nil, &APIError{Status: resp.StatusCode, Message: "malformed /me/player response: " + err.Error()}
	}
	return st, body, nil
}

// RecentItem is one play from recently-played.
type RecentItem struct {
	Track    Track        `json:"track"`
	PlayedAt string       `json:"played_at"` // RFC 3339
	Context  *PlayContext `json:"context"`
	// Raw is the verbatim item JSON.
	Raw json.RawMessage `json:"-"`
}

// RecentPage is one page of recently-played.
type RecentPage struct {
	Items   []RecentItem
	Next    string
	Cursors struct {
		After  string `json:"after"`
		Before string `json:"before"`
	}
}

// RecentlyPlayed returns up to limit (1-50) plays after afterMs (epoch ms;
// 0 for the most recent plays).
func (c *Client) RecentlyPlayed(ctx context.Context, afterMs int64, limit int) (*RecentPage, error) {
	q := url.Values{"limit": {strconv.Itoa(min(max(limit, 1), 50))}}
	if afterMs > 0 {
		q.Set("after", strconv.FormatInt(afterMs, 10))
	}
	resp, err := c.Do(ctx, http.MethodGet, "/me/player/recently-played?"+q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read recently-played: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp.StatusCode, body)
	}
	var page struct {
		Items   []json.RawMessage `json:"items"`
		Next    *string           `json:"next"`
		Cursors *struct {
			After  string `json:"after"`
			Before string `json:"before"`
		} `json:"cursors"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, &APIError{Status: resp.StatusCode, Message: "malformed recently-played response: " + err.Error()}
	}
	out := &RecentPage{Items: make([]RecentItem, 0, len(page.Items))}
	if page.Next != nil {
		out.Next = *page.Next
	}
	if page.Cursors != nil {
		out.Cursors.After, out.Cursors.Before = page.Cursors.After, page.Cursors.Before
	}
	for _, raw := range page.Items {
		var it RecentItem
		if err := json.Unmarshal(raw, &it); err != nil {
			return nil, &APIError{Status: resp.StatusCode, Message: "malformed recently-played item: " + err.Error()}
		}
		it.Raw = raw
		out.Items = append(out.Items, it)
	}
	return out, nil
}
