package api

import (
	"context"
	"errors"

	"spotussi/internal/query"
)

// Sentinel errors a Backend returns; the handlers map them to HTTP status
// codes. Errors wrapping query.ErrInvalid also map to 400.
var (
	ErrNotFound   = errors.New("not found")
	ErrBadRequest = errors.New("bad request")
	// ErrNotImplemented maps to 501: the endpoint exists in the contract
	// but this backend cannot answer it yet.
	ErrNotImplemented = errors.New("this endpoint is not implemented yet; run `spotussi serve --mock` for the synthetic dataset")
)

// Scope restricts a query to plays of one entity (at most one field set).
type Scope struct {
	ArtistID, AlbumID, TrackID, GenreID string
}

// IsZero reports whether no scope is set.
func (s Scope) IsZero() bool {
	return s.ArtistID == "" && s.AlbumID == "" && s.TrackID == "" && s.GenreID == ""
}

// Page is offset pagination.
type Page struct {
	Limit, Offset int
}

// TopParams are the parameters of ranked lists (/top/*, /contexts).
type TopParams struct {
	Range  query.Spec
	Metric query.Metric
	Page   Page
	Scope  Scope
}

// SeriesParams are the parameters of /stats/* time series.
type SeriesParams struct {
	Range  query.Spec
	Metric query.Metric
	Bucket query.Bucket
	Series int // number of named series (breakdowns only)
}

// HistoryParams are the History filters.
type HistoryParams struct {
	Range                     query.Spec
	Scope                     Scope
	Skipped, Shuffle, Counted *bool
	Device, Platform          string
	Cursor                    string
	Limit                     int
}

// ImportUpload is an uploaded export, already streamed to disk.
type ImportUpload struct {
	Path     string
	Filename string
	Kind     string // zip or json
	Size     int64
}

// ExclusionKind is "artists" or "tracks".
type ExclusionKind string

const (
	ExcludeArtists ExclusionKind = "artists"
	ExcludeTracks  ExclusionKind = "tracks"
)

// Backend answers every /api/v1 data endpoint. The mock (synthetic
// dataset, naive aggregation) implements it in M0; the rollup-backed query
// layer implements it in M6. Handlers only parse and validate parameters.
type Backend interface {
	SystemStatus(ctx context.Context) (*SystemStatus, error)
	NowPlaying(ctx context.Context) (*NowPlaying, error)

	OverviewSummary(ctx context.Context, r query.Spec, m query.Metric) (*OverviewSummary, error)
	// Timeline adds the aligned previous-period series when compare is set.
	Timeline(ctx context.Context, r query.Spec, b query.Bucket, s Scope, compare bool) (*Timeline, error)
	Clock(ctx context.Context, r query.Spec, s Scope) (*Clock, error)

	TopTracks(ctx context.Context, p TopParams) (*TopTracks, error)
	TopArtists(ctx context.Context, p TopParams) (*TopArtists, error)
	TopAlbums(ctx context.Context, p TopParams) (*TopAlbums, error)
	TopGenres(ctx context.Context, p TopParams) (*TopGenres, error)

	HourlyTop(ctx context.Context, r query.Spec, m query.Metric) (*HourlyTop, error)
	ArtistsOverTime(ctx context.Context, p SeriesParams) (*ArtistsOverTime, error)
	GenresOverTime(ctx context.Context, p SeriesParams) (*GenresOverTime, error)
	ReleaseYears(ctx context.Context, p SeriesParams) (*AverageSeries, error)
	ArtistsPerTrack(ctx context.Context, p SeriesParams) (*AverageSeries, error)

	Artist(ctx context.Context, id string, r query.Spec, m query.Metric) (*ArtistDetail, error)
	Album(ctx context.Context, id string, r query.Spec, m query.Metric) (*AlbumDetail, error)
	Track(ctx context.Context, id string, r query.Spec, m query.Metric) (*TrackDetail, error)
	Genre(ctx context.Context, id string, r query.Spec, m query.Metric) (*GenreDetail, error)
	Contexts(ctx context.Context, p TopParams) (*Contexts, error)
	SetPlaylistName(ctx context.Context, id string, name *string) (*ContextRef, error)

	History(ctx context.Context, p HistoryParams) (*History, error)
	Devices(ctx context.Context, r query.Spec) (*Devices, error)
	Sessions(ctx context.Context, r query.Spec, p Page) (*Sessions, error)
	Session(ctx context.Context, id string) (*SessionDetail, error)
	Search(ctx context.Context, q string, limit int) (*SearchResults, error)

	// CreateImport queues an import of an uploaded export. The backend
	// owns up.Path from then on (and removes it when done).
	CreateImport(ctx context.Context, up ImportUpload) (*ImportJob, error)
	Imports(ctx context.Context) (*ImportJobList, error)
	Import(ctx context.Context, id string) (*ImportJob, error)

	Settings(ctx context.Context) (*Settings, error)
	UpdateSettings(ctx context.Context, p SettingsPatch) (*Settings, error)
	Exclusions(ctx context.Context) (*Exclusions, error)
	AddExclusion(ctx context.Context, kind ExclusionKind, id string) (*Exclusions, error)
	RemoveExclusion(ctx context.Context, kind ExclusionKind, id string) (*Exclusions, error)
}
