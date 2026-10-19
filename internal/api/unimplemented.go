package api

import (
	"context"

	"spotussi/internal/query"
)

// UnimplementedBackend answers every Backend method with
// ErrNotImplemented (501). Partial backends embed it and override what
// they support, so the live backend can grow endpoint by endpoint until
// the M6 query layer implements the whole contract.
type UnimplementedBackend struct{}

var _ Backend = UnimplementedBackend{}

func (UnimplementedBackend) SystemStatus(context.Context) (*SystemStatus, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) NowPlaying(context.Context) (*NowPlaying, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) OverviewSummary(context.Context, query.Spec, query.Metric) (*OverviewSummary, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Timeline(context.Context, query.Spec, query.Bucket, Scope, bool) (*Timeline, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Clock(context.Context, query.Spec, Scope) (*Clock, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) TopTracks(context.Context, TopParams) (*TopTracks, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) TopArtists(context.Context, TopParams) (*TopArtists, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) TopAlbums(context.Context, TopParams) (*TopAlbums, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) TopGenres(context.Context, TopParams) (*TopGenres, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) HourlyTop(context.Context, query.Spec, query.Metric) (*HourlyTop, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) ArtistsOverTime(context.Context, SeriesParams) (*ArtistsOverTime, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) GenresOverTime(context.Context, SeriesParams) (*GenresOverTime, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) ReleaseYears(context.Context, SeriesParams) (*AverageSeries, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) ArtistsPerTrack(context.Context, SeriesParams) (*AverageSeries, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Artist(context.Context, string, query.Spec, query.Metric) (*ArtistDetail, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Album(context.Context, string, query.Spec, query.Metric) (*AlbumDetail, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Track(context.Context, string, query.Spec, query.Metric) (*TrackDetail, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Genre(context.Context, string, query.Spec, query.Metric) (*GenreDetail, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Contexts(context.Context, TopParams) (*Contexts, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) SetPlaylistName(context.Context, string, *string) (*ContextRef, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) History(context.Context, HistoryParams) (*History, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Devices(context.Context, query.Spec) (*Devices, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Sessions(context.Context, query.Spec, Page) (*Sessions, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Session(context.Context, string) (*SessionDetail, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Search(context.Context, string, int) (*SearchResults, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Settings(context.Context) (*Settings, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) UpdateSettings(context.Context, SettingsPatch) (*Settings, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Exclusions(context.Context) (*Exclusions, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) AddExclusion(context.Context, ExclusionKind, string) (*Exclusions, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) RemoveExclusion(context.Context, ExclusionKind, string) (*Exclusions, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) CreateImport(context.Context, ImportUpload) (*ImportJob, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Imports(context.Context) (*ImportJobList, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) Import(context.Context, string) (*ImportJob, error) {
	return nil, ErrNotImplemented
}

func (UnimplementedBackend) LibrarySummary(context.Context) (*LibrarySummary, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) LibraryTracks(context.Context, LibraryTracksParams) (*LibraryTracks, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) LibraryAlbums(context.Context, LibraryAlbumsParams) (*LibraryAlbums, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) LibraryPlaylists(context.Context, LibraryPlaylistsParams) (*LibraryPlaylists, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) LibraryPlaylist(context.Context, string, Page) (*LibraryPlaylistDetail, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) LibraryTimeline(context.Context, query.Spec, query.Bucket) (*LibraryTimeline, error) {
	return nil, ErrNotImplemented
}
func (UnimplementedBackend) SyncLibrary(context.Context) (*LibraryStatus, error) {
	return nil, ErrNotImplemented
}
