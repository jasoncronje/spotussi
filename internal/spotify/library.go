package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Library sync (D36): saved tracks, saved albums, the user's playlists
// and the items of the playlists the user owns or collaborates on. Every
// list is a page of at most 50 items; each item keeps its verbatim JSON
// for the raw layer (D3). None of these are catalog lookups: /me/… is in
// the user quota bucket and /playlists/… in its own (D24).

// LibraryPageSize is the largest page the library endpoints return.
const LibraryPageSize = 50

// Page is one page of a list: Spotify's total and the items with their
// verbatim JSON.
type Page[T any] struct {
	Total int
	Items []T
	Raw   []json.RawMessage
}

// SavedTrack is an item of GET /me/tracks.
type SavedTrack struct {
	AddedAt string `json:"added_at"`
	Track   Track  `json:"track"`
}

// SavedAlbum is an item of GET /me/albums (the album's own track list is
// not read).
type SavedAlbum struct {
	AddedAt string      `json:"added_at"`
	Album   SimpleAlbum `json:"album"`
}

// PlaylistSummary is an item of GET /me/playlists. Its item count is in
// "items" since Feb 2026 ("tracks" before).
type PlaylistSummary struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Owner         PlaylistOwner `json:"owner"`
	Collaborative bool          `json:"collaborative"`
	Public        *bool         `json:"public"`
	SnapshotID    string        `json:"snapshot_id"`
	Images        []Image       `json:"images"`
	Items         *struct {
		Total int `json:"total"`
	} `json:"items"`
	Tracks *struct {
		Total int `json:"total"`
	} `json:"tracks"`
}

// ItemsTotal is the playlist's item count (-1 when not given).
func (p *PlaylistSummary) ItemsTotal() int {
	switch {
	case p.Items != nil:
		return p.Items.Total
	case p.Tracks != nil:
		return p.Tracks.Total
	}
	return -1
}

// PlaylistItem is an item of GET /playlists/{id}/items. The object is in
// "item" since Feb 2026 ("track" before, still sent, deprecated). Only
// tracks are synced (D11); Episode reports any other item.
type PlaylistItem struct {
	AddedAt string `json:"added_at"`
	AddedBy *struct {
		ID string `json:"id"`
	} `json:"added_by"`
	IsLocal bool   `json:"is_local"`
	Item    *Track `json:"item"`
	Track   *Track `json:"track"`
}

// Object is the item's track (nil for episodes and missing items).
func (p *PlaylistItem) Object() *Track {
	t := p.Item
	if t == nil {
		t = p.Track
	}
	if t == nil || (t.Type != "" && t.Type != "track") || (t.ID == "" && t.URI == "") {
		return nil
	}
	return t
}

// AddedByID returns the adder's user ID ("" when unknown).
func (p *PlaylistItem) AddedByID() string {
	if p.AddedBy == nil {
		return ""
	}
	return p.AddedBy.ID
}

// ParseAddedAt parses an added_at timestamp to epoch ms (0 when absent or
// malformed).
func ParseAddedAt(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// SavedTracks reads a page of saved tracks, newest first.
func (c *Client) SavedTracks(ctx context.Context, offset int) (*Page[SavedTrack], error) {
	return getPage[SavedTrack](ctx, c, "/me/tracks", offset, nil)
}

// SavedAlbums reads a page of saved albums, newest first.
func (c *Client) SavedAlbums(ctx context.Context, offset int) (*Page[SavedAlbum], error) {
	return getPage[SavedAlbum](ctx, c, "/me/albums", offset, nil)
}

// MyPlaylists reads a page of the user's playlists (own, collaborative
// and followed), in the user's order.
func (c *Client) MyPlaylists(ctx context.Context, offset int) (*Page[PlaylistSummary], error) {
	return getPage[PlaylistSummary](ctx, c, "/me/playlists", offset, nil)
}

// PlaylistItems reads a page of a playlist's items. Spotify answers 403
// unless the user owns or collaborates on the playlist (Feb 2026).
func (c *Client) PlaylistItems(ctx context.Context, id string, offset int) (*Page[PlaylistItem], error) {
	if !ValidID(id) {
		return nil, ErrInvalidID
	}
	return getPage[PlaylistItem](ctx, c, "/playlists/"+id+"/items", offset, nil)
}

func getPage[T any](ctx context.Context, c *Client, path string, offset int, q url.Values) (*Page[T], error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("limit", strconv.Itoa(LibraryPageSize))
	q.Set("offset", strconv.Itoa(offset))
	resp, err := c.Do(ctx, http.MethodGet, path+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp.StatusCode, body)
	}
	var raw struct {
		Total int               `json:"total"`
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, &APIError{Status: resp.StatusCode, Message: "malformed " + path + " response: " + err.Error()}
	}
	p := &Page[T]{Total: raw.Total, Items: make([]T, 0, len(raw.Items)), Raw: make([]json.RawMessage, 0, len(raw.Items))}
	for _, it := range raw.Items {
		if len(it) == 0 || bytes.Equal(it, []byte("null")) {
			continue
		}
		var v T
		if err := json.Unmarshal(it, &v); err != nil {
			return nil, &APIError{Status: resp.StatusCode, Message: "malformed " + path + " item: " + err.Error()}
		}
		p.Items = append(p.Items, v)
		p.Raw = append(p.Raw, it)
	}
	return p, nil
}
