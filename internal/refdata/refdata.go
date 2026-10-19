// Package refdata generates the synthetic reference dataset: a seeded,
// deterministic listening history with realistic structure, used for UI
// fixtures (mock mode), contract tests and, from M5, performance benchmarks
// and as input to the correctness oracle.
//
// The generated history is already "derived" (one row per listening event
// with merged provenance flags, as plays would look after D8 merging), but
// it does not decide which rows count as plays: that is the play rule's job
// (internal/derive), applied by consumers.
//
// Same Config (including End) ⇒ byte-identical dataset.
package refdata

import (
	"fmt"
	"time"
)

// Size selects a dataset scale.
type Size string

const (
	// Full matches the plan's reference size: 15 years, ~500k rows,
	// ~40k tracks, ~10k artists.
	Full  Size = "full"
	Small Size = "small" // 5 years, ~50k rows: quick local iteration
	Tiny  Size = "tiny"  // 1 year, ~5k rows: unit tests
)

type sizeParams struct {
	years, rows, tracks, artists, genres, playlists int
}

var sizes = map[Size]sizeParams{
	Full:  {years: 15, rows: 500_000, tracks: 40_000, artists: 10_000, genres: 400, playlists: 120},
	Small: {years: 5, rows: 50_000, tracks: 6_000, artists: 1_500, genres: 150, playlists: 40},
	Tiny:  {years: 1, rows: 5_000, tracks: 800, artists: 200, genres: 60, playlists: 12},
}

// ParseSize validates a size name.
func ParseSize(s string) (Size, error) {
	if _, ok := sizes[Size(s)]; ok {
		return Size(s), nil
	}
	return "", fmt.Errorf("unknown dataset size %q (want full, small or tiny)", s)
}

// Config parameterises generation.
type Config struct {
	Size Size
	Seed uint64
	// End is the moment the history ends ("now" for mock mode; fixed in
	// tests so output is reproducible).
	End time.Time
	// Timezone is the listener's home timezone, which shapes daily rhythms.
	// Default Europe/London.
	Timezone string
}

// Play flags.
const (
	FlagSkipped      uint8 = 1 << iota
	FlagShuffle            // shuffle was on (only meaningful with FlagShuffleKnown)
	FlagShuffleKnown       // shuffle state is known
	FlagEstimated          // ms played is the track length, not measured (recently-played only, D7)
	FlagSrcExport          // confirmed by the Extended Streaming History export
	FlagSrcPoller          // captured by the live poller
	FlagSrcRecent          // seen in recently-played
	FlagContradicted       // poller-only play inside the export window (D8)
)

// Play is one listening row. 24 bytes; the full dataset is ~12 MB.
type Play struct {
	StartMs  int64
	MsPlayed int32
	Track    int32
	Context  int32 // index into Contexts, -1 if unknown
	Device   int8  // index into Devices, -1 if unknown (export-only rows)
	Platform int8  // index into Platforms, -1 if unknown (poller-only rows)
	Flags    uint8
}

// Has reports whether all bits of f are set.
func (p Play) Has(f uint8) bool { return p.Flags&f == f }

// EndMs is when the row ended (start + ms played; pauses are not modelled).
func (p Play) EndMs() int64 { return p.StartMs + int64(p.MsPlayed) }

type Genre struct {
	ID, Name string
	Family   int
}

type Artist struct {
	ID, Name string
	Genres   []int32
	Hydrated bool
}

type Album struct {
	ID, Name    string
	Type        string // album, single, compilation
	ReleaseDate string // YYYY or YYYY-MM-DD
	ReleaseYear int
	Artists     []int32
	Tracks      []int32 // played tracks, in track-number order
	TotalTracks int     // tracks on the release, played or not
}

type Track struct {
	ID, Name   string
	DurationMs int32
	Explicit   bool
	Album      int32
	Artists    []int32 // credit order; [0] is the primary artist
	// Hydrated is false for a small tail of tracks whose catalog metadata
	// (duration, artwork) is "not fetched yet" (D9). The play rule then
	// falls back to 30 s (D7).
	Hydrated bool
}

type Context struct {
	ID, URI string
	Type    string // playlist, album, artist, collection
	// Name is "" when unresolved (Spotify-owned playlists, D17).
	Name         string
	NameSource   string // api, catalog, manual, unresolved
	SpotifyOwned bool
	Ref          int32 // album or artist index for catalog contexts, else -1
}

type Device struct {
	Name, Type string
}

// NowPlaying describes what the synthetic listener is playing at End.
type NowPlaying struct {
	Track     int32
	Context   int32
	Device    int8
	StartedMs int64
	Shuffle   bool
}

// Dataset is a generated listening history plus its catalog.
type Dataset struct {
	Timezone string
	Start    time.Time // local midnight of the first day
	End      time.Time
	// PollerSince is when live capture started; earlier rows come from
	// the export only (no context, no device).
	PollerSince int64
	// ExportUntil is the end of the export's coverage; later rows come
	// from the poller and recently-played only.
	ExportUntil int64

	Genres    []Genre
	Artists   []Artist
	Albums    []Album
	Tracks    []Track
	Contexts  []Context
	Devices   []Device
	Platforms []string
	Plays     []Play // sorted by StartMs
	Now       NowPlaying
}
