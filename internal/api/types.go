package api

import "encoding/json"

// Wire types for /api/v1. They mirror api/openapi.yaml one to one; the
// contract tests validate real responses against the spec, so drift between
// this file and the spec fails `go test`. Pointer fields are nullable.

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ResolvedRange struct {
	Preset   string `json:"preset"`
	From     string `json:"from"`
	To       string `json:"to"`
	StartAt  int64  `json:"start_at"`
	EndAt    int64  `json:"end_at"`
	Timezone string `json:"timezone"`
	// FirstPlayDate is the local date of the earliest play overall (null
	// when there are no plays).
	FirstPlayDate *string `json:"first_play_date"`
}

type Measure struct {
	Plays    int64 `json:"plays"`
	MsPlayed int64 `json:"ms_played"`
}

// EntityMetadata says where an entity's details come from (M7, D27).
type EntityMetadata struct {
	// State: spotify, fallback or pending.
	State string `json:"state"`
	// Sources: the fallback sources that filled fields (oembed, deezer,
	// listenbrainz, musicbrainz), in stage order; empty unless State is
	// fallback.
	Sources []string `json:"sources"`
}

// Metadata states.
const (
	MetaSpotify  = "spotify"
	MetaFallback = "fallback"
	MetaPending  = "pending"
)

// SourceMix splits a range's listening by where the plays came from (D8).
type SourceMix struct {
	Export Measure `json:"export"`
	Live   Measure `json:"live"`
	Both   Measure `json:"both"`
}

// CatalogMix splits a range's listening by where its tracks' details
// come from.
type CatalogMix struct {
	Spotify  Measure `json:"spotify"`
	Fallback Measure `json:"fallback"`
	Pending  Measure `json:"pending"`
}

// ---------- entity references ----------

type ArtistLite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AlbumLite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ArtistRef struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ImageURL *string `json:"image_url"`
}

type AlbumRef struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	ImageURL    *string      `json:"image_url"`
	ReleaseYear *int         `json:"release_year"`
	Artists     []ArtistLite `json:"artists"`
}

type TrackRef struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	ImageURL   *string      `json:"image_url"`
	DurationMs *int64       `json:"duration_ms"`
	Artists    []ArtistLite `json:"artists"`
	Album      *AlbumLite   `json:"album"`
}

type GenreRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ContextRef struct {
	URI          string  `json:"uri"`
	Type         string  `json:"type"`
	ID           string  `json:"id"`
	Name         *string `json:"name"`
	NameSource   string  `json:"name_source"`
	SpotifyOwned bool    `json:"spotify_owned"`
	ImageURL     *string `json:"image_url"`
}

// ---------- system ----------

type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Mode    string `json:"mode"`
}

type SystemStatus struct {
	Spotify SpotifyStatus `json:"spotify"`
	Capture CaptureStatus `json:"capture"`
	Imports []ImportJob   `json:"imports"`
	Catalog CatalogStatus `json:"catalog"`
	Derive  DeriveStatus  `json:"derive"`
	Data    DataStatus    `json:"data"`
	Auth    AuthStatus    `json:"auth"`
}

// AuthStatus reports the instance access mode (D13); the UI shows a
// persistent warning for "none".
type AuthStatus struct {
	Mode string `json:"mode"`
}

type SpotifyStatus struct {
	State       string  `json:"state"`
	AccountName *string `json:"account_name"`
	ConnectedAt *int64  `json:"connected_at"`
}

type CaptureStatus struct {
	State      string `json:"state"`
	LastPollAt *int64 `json:"last_poll_at"`
	LastPlayAt *int64 `json:"last_play_at"`
	// Live capture diagnostics (M2); null in mock mode and while capture
	// is not running.
	PollIntervalMs *int64              `json:"poll_interval_ms"`
	NextPollAt     *int64              `json:"next_poll_at"`
	LastError      *string             `json:"last_error"`
	LastErrorAt    *int64              `json:"last_error_at"`
	Backstop       *CaptureBackstop    `json:"backstop"`
	RateLimit      *RateLimitStatus    `json:"rate_limit"`
	Calibration    *CaptureCalibration `json:"calibration"`
}

// CaptureBackstop is the recently-played backstop's state.
type CaptureBackstop struct {
	LastRunAt   *int64  `json:"last_run_at"`
	LastError   *string `json:"last_error"`
	ItemsStored int64   `json:"items_stored"`
}

// RateLimitStatus is the shared Spotify rate limiter (D9).
type RateLimitStatus struct {
	RatePerSec          float64          `json:"rate_per_sec"`
	EffectiveRatePerSec float64          `json:"effective_rate_per_sec"`
	Burst               int              `json:"burst"`
	LiveReserve         int              `json:"live_reserve"`
	BlockedUntil        *int64           `json:"blocked_until"`
	Throttled           int64            `json:"throttled"`
	QuotaExceeded       int64            `json:"quota_exceeded"`
	Last429At           *int64           `json:"last_429_at"`
	Requests            map[string]int64 `json:"requests"`
	QuotaBlocked        map[string]int64 `json:"quota_blocked"`
}

// CaptureCalibration summarizes the played_at calibration spike.
type CaptureCalibration struct {
	Samples        int           `json:"samples"`
	Discriminating int           `json:"discriminating"`
	Verdict        string        `json:"verdict"`
	StartOffset    OffsetSummary `json:"start_offset"`
	EndOffset      OffsetSummary `json:"end_offset"`
	UpdatedAt      *int64        `json:"updated_at"`
}

// OffsetSummary is played_at minus a poller timestamp.
type OffsetSummary struct {
	MedianMs int64   `json:"median_ms"`
	P10Ms    int64   `json:"p10_ms"`
	P90Ms    int64   `json:"p90_ms"`
	HitRate  float64 `json:"hit_rate"`
}

type ImportJob struct {
	ID            string  `json:"id"`
	Filename      string  `json:"filename"`
	Origin        string  `json:"origin"`
	State         string  `json:"state"`
	Files         int     `json:"files"`
	FilesDone     int     `json:"files_done"`
	BytesTotal    *int64  `json:"bytes_total"`
	BytesDone     int64   `json:"bytes_done"`
	RowsTotal     *int64  `json:"rows_total"`
	RowsDone      int64   `json:"rows_done"`
	RowsNew       int64   `json:"rows_new"`
	RowsDuplicate int64   `json:"rows_duplicate"`
	StartedAt     int64   `json:"started_at"`
	FinishedAt    *int64  `json:"finished_at"`
	ErrorCode     *string `json:"error_code"`
	Error         *string `json:"error"`
}

type ImportJobList struct {
	Items []ImportJob `json:"items"`
}

type CatalogStatus struct {
	State            string             `json:"state"`
	TracksTotal      int64              `json:"tracks_total"`
	TracksHydrated   int64              `json:"tracks_hydrated"`
	AlbumsTotal      int64              `json:"albums_total"`
	AlbumsHydrated   int64              `json:"albums_hydrated"`
	ArtistsTotal     int64              `json:"artists_total"`
	ArtistsHydrated  int64              `json:"artists_hydrated"`
	ContextsTotal    int64              `json:"contexts_total"`
	ContextsResolved int64              `json:"contexts_resolved"`
	QueueLength      int64              `json:"queue_length"`
	Queue            CatalogQueueCounts `json:"queue"`
	QueueFailed      int64              `json:"queue_failed"`
	RatePerSec       *float64           `json:"rate_per_sec"`
	EtaAt            *int64             `json:"eta_at"`
	ResumesAt        *int64             `json:"resumes_at"`
	QuotaLookups     *int64             `json:"quota_lookups"`
	LastError        *string            `json:"last_error"`
	LastErrorAt      *int64             `json:"last_error_at"`
	LastHydratedAt   *int64             `json:"last_hydrated_at"`
	Fallbacks        []CatalogFallback  `json:"fallbacks"`
}

// CatalogFallback is one fallback metadata source's progress (D27).
type CatalogFallback struct {
	Source         string   `json:"source"`
	State          string   `json:"state"`
	Matched        int64    `json:"matched"`
	Unmatched      int64    `json:"unmatched"`
	Pending        int64    `json:"pending"`
	Failed         int64    `json:"failed"`
	ArtistsMatched int64    `json:"artists_matched"`
	AlbumsMatched  int64    `json:"albums_matched"`
	Requests       int64    `json:"requests"`
	RatePerSec     *float64 `json:"rate_per_sec"`
	PausedUntil    *int64   `json:"paused_until"`
	LastError      *string  `json:"last_error"`
	LastErrorAt    *int64   `json:"last_error_at"`
}

type CatalogQueueCounts struct {
	Track   int64 `json:"track"`
	Album   int64 `json:"album"`
	Artist  int64 `json:"artist"`
	Context int64 `json:"context"`
}

type DeriveStatus struct {
	State         string `json:"state"`
	LastRebuildAt *int64 `json:"last_rebuild_at"`
	LastRebuildMs *int64 `json:"last_rebuild_ms"`
	// Progress (0–1) is set only while rebuilding.
	Progress *float64 `json:"progress,omitempty"`
}

type DataStatus struct {
	Rows        int64  `json:"rows"`
	Plays       int64  `json:"plays"`
	FirstPlayAt *int64 `json:"first_play_at"`
	LastPlayAt  *int64 `json:"last_play_at"`
}

// ---------- overview ----------

type Summary struct {
	Plays       int64         `json:"plays"`
	MsPlayed    int64         `json:"ms_played"`
	MsEstimated int64         `json:"ms_estimated"`
	Skips       int64         `json:"skips"`
	Tracks      int64         `json:"tracks"`
	Artists     int64         `json:"artists"`
	Albums      int64         `json:"albums"`
	TopArtist   *RankedArtist `json:"top_artist"`
	TopTrack    *RankedTrack  `json:"top_track"`
}

type OverviewSummary struct {
	Range           ResolvedRange  `json:"range"`
	Previous        *ResolvedRange `json:"previous"`
	Metric          string         `json:"metric"`
	Current         Summary        `json:"current"`
	PreviousSummary *Summary       `json:"previous_summary"`
	Sources         SourceMix      `json:"sources"`
	Catalog         CatalogMix     `json:"catalog"`
}

type TimelinePoint struct {
	Key         string `json:"key"`
	StartAt     int64  `json:"start_at"`
	Plays       int64  `json:"plays"`
	MsPlayed    int64  `json:"ms_played"`
	MsEstimated int64  `json:"ms_estimated"`
	Skips       int64  `json:"skips"`
	Artists     int64  `json:"artists"`
}

type Timeline struct {
	Range  ResolvedRange   `json:"range"`
	Bucket string          `json:"bucket"`
	Points []TimelinePoint `json:"points"`
	// Only with compare=true: Compared marks the fields as present.
	Compared       bool            `json:"-"`
	Previous       *ResolvedRange  `json:"-"`
	PreviousPoints []TimelinePoint `json:"-"`
}

// MarshalJSON emits previous/previous_points only for compare=true
// (previous is then present even when null).
func (t Timeline) MarshalJSON() ([]byte, error) {
	type base struct {
		Range  ResolvedRange   `json:"range"`
		Bucket string          `json:"bucket"`
		Points []TimelinePoint `json:"points"`
	}
	b := base{t.Range, t.Bucket, t.Points}
	if !t.Compared {
		return json.Marshal(b)
	}
	pts := t.PreviousPoints
	if pts == nil {
		pts = []TimelinePoint{}
	}
	return json.Marshal(struct {
		base
		Previous       *ResolvedRange  `json:"previous"`
		PreviousPoints []TimelinePoint `json:"previous_points"`
	}{b, t.Previous, pts})
}

type ClockHour struct {
	Hour     int   `json:"hour"`
	Plays    int64 `json:"plays"`
	MsPlayed int64 `json:"ms_played"`
}

type Clock struct {
	Range ResolvedRange `json:"range"`
	Hours []ClockHour   `json:"hours"`
}

type NowPlaying struct {
	IsPlaying  bool        `json:"is_playing"`
	Track      *TrackRef   `json:"track"`
	ProgressMs *int64      `json:"progress_ms"`
	StartedAt  *int64      `json:"started_at"`
	Device     *Device     `json:"device"`
	Context    *ContextRef `json:"context"`
	Shuffle    *bool       `json:"shuffle"`
	ObservedAt *int64      `json:"observed_at"`
}

// ---------- top lists ----------

type RankedTrack struct {
	Rank     int      `json:"rank"`
	PrevRank *int     `json:"prev_rank"`
	Plays    int64    `json:"plays"`
	MsPlayed int64    `json:"ms_played"`
	Track    TrackRef `json:"track"`
}

type RankedArtist struct {
	Rank     int       `json:"rank"`
	PrevRank *int      `json:"prev_rank"`
	Plays    int64     `json:"plays"`
	MsPlayed int64     `json:"ms_played"`
	Artist   ArtistRef `json:"artist"`
}

type RankedAlbum struct {
	Rank     int      `json:"rank"`
	PrevRank *int     `json:"prev_rank"`
	Plays    int64    `json:"plays"`
	MsPlayed int64    `json:"ms_played"`
	Album    AlbumRef `json:"album"`
}

type RankedGenre struct {
	Rank     int      `json:"rank"`
	PrevRank *int     `json:"prev_rank"`
	Plays    int64    `json:"plays"`
	MsPlayed int64    `json:"ms_played"`
	Genre    GenreRef `json:"genre"`
}

// TopList is the shared envelope of the /top/* endpoints.
type TopList[T any] struct {
	Range    ResolvedRange  `json:"range"`
	Previous *ResolvedRange `json:"previous"`
	Metric   string         `json:"metric"`
	Total    int            `json:"total"`
	Offset   int            `json:"offset"`
	Limit    int            `json:"limit"`
	Items    []T            `json:"items"`
}

type (
	TopTracks  = TopList[RankedTrack]
	TopArtists = TopList[RankedArtist]
	TopAlbums  = TopList[RankedAlbum]
	TopGenres  = TopList[RankedGenre]
)

// ---------- stats ----------

type HourTopArtist struct {
	Plays    int64     `json:"plays"`
	MsPlayed int64     `json:"ms_played"`
	Artist   ArtistRef `json:"artist"`
}

type HourTopTrack struct {
	Plays    int64    `json:"plays"`
	MsPlayed int64    `json:"ms_played"`
	Track    TrackRef `json:"track"`
}

type HourTop struct {
	Hour      int            `json:"hour"`
	Plays     int64          `json:"plays"`
	MsPlayed  int64          `json:"ms_played"`
	TopArtist *HourTopArtist `json:"top_artist"`
	TopTrack  *HourTopTrack  `json:"top_track"`
}

type HourlyTop struct {
	Range  ResolvedRange `json:"range"`
	Metric string        `json:"metric"`
	Hours  []HourTop     `json:"hours"`
}

type BreakdownPoint struct {
	Key     string  `json:"key"`
	StartAt int64   `json:"start_at"`
	Total   int64   `json:"total"`
	Values  []int64 `json:"values"`
	Other   int64   `json:"other"`
}

type Breakdown[T any] struct {
	Range  ResolvedRange    `json:"range"`
	Metric string           `json:"metric"`
	Bucket string           `json:"bucket"`
	Series []T              `json:"series"`
	Points []BreakdownPoint `json:"points"`
}

type (
	ArtistsOverTime = Breakdown[ArtistRef]
	GenresOverTime  = Breakdown[GenreRef]
)

type AveragePoint struct {
	Key      string   `json:"key"`
	StartAt  int64    `json:"start_at"`
	Value    *float64 `json:"value"`
	Coverage float64  `json:"coverage"`
}

type AverageSeries struct {
	Range   ResolvedRange  `json:"range"`
	Metric  string         `json:"metric"`
	Bucket  string         `json:"bucket"`
	Overall *float64       `json:"overall"`
	Points  []AveragePoint `json:"points"`
}

// ---------- entity pages ----------

type EntityStats struct {
	Rank        *int  `json:"rank"`
	RankedTotal int   `json:"ranked_total"`
	Plays       int64 `json:"plays"`
	MsPlayed    int64 `json:"ms_played"`
	MsEstimated int64 `json:"ms_estimated"`
	Skips       int64 `json:"skips"`
}

type AllTimeStats struct {
	Plays         int64  `json:"plays"`
	MsPlayed      int64  `json:"ms_played"`
	Skips         int64  `json:"skips"`
	FirstPlayedAt *int64 `json:"first_played_at"`
	LastPlayedAt  *int64 `json:"last_played_at"`
}

type ArtistFull struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	ImageURL *string        `json:"image_url"`
	Genres   []GenreRef     `json:"genres"`
	Metadata EntityMetadata `json:"metadata"`
}

type AlbumFull struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	ImageURL    *string        `json:"image_url"`
	AlbumType   *string        `json:"album_type"`
	ReleaseDate *string        `json:"release_date"`
	ReleaseYear *int           `json:"release_year"`
	TotalTracks *int           `json:"total_tracks"`
	Artists     []ArtistLite   `json:"artists"`
	Metadata    EntityMetadata `json:"metadata"`
}

type TrackFull struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	ImageURL   *string        `json:"image_url"`
	DurationMs *int64         `json:"duration_ms"`
	Explicit   *bool          `json:"explicit"`
	Artists    []ArtistRef    `json:"artists"`
	Album      *AlbumRef      `json:"album"`
	Metadata   EntityMetadata `json:"metadata"`
}

type ArtistDetail struct {
	Artist   ArtistFull     `json:"artist"`
	Excluded bool           `json:"excluded"`
	Range    ResolvedRange  `json:"range"`
	Previous *ResolvedRange `json:"previous"`
	Metric   string         `json:"metric"`
	InRange  EntityStats    `json:"in_range"`
	AllTime  AllTimeStats   `json:"all_time"`
}

type AlbumDetail struct {
	Album    AlbumFull      `json:"album"`
	Range    ResolvedRange  `json:"range"`
	Previous *ResolvedRange `json:"previous"`
	Metric   string         `json:"metric"`
	InRange  EntityStats    `json:"in_range"`
	AllTime  AllTimeStats   `json:"all_time"`
}

type TrackDetail struct {
	Track    TrackFull      `json:"track"`
	Excluded bool           `json:"excluded"`
	Range    ResolvedRange  `json:"range"`
	Previous *ResolvedRange `json:"previous"`
	Metric   string         `json:"metric"`
	InRange  EntityStats    `json:"in_range"`
	AllTime  AllTimeStats   `json:"all_time"`
}

type GenreDetail struct {
	Genre    GenreRef       `json:"genre"`
	Artists  int            `json:"artists"`
	Range    ResolvedRange  `json:"range"`
	Previous *ResolvedRange `json:"previous"`
	Metric   string         `json:"metric"`
	InRange  EntityStats    `json:"in_range"`
	AllTime  AllTimeStats   `json:"all_time"`
}

type RankedContext struct {
	Rank     int        `json:"rank"`
	Plays    int64      `json:"plays"`
	MsPlayed int64      `json:"ms_played"`
	Context  ContextRef `json:"context"`
}

type Contexts struct {
	Range   ResolvedRange   `json:"range"`
	Metric  string          `json:"metric"`
	Total   int             `json:"total"`
	Offset  int             `json:"offset"`
	Limit   int             `json:"limit"`
	Items   []RankedContext `json:"items"`
	Unknown Measure         `json:"unknown"`
}

type PlaylistNameInput struct {
	Name *string `json:"name"`
}

// ---------- history ----------

type Device struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Play struct {
	ID           string      `json:"id"`
	PlayedAt     int64       `json:"played_at"`
	EndedAt      int64       `json:"ended_at"`
	MsPlayed     int64       `json:"ms_played"`
	MsEstimated  bool        `json:"ms_estimated"`
	CountsAsPlay bool        `json:"counts_as_play"`
	Skipped      bool        `json:"skipped"`
	Shuffle      *bool       `json:"shuffle"`
	Platform     *string     `json:"platform"`
	Device       *Device     `json:"device"`
	Context      *ContextRef `json:"context"`
	Sources      []string    `json:"sources"`
	Contradicted bool        `json:"contradicted"`
	Track        TrackRef    `json:"track"`
}

type History struct {
	Range      ResolvedRange `json:"range"`
	Total      int           `json:"total"`
	Items      []Play        `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

type DeviceUsage struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Plays    int64  `json:"plays"`
	MsPlayed int64  `json:"ms_played"`
}

type PlatformUsage struct {
	Name     string `json:"name"`
	Plays    int64  `json:"plays"`
	MsPlayed int64  `json:"ms_played"`
}

type Devices struct {
	Range     ResolvedRange   `json:"range"`
	Devices   []DeviceUsage   `json:"devices"`
	Platforms []PlatformUsage `json:"platforms"`
}

// ---------- sessions ----------

type Session struct {
	ID         string      `json:"id"`
	StartedAt  int64       `json:"started_at"`
	EndedAt    int64       `json:"ended_at"`
	DurationMs int64       `json:"duration_ms"`
	MsPlayed   int64       `json:"ms_played"`
	Rows       int64       `json:"rows"`
	Plays      int64       `json:"plays"`
	Skips      int64       `json:"skips"`
	TopArtists []ArtistRef `json:"top_artists"`
}

type Sessions struct {
	Range      ResolvedRange `json:"range"`
	GapMinutes int           `json:"gap_minutes"`
	Total      int           `json:"total"`
	Offset     int           `json:"offset"`
	Limit      int           `json:"limit"`
	Items      []Session     `json:"items"`
}

type SessionDetail struct {
	Session Session `json:"session"`
	Plays   []Play  `json:"plays"`
}

// ---------- search ----------

type SearchResults struct {
	Artists []ArtistRef `json:"artists"`
	Albums  []AlbumRef  `json:"albums"`
	Tracks  []TrackRef  `json:"tracks"`
}

// ---------- settings ----------

type Settings struct {
	PlayRule            string `json:"play_rule"`
	PlayRuleThresholdMs int64  `json:"play_rule_threshold_ms"`
	Timezone            string `json:"timezone"`
	WeekStart           string `json:"week_start"`
	DateFormat          string `json:"date_format"`
	SessionGapMinutes   int    `json:"session_gap_minutes"`
}

type SettingsPatch struct {
	PlayRule            *string `json:"play_rule,omitempty"`
	PlayRuleThresholdMs *int64  `json:"play_rule_threshold_ms,omitempty"`
	Timezone            *string `json:"timezone,omitempty"`
	WeekStart           *string `json:"week_start,omitempty"`
	DateFormat          *string `json:"date_format,omitempty"`
	SessionGapMinutes   *int    `json:"session_gap_minutes,omitempty"`
}

type Exclusions struct {
	Artists []ArtistRef `json:"artists"`
	Tracks  []TrackRef  `json:"tracks"`
}
