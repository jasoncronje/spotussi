package api

import (
	"errors"
	"net/http"

	"spotussi/internal/query"
)

// Library endpoints (D36): saved tracks and albums, playlists, and how
// saves turn into plays.

// RotationPlays is the counted play that marks heavy rotation (D36).
const RotationPlays = 5

// Errors of POST /library/sync.
var (
	ErrSyncRunning  = errors.New("a library sync is already running; try again when it finishes")
	ErrSyncDisabled = errors.New("library sync is off on this instance (LIBRARY_SYNC_ENABLED=false)")
)

// LibraryTracksParams are the /library/tracks parameters.
type LibraryTracksParams struct {
	Played string // all, played, maybe_played, never_played
	Sort   string // added, plays, last_played
	Page   Page
}

// LibraryAlbumsParams are the /library/albums parameters.
type LibraryAlbumsParams struct {
	Sort string // added, plays, played_share
	Page Page
}

// LibraryPlaylistsParams are the /library/playlists parameters.
type LibraryPlaylistsParams struct {
	Relation string // all, own, collaborative, followed
	Sort     string // plays_here, played_share, name
	Page     Page
}

// Play statuses of a saved track.
const (
	StatusPlayed      = "played"
	StatusMaybePlayed = "maybe_played"
	StatusNeverPlayed = "never_played"
)

type LibraryListStatus struct {
	State     string  `json:"state"`
	LastError *string `json:"last_error"`
}

type LibraryLists struct {
	Tracks    LibraryListStatus `json:"tracks"`
	Albums    LibraryListStatus `json:"albums"`
	Playlists LibraryListStatus `json:"playlists"`
}

type LibraryStatus struct {
	State              string       `json:"state"`
	LastSyncAt         *int64       `json:"last_sync_at"`
	LastFullAt         *int64       `json:"last_full_at"`
	NextSyncAt         *int64       `json:"next_sync_at"`
	ResumesAt          *int64       `json:"resumes_at"`
	Progress           *float64     `json:"progress"`
	Lists              LibraryLists `json:"lists"`
	SavedTracks        int          `json:"saved_tracks"`
	SavedAlbums        int          `json:"saved_albums"`
	Playlists          int          `json:"playlists"`
	PlaylistsWithItems int          `json:"playlists_with_items"`
	LastError          *string      `json:"last_error"`
	LastErrorAt        *int64       `json:"last_error_at"`
}

type LibraryDays struct {
	Median *float64 `json:"median"`
	Count  int      `json:"count"`
}

type LibraryStatusCounts struct {
	Played      int `json:"played"`
	MaybePlayed int `json:"maybe_played"`
	NeverPlayed int `json:"never_played"`
}

type RotationMix struct {
	BeforeSaving int `json:"before_saving"`
	WithinDay    int `json:"within_day"`
	WithinWeek   int `json:"within_week"`
	WithinMonth  int `json:"within_month"`
	Later        int `json:"later"`
	NotYet       int `json:"not_yet"`
}

type LibrarySummary struct {
	LastSyncAt      *int64              `json:"last_sync_at"`
	SavedTracks     int                 `json:"saved_tracks"`
	SavedAlbums     int                 `json:"saved_albums"`
	Playlists       int                 `json:"playlists"`
	Tracks          LibraryStatusCounts `json:"tracks"`
	RotationPlays   int                 `json:"rotation_plays"`
	DaysToFirstPlay LibraryDays         `json:"days_to_first_play"`
	DaysToRotation  LibraryDays         `json:"days_to_rotation"`
	RotationMix     RotationMix         `json:"rotation_mix"`
}

type LibraryTrack struct {
	Track         TrackRef `json:"track"`
	AddedAt       *int64   `json:"added_at"`
	Status        string   `json:"status"`
	Plays         int64    `json:"plays"`
	MsPlayed      int64    `json:"ms_played"`
	FirstPlayedAt *int64   `json:"first_played_at"`
	LastPlayedAt  *int64   `json:"last_played_at"`
	RotationAt    *int64   `json:"rotation_at"`
}

type LibraryTrackCounts struct {
	All         int `json:"all"`
	Played      int `json:"played"`
	MaybePlayed int `json:"maybe_played"`
	NeverPlayed int `json:"never_played"`
}

type LibraryTracks struct {
	Counts LibraryTrackCounts `json:"counts"`
	Total  int                `json:"total"`
	Offset int                `json:"offset"`
	Limit  int                `json:"limit"`
	Items  []LibraryTrack     `json:"items"`
}

type LibraryAlbum struct {
	Album        AlbumRef `json:"album"`
	AddedAt      *int64   `json:"added_at"`
	TracksTotal  *int     `json:"tracks_total"`
	TracksPlayed int      `json:"tracks_played"`
	Plays        int64    `json:"plays"`
	MsPlayed     int64    `json:"ms_played"`
}

type LibraryAlbums struct {
	Total  int            `json:"total"`
	Offset int            `json:"offset"`
	Limit  int            `json:"limit"`
	Items  []LibraryAlbum `json:"items"`
}

type LibraryPlaylistRef struct {
	ID           string  `json:"id"`
	URI          string  `json:"uri"`
	Name         string  `json:"name"`
	OwnerName    *string `json:"owner_name"`
	Relation     string  `json:"relation"`
	ImageURL     *string `json:"image_url"`
	SpotifyOwned bool    `json:"spotify_owned"`
}

type LibraryPlaylist struct {
	Playlist         LibraryPlaylistRef `json:"playlist"`
	ItemsTotal       *int               `json:"items_total"`
	ItemsAvailable   bool               `json:"items_available"`
	Tracks           *int               `json:"tracks"`
	TracksPlayed     *int               `json:"tracks_played"`
	PlaysHere        int64              `json:"plays_here"`
	MsHere           int64              `json:"ms_here"`
	LastPlayedHereAt *int64             `json:"last_played_here_at"`
}

type LibraryPlaylists struct {
	Total  int               `json:"total"`
	Offset int               `json:"offset"`
	Limit  int               `json:"limit"`
	Items  []LibraryPlaylist `json:"items"`
}

type LibraryPlaylistItem struct {
	Position     int      `json:"position"`
	Track        TrackRef `json:"track"`
	AddedAt      *int64   `json:"added_at"`
	AddedBy      *string  `json:"added_by"`
	AddedByOwner bool     `json:"added_by_owner"`
	Status       string   `json:"status"`
	Plays        int64    `json:"plays"`
	PlaysHere    int64    `json:"plays_here"`
}

type LibraryPlaylistDetail struct {
	Playlist LibraryPlaylist       `json:"playlist"`
	Total    int                   `json:"total"`
	Offset   int                   `json:"offset"`
	Limit    int                   `json:"limit"`
	Items    []LibraryPlaylistItem `json:"items"`
}

type LibraryTimelinePoint struct {
	Key              string `json:"key"`
	StartAt          int64  `json:"start_at"`
	TracksSaved      int    `json:"tracks_saved"`
	TracksRemoved    int    `json:"tracks_removed"`
	AlbumsSaved      int    `json:"albums_saved"`
	AlbumsRemoved    int    `json:"albums_removed"`
	PlaylistAdds     int    `json:"playlist_adds"`
	PlaylistRemovals int    `json:"playlist_removals"`
}

type LibraryTimeline struct {
	Range  ResolvedRange          `json:"range"`
	Bucket string                 `json:"bucket"`
	Points []LibraryTimelinePoint `json:"points"`
}

func (p *params) oneOf(name, def string, allowed ...string) string {
	v := p.q.Get(name)
	if v == "" {
		return def
	}
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	p.fail("%s must be one of %v", name, allowed)
	return def
}

func registerLibrary(mux *http.ServeMux, b Backend) {
	get(mux, v1+"/library/summary", func(r *http.Request, _ *params) (*LibrarySummary, error) {
		return b.LibrarySummary(r.Context())
	})
	get(mux, v1+"/library/tracks", func(r *http.Request, p *params) (*LibraryTracks, error) {
		lp := LibraryTracksParams{Played: p.oneOf("played", "all", "all", StatusPlayed, StatusMaybePlayed, StatusNeverPlayed),
			Sort: p.oneOf("sort", "added", "added", "plays", "last_played"), Page: p.page()}
		if p.err != nil {
			return nil, nil
		}
		return b.LibraryTracks(r.Context(), lp)
	})
	get(mux, v1+"/library/albums", func(r *http.Request, p *params) (*LibraryAlbums, error) {
		lp := LibraryAlbumsParams{Sort: p.oneOf("sort", "added", "added", "plays", "played_share"), Page: p.page()}
		if p.err != nil {
			return nil, nil
		}
		return b.LibraryAlbums(r.Context(), lp)
	})
	get(mux, v1+"/library/playlists", func(r *http.Request, p *params) (*LibraryPlaylists, error) {
		lp := LibraryPlaylistsParams{Relation: p.oneOf("relation", "all", "all", "own", "collaborative", "followed"),
			Sort: p.oneOf("sort", "plays_here", "plays_here", "played_share", "name"), Page: p.page()}
		if p.err != nil {
			return nil, nil
		}
		return b.LibraryPlaylists(r.Context(), lp)
	})
	get(mux, v1+"/library/playlists/{id}", func(r *http.Request, p *params) (*LibraryPlaylistDetail, error) {
		pg := p.page()
		if p.err != nil {
			return nil, nil
		}
		return b.LibraryPlaylist(r.Context(), r.PathValue("id"), pg)
	})
	get(mux, v1+"/library/timeline", func(r *http.Request, p *params) (*LibraryTimeline, error) {
		s, bk := p.spec(query.All), p.bucket()
		if p.err != nil {
			return nil, nil
		}
		return b.LibraryTimeline(r.Context(), s, bk)
	})
	mux.HandleFunc("POST "+v1+"/library/sync", func(w http.ResponseWriter, r *http.Request) {
		st, err := b.SyncLibrary(r.Context())
		switch {
		case errors.Is(err, ErrSyncRunning):
			writeError(w, r, http.StatusConflict, "sync_running", err.Error())
		case errors.Is(err, ErrSyncDisabled):
			writeError(w, r, http.StatusServiceUnavailable, "sync_disabled", err.Error())
		case err != nil:
			writeResult[LibraryStatus](w, r, nil, err)
		default:
			writeJSON(w, r, http.StatusAccepted, st)
		}
	})
}
