package mcp

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"spotussi/internal/api"
	"spotussi/internal/query"
)

// ---------- get_status ----------

type statusOut struct {
	Version  string      `json:"version"`
	Mode     string      `json:"mode"`
	Settings settingsOut `json:"settings"`
	Data     dataOut     `json:"data"`
	Capture  captureOut  `json:"capture"`
	Catalog  catalogOut  `json:"catalog"`
	Derive   deriveOut   `json:"stats_rebuild"`
	Imports  int         `json:"imports_running"`
	Playing  *nowPlayOut `json:"now_playing"`
}

type settingsOut struct {
	Timezone          string `json:"timezone"`
	WeekStart         string `json:"week_start"`
	PlayRule          string `json:"play_rule"`
	PlayRuleSeconds   int64  `json:"play_rule_threshold_seconds"`
	SessionGapMinutes int    `json:"session_gap_minutes"`
}

type dataOut struct {
	Plays     int64   `json:"plays"`
	FirstPlay *string `json:"first_play"`
	LastPlay  *string `json:"last_play"`
}

type captureOut struct {
	State     string  `json:"state"`
	LastPlay  *string `json:"last_play"`
	LastError *string `json:"last_error,omitempty"`
}

type catalogOut struct {
	State string `json:"state"`
	// Hydrated counts entities with full catalog metadata (artwork,
	// genres, release dates); the rest show export names only.
	TracksHydrated  string `json:"tracks_hydrated"`
	ArtistsHydrated string `json:"artists_hydrated"`
	AlbumsHydrated  string `json:"albums_hydrated"`
	Queued          int64  `json:"queued"`
}

type deriveOut struct {
	State    string   `json:"state"`
	Progress *float64 `json:"progress,omitempty"`
}

type nowPlayOut struct {
	IsPlaying bool      `json:"is_playing"`
	Track     *trackOut `json:"track,omitempty"`
	Seconds   *int64    `json:"progress_seconds,omitempty"`
	Device    string    `json:"device,omitempty"`
}

func status(ctx context.Context, b api.Backend, o Options) (*statusOut, error) {
	st, err := b.SystemStatus(ctx)
	if err != nil {
		return nil, err
	}
	set, err := b.Settings(ctx)
	if err != nil {
		return nil, err
	}
	tz := set.Timezone
	out := &statusOut{Version: o.Version, Mode: o.Mode,
		Settings: settingsOut{Timezone: tz, WeekStart: set.WeekStart, PlayRule: set.PlayRule,
			PlayRuleSeconds: seconds(set.PlayRuleThresholdMs), SessionGapMinutes: set.SessionGapMinutes},
		Data: dataOut{Plays: st.Data.Plays, FirstPlay: localTimePtr(st.Data.FirstPlayAt, tz),
			LastPlay: localTimePtr(st.Data.LastPlayAt, tz)},
		Capture: captureOut{State: st.Capture.State, LastPlay: localTimePtr(st.Capture.LastPlayAt, tz),
			LastError: st.Capture.LastError},
		Catalog: catalogOut{State: st.Catalog.State,
			TracksHydrated:  fmt.Sprintf("%d/%d", st.Catalog.TracksHydrated, st.Catalog.TracksTotal),
			ArtistsHydrated: fmt.Sprintf("%d/%d", st.Catalog.ArtistsHydrated, st.Catalog.ArtistsTotal),
			AlbumsHydrated:  fmt.Sprintf("%d/%d", st.Catalog.AlbumsHydrated, st.Catalog.AlbumsTotal),
			Queued:          st.Catalog.QueueLength},
		Derive: deriveOut{State: st.Derive.State, Progress: st.Derive.Progress},
	}
	for _, j := range st.Imports {
		if j.State == "queued" || j.State == "running" {
			out.Imports++
		}
	}
	np, err := b.NowPlaying(ctx)
	if err != nil {
		return nil, err
	}
	if np != nil {
		p := &nowPlayOut{IsPlaying: np.IsPlaying}
		if np.Track != nil {
			t := compactTrack(*np.Track)
			p.Track = &t
		}
		if np.ProgressMs != nil {
			s := seconds(*np.ProgressMs)
			p.Seconds = &s
		}
		if np.Device != nil {
			p.Device = np.Device.Name
		}
		out.Playing = p
	}
	return out, nil
}

// ---------- get_overview ----------

type overviewArgs struct {
	rangeArgs
	Metric string `json:"metric,omitempty" jsonschema:"rank the top lists by plays (default) or listening time"`
}

type summaryOut struct {
	Plays            int64   `json:"plays"`
	Minutes          float64 `json:"minutes"`
	MinutesEstimated float64 `json:"minutes_estimated"`
	Skips            int64   `json:"skips"`
	Tracks           int64   `json:"tracks"`
	Artists          int64   `json:"artists"`
	Albums           int64   `json:"albums"`
}

type changeOut struct {
	PlaysPct   *float64 `json:"plays_pct"`
	MinutesPct *float64 `json:"minutes_pct"`
}

type overviewOut struct {
	Range         rangeOut       `json:"range"`
	PreviousRange *rangeOut      `json:"previous_range"`
	Metric        string         `json:"metric"`
	Current       summaryOut     `json:"current"`
	Previous      *summaryOut    `json:"previous"`
	Change        *changeOut     `json:"change"`
	TopArtists    []rankedArtist `json:"top_artists"`
	TopTracks     []rankedTrack  `json:"top_tracks"`
	TopAlbums     []rankedAlbum  `json:"top_albums"`
	TopGenres     []rankedGenre  `json:"top_genres"`
	ByHourPlays   [24]int64      `json:"by_hour_plays"`
	ByHourMinutes [24]float64    `json:"by_hour_minutes"`
}

func compactSummary(s api.Summary) summaryOut {
	return summaryOut{Plays: s.Plays, Minutes: minutes(s.MsPlayed), MinutesEstimated: minutes(s.MsEstimated),
		Skips: s.Skips, Tracks: s.Tracks, Artists: s.Artists, Albums: s.Albums}
}

func pctChange(cur, prev int64) *float64 {
	if prev == 0 {
		return nil
	}
	v := math.Round(float64(cur-prev)/float64(prev)*1000) / 10
	return &v
}

func overview(ctx context.Context, b api.Backend, in overviewArgs) (*overviewOut, error) {
	spec, err := in.spec(query.Last4w)
	if err != nil {
		return nil, err
	}
	m, err := metric(in.Metric)
	if err != nil {
		return nil, err
	}
	sum, err := b.OverviewSummary(ctx, spec, m)
	if err != nil {
		return nil, err
	}
	out := &overviewOut{Range: compactRange(sum.Range), PreviousRange: compactRangePtr(sum.Previous),
		Metric: sum.Metric, Current: compactSummary(sum.Current)}
	if sum.PreviousSummary != nil {
		p := compactSummary(*sum.PreviousSummary)
		out.Previous = &p
		out.Change = &changeOut{PlaysPct: pctChange(sum.Current.Plays, sum.PreviousSummary.Plays),
			MinutesPct: pctChange(sum.Current.MsPlayed, sum.PreviousSummary.MsPlayed)}
	}
	tp := api.TopParams{Range: spec, Metric: m, Page: api.Page{Limit: relatedLimit}}
	ar, err := b.TopArtists(ctx, tp)
	if err != nil {
		return nil, err
	}
	tr, err := b.TopTracks(ctx, tp)
	if err != nil {
		return nil, err
	}
	al, err := b.TopAlbums(ctx, tp)
	if err != nil {
		return nil, err
	}
	ge, err := b.TopGenres(ctx, tp)
	if err != nil {
		return nil, err
	}
	out.TopArtists, out.TopTracks = rankedArtists(ar.Items), rankedTracks(tr.Items)
	out.TopAlbums, out.TopGenres = rankedAlbums(al.Items), rankedGenres(ge.Items)
	clock, err := b.Clock(ctx, spec, api.Scope{})
	if err != nil {
		return nil, err
	}
	for _, h := range clock.Hours {
		if h.Hour >= 0 && h.Hour < 24 {
			out.ByHourPlays[h.Hour], out.ByHourMinutes[h.Hour] = h.Plays, minutes(h.MsPlayed)
		}
	}
	return out, nil
}

// ---------- get_top ----------

type topArgs struct {
	Type string `json:"type" jsonschema:"what to rank: tracks, artists, albums, genres or contexts (playlists, albums and artists playback was started from)"`
	rangeArgs
	Metric string `json:"metric,omitempty" jsonschema:"rank by plays (default) or listening time"`
	Limit  int    `json:"limit,omitempty" jsonschema:"items to return, 1-50 (default 10)"`
	Offset int    `json:"offset,omitempty" jsonschema:"items to skip, for paging (default 0)"`
	scopeArgs
}

type topOut struct {
	Type          string    `json:"type"`
	Range         rangeOut  `json:"range"`
	PreviousRange *rangeOut `json:"previous_range"`
	Metric        string    `json:"metric"`
	Total         int       `json:"total"`
	Offset        int       `json:"offset"`
	NextOffset    *int      `json:"next_offset"`
	Items         any       `json:"items"`
	// Unknown is listening with no recorded context (contexts only).
	Unknown *unknownOut `json:"unknown_context,omitempty"`
}

type unknownOut struct {
	Plays   int64   `json:"plays"`
	Minutes float64 `json:"minutes"`
}

func nextOffset(offset, n, total int) *int {
	if offset+n >= total || n == 0 {
		return nil
	}
	v := offset + n
	return &v
}

func top(ctx context.Context, b api.Backend, in topArgs) (*topOut, error) {
	spec, err := in.spec(query.Last4w)
	if err != nil {
		return nil, err
	}
	m, err := metric(in.Metric)
	if err != nil {
		return nil, err
	}
	limit, err := clamp(in.Limit, 10, 1, maxTopLimit, "limit")
	if err != nil {
		return nil, err
	}
	if in.Offset < 0 {
		return nil, fmt.Errorf("%w: offset must not be negative", api.ErrBadRequest)
	}
	var sc api.Scope
	switch in.Type {
	case "tracks":
		sc, err = in.scope(true, true, false, true)
	case "artists":
		sc, err = in.scope(false, true, false, true)
	case "albums":
		sc, err = in.scope(true, false, false, true)
	case "genres":
		sc, err = in.scope(false, false, false, false)
	case "contexts":
		sc, err = in.scope(true, true, true, true)
	default:
		return nil, fmt.Errorf("%w: type must be tracks, artists, albums, genres or contexts", api.ErrBadRequest)
	}
	if err != nil {
		return nil, err
	}
	p := api.TopParams{Range: spec, Metric: m, Page: api.Page{Limit: limit, Offset: in.Offset}, Scope: sc}
	out := &topOut{Type: in.Type, Offset: in.Offset}
	var n int
	switch in.Type {
	case "tracks":
		r, err := b.TopTracks(ctx, p)
		if err != nil {
			return nil, err
		}
		out.Range, out.PreviousRange, out.Metric, out.Total = compactRange(r.Range), compactRangePtr(r.Previous), r.Metric, r.Total
		out.Items, n = rankedTracks(r.Items), len(r.Items)
	case "artists":
		r, err := b.TopArtists(ctx, p)
		if err != nil {
			return nil, err
		}
		out.Range, out.PreviousRange, out.Metric, out.Total = compactRange(r.Range), compactRangePtr(r.Previous), r.Metric, r.Total
		out.Items, n = rankedArtists(r.Items), len(r.Items)
	case "albums":
		r, err := b.TopAlbums(ctx, p)
		if err != nil {
			return nil, err
		}
		out.Range, out.PreviousRange, out.Metric, out.Total = compactRange(r.Range), compactRangePtr(r.Previous), r.Metric, r.Total
		out.Items, n = rankedAlbums(r.Items), len(r.Items)
	case "genres":
		r, err := b.TopGenres(ctx, p)
		if err != nil {
			return nil, err
		}
		out.Range, out.PreviousRange, out.Metric, out.Total = compactRange(r.Range), compactRangePtr(r.Previous), r.Metric, r.Total
		out.Items, n = rankedGenres(r.Items), len(r.Items)
	case "contexts":
		r, err := b.Contexts(ctx, p)
		if err != nil {
			return nil, err
		}
		out.Range, out.Metric, out.Total = compactRange(r.Range), r.Metric, r.Total
		out.Items, n = rankedContexts(r.Items), len(r.Items)
		out.Unknown = &unknownOut{Plays: r.Unknown.Plays, Minutes: minutes(r.Unknown.MsPlayed)}
	}
	out.NextOffset = nextOffset(in.Offset, n, out.Total)
	return out, nil
}

// ---------- get_entity ----------

type entityArgs struct {
	Kind string `json:"kind" jsonschema:"artist, album, track or genre"`
	ID   string `json:"id" jsonschema:"the entity's ID (from search_catalog or another result)"`
	rangeArgs
	Metric string `json:"metric,omitempty" jsonschema:"rank by plays (default) or listening time"`
}

type entityOut struct {
	Kind          string     `json:"kind"`
	Entity        any        `json:"entity"`
	Excluded      bool       `json:"excluded,omitempty"`
	Range         rangeOut   `json:"range"`
	PreviousRange *rangeOut  `json:"previous_range"`
	Metric        string     `json:"metric"`
	InRange       inRangeOut `json:"in_range"`
	AllTime       allTimeOut `json:"all_time"`

	TopTracks  []rankedTrack   `json:"top_tracks,omitempty"`
	TopAlbums  []rankedAlbum   `json:"top_albums,omitempty"`
	TopArtists []rankedArtist  `json:"top_artists,omitempty"`
	PlayedFrom []rankedContext `json:"played_from,omitempty"`
}

type artistFullOut struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Genres []ref  `json:"genres"`
}

type albumFullOut struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        *string `json:"type"`
	ReleaseDate *string `json:"release_date"`
	TotalTracks *int    `json:"total_tracks"`
	Artists     []ref   `json:"artists"`
}

type trackFullOut struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Seconds  *int64    `json:"duration_seconds"`
	Explicit *bool     `json:"explicit"`
	Artists  []ref     `json:"artists"`
	Album    *albumOut `json:"album"`
}

type genreFullOut struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Artists int    `json:"artists"`
}

func entity(ctx context.Context, b api.Backend, in entityArgs) (*entityOut, error) {
	spec, err := in.spec(query.Last4w)
	if err != nil {
		return nil, err
	}
	m, err := metric(in.Metric)
	if err != nil {
		return nil, err
	}
	if in.ID == "" {
		return nil, fmt.Errorf("%w: id is required", api.ErrBadRequest)
	}
	out := &entityOut{Kind: in.Kind}
	head := func(r api.ResolvedRange, prev *api.ResolvedRange, metric string, s api.EntityStats, all api.AllTimeStats) {
		out.Range, out.PreviousRange, out.Metric = compactRange(r), compactRangePtr(prev), metric
		out.InRange, out.AllTime = compactInRange(s), compactAllTime(all, r.Timezone)
	}
	related := api.TopParams{Range: spec, Metric: m, Page: api.Page{Limit: relatedLimit}}
	switch in.Kind {
	case "artist":
		d, err := b.Artist(ctx, in.ID, spec, m)
		if err != nil {
			return nil, err
		}
		head(d.Range, d.Previous, d.Metric, d.InRange, d.AllTime)
		out.Entity = artistFullOut{ID: d.Artist.ID, Name: d.Artist.Name, Genres: genreRefs(d.Artist.Genres)}
		out.Excluded = d.Excluded
		related.Scope = api.Scope{ArtistID: in.ID}
		tr, err := b.TopTracks(ctx, related)
		if err != nil {
			return nil, err
		}
		al, err := b.TopAlbums(ctx, related)
		if err != nil {
			return nil, err
		}
		out.TopTracks, out.TopAlbums = rankedTracks(tr.Items), rankedAlbums(al.Items)
	case "album":
		d, err := b.Album(ctx, in.ID, spec, m)
		if err != nil {
			return nil, err
		}
		head(d.Range, d.Previous, d.Metric, d.InRange, d.AllTime)
		a := d.Album
		out.Entity = albumFullOut{ID: a.ID, Name: a.Name, Type: a.AlbumType, ReleaseDate: a.ReleaseDate,
			TotalTracks: a.TotalTracks, Artists: lites(a.Artists)}
		related.Scope = api.Scope{AlbumID: in.ID}
		tr, err := b.TopTracks(ctx, related)
		if err != nil {
			return nil, err
		}
		out.TopTracks = rankedTracks(tr.Items)
	case "track":
		d, err := b.Track(ctx, in.ID, spec, m)
		if err != nil {
			return nil, err
		}
		head(d.Range, d.Previous, d.Metric, d.InRange, d.AllTime)
		t := d.Track
		e := trackFullOut{ID: t.ID, Name: t.Name, Explicit: t.Explicit, Artists: artistRefs(t.Artists)}
		if t.DurationMs != nil {
			s := seconds(*t.DurationMs)
			e.Seconds = &s
		}
		if t.Album != nil {
			a := compactAlbum(*t.Album)
			e.Album = &a
		}
		out.Entity, out.Excluded = e, d.Excluded
		related.Scope = api.Scope{TrackID: in.ID}
		cx, err := b.Contexts(ctx, related)
		if err != nil {
			return nil, err
		}
		out.PlayedFrom = rankedContexts(cx.Items)
	case "genre":
		d, err := b.Genre(ctx, in.ID, spec, m)
		if err != nil {
			return nil, err
		}
		head(d.Range, d.Previous, d.Metric, d.InRange, d.AllTime)
		out.Entity = genreFullOut{ID: d.Genre.ID, Name: d.Genre.Name, Artists: d.Artists}
		related.Scope = api.Scope{GenreID: in.ID}
		ar, err := b.TopArtists(ctx, related)
		if err != nil {
			return nil, err
		}
		tr, err := b.TopTracks(ctx, related)
		if err != nil {
			return nil, err
		}
		out.TopArtists, out.TopTracks = rankedArtists(ar.Items), rankedTracks(tr.Items)
	default:
		return nil, fmt.Errorf("%w: kind must be artist, album, track or genre", api.ErrBadRequest)
	}
	return out, nil
}

// ---------- get_timeline ----------

type timelineArgs struct {
	rangeArgs
	Bucket string `json:"bucket,omitempty" jsonschema:"bucket size: auto (default), hour, day, week, month or year"`
	scopeArgs
}

type pointOut struct {
	Key     string  `json:"key"`
	Plays   int64   `json:"plays"`
	Minutes float64 `json:"minutes"`
	Skips   int64   `json:"skips"`
	Artists int64   `json:"artists"`
}

type timelineOut struct {
	Range  rangeOut   `json:"range"`
	Bucket string     `json:"bucket"`
	Points []pointOut `json:"points"`
}

func timeline(ctx context.Context, b api.Backend, in timelineArgs) (*timelineOut, error) {
	spec, err := in.spec(query.Last4w)
	if err != nil {
		return nil, err
	}
	bk, err := query.ParseBucket(in.Bucket)
	if err != nil {
		return nil, err
	}
	sc, err := in.scope(true, true, true, true)
	if err != nil {
		return nil, err
	}
	t, err := b.Timeline(ctx, spec, bk, sc, false)
	if err != nil {
		return nil, err
	}
	if len(t.Points) > maxTimelinePts {
		return nil, fmt.Errorf("%w: %d %s buckets is more than %d; use a coarser bucket or a shorter range",
			api.ErrBadRequest, len(t.Points), t.Bucket, maxTimelinePts)
	}
	out := &timelineOut{Range: compactRange(t.Range), Bucket: t.Bucket, Points: make([]pointOut, len(t.Points))}
	for i, p := range t.Points {
		out.Points[i] = pointOut{Key: p.Key, Plays: p.Plays, Minutes: minutes(p.MsPlayed), Skips: p.Skips, Artists: p.Artists}
	}
	return out, nil
}

// ---------- get_history ----------

type historyArgs struct {
	rangeArgs
	scopeArgs
	Skipped  *bool  `json:"skipped,omitempty" jsonschema:"only skipped (true) or only not skipped (false) plays"`
	Shuffle  *bool  `json:"shuffle,omitempty" jsonschema:"only plays with shuffle on (true) or off (false)"`
	Counted  *bool  `json:"counted,omitempty" jsonschema:"only plays that counted under the play rule (true) or that did not (false)"`
	Device   string `json:"device,omitempty" jsonschema:"only plays on this device name (exact)"`
	Platform string `json:"platform,omitempty" jsonschema:"only plays on this platform (exact)"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
	Limit    int    `json:"limit,omitempty" jsonschema:"plays to return, 1-100 (default 25)"`
}

type historyOut struct {
	Range      rangeOut  `json:"range"`
	Total      int       `json:"total"`
	Items      []playOut `json:"items"`
	NextCursor *string   `json:"next_cursor"`
}

func history(ctx context.Context, b api.Backend, in historyArgs) (*historyOut, error) {
	spec, err := in.spec(query.All)
	if err != nil {
		return nil, err
	}
	sc, err := in.scope(true, true, true, true)
	if err != nil {
		return nil, err
	}
	limit, err := clamp(in.Limit, 25, 1, maxHistoryLimit, "limit")
	if err != nil {
		return nil, err
	}
	h, err := b.History(ctx, api.HistoryParams{Range: spec, Scope: sc, Skipped: in.Skipped, Shuffle: in.Shuffle,
		Counted: in.Counted, Device: in.Device, Platform: in.Platform, Cursor: in.Cursor, Limit: limit})
	if err != nil {
		return nil, err
	}
	tz := h.Range.Timezone
	out := &historyOut{Range: compactRange(h.Range), Total: h.Total, Items: make([]playOut, len(h.Items)), NextCursor: h.NextCursor}
	for i, p := range h.Items {
		out.Items[i] = compactPlay(p, tz)
	}
	return out, nil
}

// ---------- get_sessions ----------

type sessionsArgs struct {
	rangeArgs
	Limit     int    `json:"limit,omitempty" jsonschema:"sessions to return, 1-50 (default 10)"`
	Offset    int    `json:"offset,omitempty" jsonschema:"sessions to skip, for paging (default 0)"`
	SessionID string `json:"session_id,omitempty" jsonschema:"return this one session with its plays (range, limit and offset are ignored)"`
}

type sessionsOut struct {
	Range      rangeOut     `json:"range"`
	GapMinutes int          `json:"gap_minutes"`
	Total      int          `json:"total"`
	Offset     int          `json:"offset"`
	NextOffset *int         `json:"next_offset"`
	Items      []sessionOut `json:"items"`
}

type sessionDetailOut struct {
	Session    sessionOut `json:"session"`
	Plays      []playOut  `json:"plays"`
	PlaysShown int        `json:"plays_shown"`
	PlaysTotal int        `json:"plays_total"`
}

func sessions(ctx context.Context, b api.Backend, in sessionsArgs) (any, error) {
	if in.SessionID != "" {
		d, err := b.Session(ctx, in.SessionID)
		if err != nil {
			return nil, err
		}
		set, err := b.Settings(ctx)
		if err != nil {
			return nil, err
		}
		tz := set.Timezone
		plays := d.Plays
		if len(plays) > maxSessionPlays {
			plays = plays[:maxSessionPlays]
		}
		out := &sessionDetailOut{Session: compactSession(d.Session, tz), Plays: make([]playOut, len(plays)),
			PlaysShown: len(plays), PlaysTotal: len(d.Plays)}
		for i, p := range plays {
			out.Plays[i] = compactPlay(p, tz)
		}
		return out, nil
	}
	spec, err := in.spec(query.Last4w)
	if err != nil {
		return nil, err
	}
	limit, err := clamp(in.Limit, 10, 1, maxSessionLimit, "limit")
	if err != nil {
		return nil, err
	}
	if in.Offset < 0 {
		return nil, fmt.Errorf("%w: offset must not be negative", api.ErrBadRequest)
	}
	s, err := b.Sessions(ctx, spec, api.Page{Limit: limit, Offset: in.Offset})
	if err != nil {
		return nil, err
	}
	tz := s.Range.Timezone
	out := &sessionsOut{Range: compactRange(s.Range), GapMinutes: s.GapMinutes, Total: s.Total, Offset: in.Offset,
		NextOffset: nextOffset(in.Offset, len(s.Items), s.Total), Items: make([]sessionOut, len(s.Items))}
	for i, x := range s.Items {
		out.Items[i] = compactSession(x, tz)
	}
	return out, nil
}

// ---------- search_catalog ----------

type searchArgs struct {
	Query string `json:"query" jsonschema:"part of an artist, album or track name"`
	Limit int    `json:"limit,omitempty" jsonschema:"results per kind, 1-20 (default 5)"`
}

type searchOut struct {
	Artists []ref      `json:"artists"`
	Albums  []albumOut `json:"albums"`
	Tracks  []trackOut `json:"tracks"`
}

func search(ctx context.Context, b api.Backend, in searchArgs) (*searchOut, error) {
	q := strings.TrimSpace(in.Query)
	if q == "" || utf8.RuneCountInString(q) > 200 {
		return nil, fmt.Errorf("%w: query must be 1-200 characters", api.ErrBadRequest)
	}
	limit, err := clamp(in.Limit, 5, 1, maxSearchLimit, "limit")
	if err != nil {
		return nil, err
	}
	r, err := b.Search(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	out := &searchOut{Artists: artistRefs(r.Artists), Albums: make([]albumOut, len(r.Albums)), Tracks: make([]trackOut, len(r.Tracks))}
	for i, a := range r.Albums {
		out.Albums[i] = compactAlbum(a)
	}
	for i, t := range r.Tracks {
		out.Tracks[i] = compactTrack(t)
	}
	return out, nil
}
