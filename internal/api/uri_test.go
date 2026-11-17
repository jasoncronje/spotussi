package api

import (
	"encoding/json"
	"testing"
)

func TestSpotifyURI(t *testing.T) {
	for _, c := range []struct {
		kind, id string
		want     *string
	}{
		{"track", "11dFghVXANMlKmJXsNCbNl", ptr("spotify:track:11dFghVXANMlKmJXsNCbNl")},
		{"artist", "0OdUWJ0sBjDrqHygGUXeCF", ptr("spotify:artist:0OdUWJ0sBjDrqHygGUXeCF")},
		{"artist", "ar_dGhlIGJhbmQ", nil},                     // provisional (D20)
		{"album", "al_dGhlIGJhbmQAbGl2ZQ", nil},               // provisional (D20)
		{"track", "spotify:local:Artist:Album:Song:180", nil}, // local file
		{"track", "", nil},
	} {
		got := SpotifyURI(c.kind, c.id)
		if (got == nil) != (c.want == nil) || got != nil && *got != *c.want {
			t.Errorf("SpotifyURI(%q, %q) = %v, want %v", c.kind, c.id, deref(got), deref(c.want))
		}
	}
}

// Every ref and entity emits uri, null when the ID is not a Spotify ID.
func TestEntityURIOnTheWire(t *testing.T) {
	for _, c := range []struct {
		v    any
		want any
	}{
		{TrackRef{ID: "abc"}, "spotify:track:abc"},
		{ArtistRef{ID: "abc"}, "spotify:artist:abc"},
		{AlbumRef{ID: "abc"}, "spotify:album:abc"},
		{TrackFull{ID: "abc"}, "spotify:track:abc"},
		{ArtistFull{ID: "abc"}, "spotify:artist:abc"},
		{AlbumFull{ID: "abc"}, "spotify:album:abc"},
		{ArtistRef{ID: "ar_eA"}, nil},
		{AlbumFull{ID: "al_eA"}, nil},
	} {
		b, err := json.Marshal(c.v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		if v, ok := m["uri"]; !ok || v != c.want {
			t.Errorf("%T %s: uri %v, want %v", c.v, b, v, c.want)
		}
		if m["id"] == nil {
			t.Errorf("%T %s: fields lost", c.v, b)
		}
	}
}

func ptr(s string) *string { return &s }

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
