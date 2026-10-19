package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Catalog lookups (M4, D9, D17): one entity per request, since batch
// endpoints are gone for Development Mode apps. No market is sent, so
// Spotify answers with the requested track rather than relinking it.

// Artist is a full artist object.
type Artist struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	URI    string   `json:"uri"`
	Genres []string `json:"genres"`
	Images []Image  `json:"images"`
}

// Album is a full album object (only the fields the catalog keeps).
type Album struct {
	SimpleAlbum
}

// PlaylistOwner is a playlist's owner.
type PlaylistOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Playlist is the part of a playlist object context resolution reads.
type Playlist struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Owner  PlaylistOwner `json:"owner"`
	Images []Image       `json:"images"`
}

// ErrInvalidID is returned for an ID that cannot be a Spotify ID; nothing
// is sent.
var ErrInvalidID = errors.New("not a Spotify ID")

// ValidID reports whether id can be a Spotify ID and is safe in a path:
// base62 (22 characters today), leniently also - and _, up to 64.
func ValidID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// StatusOf returns the HTTP status of a Spotify API error (0 if err is
// not one).
func StatusOf(err error) int {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

// Track fetches one track.
func (c *Client) Track(ctx context.Context, id string) (*Track, error) {
	var t Track
	if err := c.getJSON(ctx, "/tracks/", id, "", &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Artist fetches one artist (genres and images).
func (c *Client) Artist(ctx context.Context, id string) (*Artist, error) {
	var a Artist
	if err := c.getJSON(ctx, "/artists/", id, "", &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// Album fetches one album.
func (c *Client) Album(ctx context.Context, id string) (*Album, error) {
	var a Album
	if err := c.getJSON(ctx, "/albums/", id, "", &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// Playlist fetches a playlist's name, owner and images (not its tracks).
// Private playlists of the connected account need playlist-read-private;
// Spotify-made playlists fail for apps created after Nov 2024 (D17).
func (c *Client) Playlist(ctx context.Context, id string) (*Playlist, error) {
	var p Playlist
	q := url.Values{"fields": {"id,name,owner(id,display_name),images"}}
	if err := c.getJSON(ctx, "/playlists/", id, q.Encode(), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) getJSON(ctx context.Context, prefix, id, query string, dst any) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	path := prefix + id
	if query != "" {
		path += "?" + query
	}
	resp, err := c.Do(ctx, http.MethodGet, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read %s: %w", prefix, err)
	}
	if resp.StatusCode != http.StatusOK {
		return apiError(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return &APIError{Status: resp.StatusCode, Message: "malformed " + prefix + " response: " + err.Error()}
	}
	return nil
}
