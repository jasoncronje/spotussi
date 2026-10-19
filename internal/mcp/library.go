package mcp

import (
	"context"
	"fmt"

	"spotussi/internal/api"
)

const maxLibraryLimit = 50

// ---------- get_library ----------

type libraryArgs struct {
	View       string `json:"view,omitempty" jsonschema:"summary (default), tracks (saved songs), albums (saved albums), playlists, or playlist (one playlist's songs; needs playlist_id)"`
	Played     string `json:"played,omitempty" jsonschema:"tracks only: all (default), played, maybe_played (no plays of this exact track but a played song with the same title and artist) or never_played"`
	Sort       string `json:"sort,omitempty" jsonschema:"tracks: added (default), plays, last_played; albums: added (default), plays, played_share; playlists: plays_here (default), played_share, name"`
	Relation   string `json:"relation,omitempty" jsonschema:"playlists only: all (default), own, collaborative or followed"`
	PlaylistID string `json:"playlist_id,omitempty" jsonschema:"the playlist for view=playlist (an id from view=playlists)"`
	Limit      int    `json:"limit,omitempty" jsonschema:"items to return, 1-50 (default 20)"`
	Offset     int    `json:"offset,omitempty" jsonschema:"items to skip, for paging (default 0)"`
}

type librarySummaryOut struct {
	LastSync    *string                 `json:"last_sync"`
	SavedTracks int                     `json:"saved_tracks"`
	SavedAlbums int                     `json:"saved_albums"`
	Playlists   int                     `json:"playlists"`
	Tracks      api.LibraryStatusCounts `json:"saved_tracks_by_status"`
	// Heavy rotation is the RotationPlays-th counted play.
	RotationPlays         int             `json:"rotation_plays"`
	MedianDaysToFirstPlay *float64        `json:"median_days_save_to_first_play"`
	MedianDaysToRotation  *float64        `json:"median_days_save_to_rotation"`
	RotationAfterSaving   api.RotationMix `json:"rotation_after_saving"`
}

type libraryTrackOut struct {
	trackOut
	Saved      *string `json:"saved"`
	Status     string  `json:"status"`
	Plays      int64   `json:"plays"`
	Minutes    float64 `json:"minutes"`
	FirstPlay  *string `json:"first_play,omitempty"`
	LastPlay   *string `json:"last_play,omitempty"`
	InRotation *string `json:"heavy_rotation_from,omitempty"`
}

type libraryAlbumOut struct {
	albumOut
	Saved        *string `json:"saved"`
	TracksTotal  *int    `json:"tracks_total"`
	TracksPlayed int     `json:"tracks_played"`
	Plays        int64   `json:"plays"`
	Minutes      float64 `json:"minutes"`
}

type libraryPlaylistOut struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Owner    *string `json:"owner"`
	Relation string  `json:"relation"`
	// Songs is absent when Spotify doesn't share the items (followed
	// playlists).
	Songs        *int    `json:"songs,omitempty"`
	SongsPlayed  *int    `json:"songs_played,omitempty"`
	ItemsTotal   *int    `json:"items_total"`
	PlaysHere    int64   `json:"plays_started_here"`
	MinutesHere  float64 `json:"minutes_here"`
	LastPlayHere *string `json:"last_play_here,omitempty"`
}

type libraryItemOut struct {
	Position int `json:"position"`
	trackOut
	Added      *string `json:"added"`
	AddedBy    *string `json:"added_by,omitempty"`
	AddedByYou bool    `json:"added_by_you"`
	Status     string  `json:"status"`
	Plays      int64   `json:"plays"`
	PlaysHere  int64   `json:"plays_here"`
}

type libraryListOut struct {
	View       string                  `json:"view"`
	Counts     *api.LibraryTrackCounts `json:"counts,omitempty"`
	Playlist   *libraryPlaylistOut     `json:"playlist,omitempty"`
	Total      int                     `json:"total"`
	Offset     int                     `json:"offset"`
	NextOffset *int                    `json:"next_offset"`
	Items      any                     `json:"items"`
}

func compactPlaylist(p api.LibraryPlaylist, tz string) *libraryPlaylistOut {
	return &libraryPlaylistOut{ID: p.Playlist.ID, Name: p.Playlist.Name, Owner: p.Playlist.OwnerName, Relation: p.Playlist.Relation,
		Songs: p.Tracks, SongsPlayed: p.TracksPlayed, ItemsTotal: p.ItemsTotal, PlaysHere: p.PlaysHere, MinutesHere: minutes(p.MsHere),
		LastPlayHere: localTimePtr(p.LastPlayedHereAt, tz)}
}

func library(ctx context.Context, b api.Backend, in libraryArgs) (any, error) {
	set, err := b.Settings(ctx)
	if err != nil {
		return nil, err
	}
	tz := set.Timezone
	limit, err := clamp(in.Limit, 20, 1, maxLibraryLimit, "limit")
	if err != nil {
		return nil, err
	}
	if in.Offset < 0 {
		return nil, fmt.Errorf("%w: offset must not be negative", api.ErrBadRequest)
	}
	pg := api.Page{Limit: limit, Offset: in.Offset}
	oneOf := func(name, v string, ok ...string) (string, error) {
		if v == "" {
			return ok[0], nil
		}
		for _, o := range ok {
			if v == o {
				return v, nil
			}
		}
		return "", fmt.Errorf("%w: %s must be one of %v", api.ErrBadRequest, name, ok)
	}
	out := &libraryListOut{View: in.View, Offset: in.Offset}
	var n int
	switch in.View {
	case "", "summary":
		s, err := b.LibrarySummary(ctx)
		if err != nil {
			return nil, err
		}
		return &librarySummaryOut{LastSync: localTimePtr(s.LastSyncAt, tz), SavedTracks: s.SavedTracks, SavedAlbums: s.SavedAlbums,
			Playlists: s.Playlists, Tracks: s.Tracks, RotationPlays: s.RotationPlays, MedianDaysToFirstPlay: s.DaysToFirstPlay.Median,
			MedianDaysToRotation: s.DaysToRotation.Median, RotationAfterSaving: s.RotationMix}, nil
	case "tracks":
		played, err := oneOf("played", in.Played, "all", api.StatusPlayed, api.StatusMaybePlayed, api.StatusNeverPlayed)
		if err != nil {
			return nil, err
		}
		sort, err := oneOf("sort", in.Sort, "added", "plays", "last_played")
		if err != nil {
			return nil, err
		}
		r, err := b.LibraryTracks(ctx, api.LibraryTracksParams{Played: played, Sort: sort, Page: pg})
		if err != nil {
			return nil, err
		}
		items := make([]libraryTrackOut, len(r.Items))
		for i, t := range r.Items {
			items[i] = libraryTrackOut{trackOut: compactTrack(t.Track), Saved: localTimePtr(t.AddedAt, tz), Status: t.Status,
				Plays: t.Plays, Minutes: minutes(t.MsPlayed), FirstPlay: localTimePtr(t.FirstPlayedAt, tz),
				LastPlay: localTimePtr(t.LastPlayedAt, tz), InRotation: localTimePtr(t.RotationAt, tz)}
		}
		out.Counts, out.Total, out.Items, n = &r.Counts, r.Total, items, len(items)
	case "albums":
		sort, err := oneOf("sort", in.Sort, "added", "plays", "played_share")
		if err != nil {
			return nil, err
		}
		r, err := b.LibraryAlbums(ctx, api.LibraryAlbumsParams{Sort: sort, Page: pg})
		if err != nil {
			return nil, err
		}
		items := make([]libraryAlbumOut, len(r.Items))
		for i, a := range r.Items {
			items[i] = libraryAlbumOut{albumOut: compactAlbum(a.Album), Saved: localTimePtr(a.AddedAt, tz), TracksTotal: a.TracksTotal,
				TracksPlayed: a.TracksPlayed, Plays: a.Plays, Minutes: minutes(a.MsPlayed)}
		}
		out.Total, out.Items, n = r.Total, items, len(items)
	case "playlists":
		rel, err := oneOf("relation", in.Relation, "all", "own", "collaborative", "followed")
		if err != nil {
			return nil, err
		}
		sort, err := oneOf("sort", in.Sort, "plays_here", "played_share", "name")
		if err != nil {
			return nil, err
		}
		r, err := b.LibraryPlaylists(ctx, api.LibraryPlaylistsParams{Relation: rel, Sort: sort, Page: pg})
		if err != nil {
			return nil, err
		}
		items := make([]*libraryPlaylistOut, len(r.Items))
		for i, p := range r.Items {
			items[i] = compactPlaylist(p, tz)
		}
		out.Total, out.Items, n = r.Total, items, len(items)
	case "playlist":
		if in.PlaylistID == "" {
			return nil, fmt.Errorf("%w: view=playlist needs playlist_id", api.ErrBadRequest)
		}
		r, err := b.LibraryPlaylist(ctx, in.PlaylistID, pg)
		if err != nil {
			return nil, err
		}
		items := make([]libraryItemOut, len(r.Items))
		for i, it := range r.Items {
			items[i] = libraryItemOut{Position: it.Position, trackOut: compactTrack(it.Track), Added: localTimePtr(it.AddedAt, tz),
				AddedBy: it.AddedBy, AddedByYou: it.AddedByOwner, Status: it.Status, Plays: it.Plays, PlaysHere: it.PlaysHere}
		}
		out.Playlist, out.Total, out.Items, n = compactPlaylist(r.Playlist, tz), r.Total, items, len(items)
	default:
		return nil, fmt.Errorf("%w: view must be summary, tracks, albums, playlists or playlist", api.ErrBadRequest)
	}
	out.NextOffset = nextOffset(in.Offset, n, out.Total)
	return out, nil
}
