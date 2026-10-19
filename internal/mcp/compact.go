package mcp

import (
	"math"
	"sync"
	"time"

	"spotussi/internal/api"
)

// Compact result shapes. Tool results are read by a model, so they keep
// names and IDs (for chaining calls) and drop what only the UI needs
// (artwork URLs, epoch milliseconds, per-field provenance). Durations are
// minutes (aggregates) or seconds (single plays); times are local
// RFC 3339 in the owner's display timezone.

type ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type rangeOut struct {
	Preset   string `json:"preset"`
	From     string `json:"from"`
	To       string `json:"to"`
	Timezone string `json:"timezone"`
}

type trackOut struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Artists []ref  `json:"artists"`
	Album   *ref   `json:"album,omitempty"`
}

type albumOut struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Artists     []ref  `json:"artists"`
	ReleaseYear *int   `json:"release_year,omitempty"`
}

// rank is the common head of a ranked list item.
type rank struct {
	Rank int `json:"rank"`
	// PrevRank is the rank in the previous period of equal length; absent
	// when unranked then (new) or when there is no previous period.
	PrevRank *int    `json:"prev_rank,omitempty"`
	Plays    int64   `json:"plays"`
	Minutes  float64 `json:"minutes"`
}

type rankedTrack struct {
	rank
	trackOut
}

type rankedArtist struct {
	rank
	ref
}

type rankedAlbum struct {
	rank
	albumOut
}

type rankedGenre struct {
	rank
	ref
}

type rankedContext struct {
	Rank    int     `json:"rank"`
	Plays   int64   `json:"plays"`
	Minutes float64 `json:"minutes"`
	contextOut
}

type contextOut struct {
	Type string  `json:"type"`
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

func minutes(ms int64) float64 { return math.Round(float64(ms)/6000) / 10 }

func seconds(ms int64) int64 { return (ms + 500) / 1000 }

func compactRange(r api.ResolvedRange) rangeOut {
	return rangeOut{Preset: r.Preset, From: r.From, To: r.To, Timezone: r.Timezone}
}

func compactRangePtr(r *api.ResolvedRange) *rangeOut {
	if r == nil {
		return nil
	}
	out := compactRange(*r)
	return &out
}

func lites(in []api.ArtistLite) []ref {
	out := make([]ref, len(in))
	for i, a := range in {
		out[i] = ref{a.ID, a.Name}
	}
	return out
}

func artistRefs(in []api.ArtistRef) []ref {
	out := make([]ref, len(in))
	for i, a := range in {
		out[i] = ref{a.ID, a.Name}
	}
	return out
}

func genreRefs(in []api.GenreRef) []ref {
	out := make([]ref, len(in))
	for i, g := range in {
		out[i] = ref{g.ID, g.Name}
	}
	return out
}

func compactTrack(t api.TrackRef) trackOut {
	out := trackOut{ID: t.ID, Name: t.Name, Artists: lites(t.Artists)}
	if t.Album != nil {
		out.Album = &ref{t.Album.ID, t.Album.Name}
	}
	return out
}

func compactAlbum(a api.AlbumRef) albumOut {
	return albumOut{ID: a.ID, Name: a.Name, Artists: lites(a.Artists), ReleaseYear: a.ReleaseYear}
}

func compactContext(c api.ContextRef) contextOut {
	return contextOut{Type: c.Type, ID: c.ID, Name: c.Name}
}

func rankedTracks(in []api.RankedTrack) []rankedTrack {
	out := make([]rankedTrack, len(in))
	for i, t := range in {
		out[i] = rankedTrack{rank{t.Rank, t.PrevRank, t.Plays, minutes(t.MsPlayed)}, compactTrack(t.Track)}
	}
	return out
}

func rankedArtists(in []api.RankedArtist) []rankedArtist {
	out := make([]rankedArtist, len(in))
	for i, a := range in {
		out[i] = rankedArtist{rank{a.Rank, a.PrevRank, a.Plays, minutes(a.MsPlayed)}, ref{a.Artist.ID, a.Artist.Name}}
	}
	return out
}

func rankedAlbums(in []api.RankedAlbum) []rankedAlbum {
	out := make([]rankedAlbum, len(in))
	for i, a := range in {
		out[i] = rankedAlbum{rank{a.Rank, a.PrevRank, a.Plays, minutes(a.MsPlayed)}, compactAlbum(a.Album)}
	}
	return out
}

func rankedGenres(in []api.RankedGenre) []rankedGenre {
	out := make([]rankedGenre, len(in))
	for i, g := range in {
		out[i] = rankedGenre{rank{g.Rank, g.PrevRank, g.Plays, minutes(g.MsPlayed)}, ref{g.Genre.ID, g.Genre.Name}}
	}
	return out
}

func rankedContexts(in []api.RankedContext) []rankedContext {
	out := make([]rankedContext, len(in))
	for i, c := range in {
		out[i] = rankedContext{c.Rank, c.Plays, minutes(c.MsPlayed), compactContext(c.Context)}
	}
	return out
}

// ---------- times ----------

var zones sync.Map // name → *time.Location

func zone(name string) *time.Location {
	if v, ok := zones.Load(name); ok {
		return v.(*time.Location)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		loc = time.UTC
	}
	zones.Store(name, loc)
	return loc
}

// localTime formats epoch ms as RFC 3339 in the named zone.
func localTime(ms int64, tz string) string {
	return time.UnixMilli(ms).In(zone(tz)).Format(time.RFC3339)
}

func localTimePtr(ms *int64, tz string) *string {
	if ms == nil {
		return nil
	}
	s := localTime(*ms, tz)
	return &s
}

// ---------- plays and sessions ----------

type playOut struct {
	ID       string `json:"id"`
	PlayedAt string `json:"played_at"`
	Seconds  int64  `json:"seconds"`
	// Estimated: the listening time is the track length, not measured.
	Estimated bool        `json:"estimated,omitempty"`
	Counted   bool        `json:"counted"`
	Skipped   bool        `json:"skipped"`
	Shuffle   *bool       `json:"shuffle,omitempty"`
	Track     trackOut    `json:"track"`
	Context   *contextOut `json:"context,omitempty"`
	Device    string      `json:"device,omitempty"`
	Platform  *string     `json:"platform,omitempty"`
}

func compactPlay(p api.Play, tz string) playOut {
	out := playOut{ID: p.ID, PlayedAt: localTime(p.PlayedAt, tz), Seconds: seconds(p.MsPlayed),
		Estimated: p.MsEstimated, Counted: p.CountsAsPlay, Skipped: p.Skipped, Shuffle: p.Shuffle,
		Track: compactTrack(p.Track), Platform: p.Platform}
	if p.Context != nil {
		c := compactContext(*p.Context)
		out.Context = &c
	}
	if p.Device != nil {
		out.Device = p.Device.Name
		if p.Device.Type != "" {
			out.Device += " (" + p.Device.Type + ")"
		}
	}
	return out
}

type sessionOut struct {
	ID            string  `json:"id"`
	StartedAt     string  `json:"started_at"`
	EndedAt       string  `json:"ended_at"`
	Minutes       float64 `json:"minutes"`
	MinutesPlayed float64 `json:"minutes_played"`
	Plays         int64   `json:"plays"`
	Rows          int64   `json:"rows"`
	Skips         int64   `json:"skips"`
	TopArtists    []ref   `json:"top_artists"`
}

func compactSession(s api.Session, tz string) sessionOut {
	return sessionOut{ID: s.ID, StartedAt: localTime(s.StartedAt, tz), EndedAt: localTime(s.EndedAt, tz),
		Minutes: minutes(s.DurationMs), MinutesPlayed: minutes(s.MsPlayed), Plays: s.Plays, Rows: s.Rows,
		Skips: s.Skips, TopArtists: artistRefs(s.TopArtists)}
}

// ---------- entity stats ----------

type inRangeOut struct {
	Rank        *int    `json:"rank"`
	RankedTotal int     `json:"ranked_total"`
	Plays       int64   `json:"plays"`
	Minutes     float64 `json:"minutes"`
	Skips       int64   `json:"skips"`
}

type allTimeOut struct {
	Plays       int64   `json:"plays"`
	Minutes     float64 `json:"minutes"`
	Skips       int64   `json:"skips"`
	FirstPlayed *string `json:"first_played"`
	LastPlayed  *string `json:"last_played"`
}

func compactInRange(s api.EntityStats) inRangeOut {
	return inRangeOut{Rank: s.Rank, RankedTotal: s.RankedTotal, Plays: s.Plays, Minutes: minutes(s.MsPlayed), Skips: s.Skips}
}

func compactAllTime(s api.AllTimeStats, tz string) allTimeOut {
	return allTimeOut{Plays: s.Plays, Minutes: minutes(s.MsPlayed), Skips: s.Skips,
		FirstPlayed: localTimePtr(s.FirstPlayedAt, tz), LastPlayed: localTimePtr(s.LastPlayedAt, tz)}
}
