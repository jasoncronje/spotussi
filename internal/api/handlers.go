package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"spotussi/internal/query"
)

// params accumulates the first validation error while reading query values.
type params struct {
	q   url.Values
	err error
}

func (p *params) fail(format string, args ...any) {
	if p.err == nil {
		p.err = fmt.Errorf("%w: "+format, append([]any{ErrBadRequest}, args...)...)
	}
}

func (p *params) spec(def query.Preset) query.Spec {
	s, err := query.ParseSpec(p.q.Get("range"), p.q.Get("from"), p.q.Get("to"), def)
	if err != nil && p.err == nil {
		p.err = err
	}
	return s
}

func (p *params) metric() query.Metric {
	m, err := query.ParseMetric(p.q.Get("metric"))
	if err != nil && p.err == nil {
		p.err = err
	}
	return m
}

func (p *params) bucket() query.Bucket {
	b, err := query.ParseBucket(p.q.Get("bucket"))
	if err != nil && p.err == nil {
		p.err = err
	}
	return b
}

func (p *params) int(name string, def, lo, hi int) int {
	v := p.q.Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		p.fail("%s must be an integer between %d and %d", name, lo, hi)
		return def
	}
	return n
}

func (p *params) bool(name string) *bool {
	v := p.q.Get(name)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		p.fail("%s must be true or false", name)
		return nil
	}
	return &b
}

// scope reads the allowed scope parameters; at most one may be set.
func (p *params) scope(artist, album, track bool) Scope {
	var s Scope
	n := 0
	if artist {
		if s.ArtistID = p.q.Get("artist_id"); s.ArtistID != "" {
			n++
		}
	}
	if album {
		if s.AlbumID = p.q.Get("album_id"); s.AlbumID != "" {
			n++
		}
	}
	if track {
		if s.TrackID = p.q.Get("track_id"); s.TrackID != "" {
			n++
		}
	}
	// genre_id is accepted wherever another scope is.
	if artist || album || track {
		if s.GenreID = p.q.Get("genre_id"); s.GenreID != "" {
			n++
		}
	}
	if n > 1 {
		p.fail("at most one of artist_id, album_id, track_id, genre_id may be given")
	}
	return s
}

func (p *params) page() Page {
	return Page{Limit: p.int("limit", 50, 1, 500), Offset: p.int("offset", 0, 0, 1<<30)}
}

func (p *params) top(artist, album, track bool) TopParams {
	return TopParams{Range: p.spec(query.Last4w), Metric: p.metric(), Page: p.page(), Scope: p.scope(artist, album, track)}
}

func (p *params) series() SeriesParams {
	return SeriesParams{Range: p.spec(query.Last4w), Metric: p.metric(), Bucket: p.bucket(), Series: p.int("series", 8, 1, 20)}
}

// get registers a GET handler that parses params then calls fn.
func get[T any](mux *http.ServeMux, pattern string, fn func(r *http.Request, p *params) (*T, error)) {
	mux.HandleFunc("GET "+pattern, func(w http.ResponseWriter, r *http.Request) {
		p := &params{q: r.URL.Query()}
		v, err := fn(r, p)
		if p.err != nil {
			writeResult[T](w, r, nil, p.err)
			return
		}
		writeResult(w, r, v, err)
	})
}

func decodeBody(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: invalid JSON body: %v", ErrBadRequest, err)
	}
	return nil
}

func exclusionKind(r *http.Request) (ExclusionKind, error) {
	switch k := ExclusionKind(r.PathValue("kind")); k {
	case ExcludeArtists, ExcludeTracks:
		return k, nil
	}
	return "", fmt.Errorf("%w: kind must be artists or tracks", ErrNotFound)
}

func registerRoutes(mux *http.ServeMux, b Backend) {
	registerLibrary(mux, b)
	get(mux, v1+"/now-playing", func(r *http.Request, _ *params) (*NowPlaying, error) {
		return b.NowPlaying(r.Context())
	})
	get(mux, v1+"/overview/summary", func(r *http.Request, p *params) (*OverviewSummary, error) {
		s, m := p.spec(query.Last4w), p.metric()
		if p.err != nil {
			return nil, nil
		}
		return b.OverviewSummary(r.Context(), s, m)
	})
	get(mux, v1+"/timeline", func(r *http.Request, p *params) (*Timeline, error) {
		s, bk, sc, cmp := p.spec(query.Last4w), p.bucket(), p.scope(true, true, true), p.bool("compare")
		if p.err != nil {
			return nil, nil
		}
		return b.Timeline(r.Context(), s, bk, sc, cmp != nil && *cmp)
	})
	get(mux, v1+"/clock", func(r *http.Request, p *params) (*Clock, error) {
		s, sc := p.spec(query.Last4w), p.scope(true, true, true)
		if p.err != nil {
			return nil, nil
		}
		return b.Clock(r.Context(), s, sc)
	})
	get(mux, v1+"/top/tracks", func(r *http.Request, p *params) (*TopTracks, error) {
		tp := p.top(true, true, false)
		if p.err != nil {
			return nil, nil
		}
		return b.TopTracks(r.Context(), tp)
	})
	get(mux, v1+"/top/artists", func(r *http.Request, p *params) (*TopArtists, error) {
		tp := p.top(false, true, false)
		if p.err != nil {
			return nil, nil
		}
		return b.TopArtists(r.Context(), tp)
	})
	get(mux, v1+"/top/albums", func(r *http.Request, p *params) (*TopAlbums, error) {
		tp := p.top(true, false, false)
		if p.err != nil {
			return nil, nil
		}
		return b.TopAlbums(r.Context(), tp)
	})
	get(mux, v1+"/top/genres", func(r *http.Request, p *params) (*TopGenres, error) {
		tp := p.top(false, false, false)
		if p.err != nil {
			return nil, nil
		}
		return b.TopGenres(r.Context(), tp)
	})
	get(mux, v1+"/stats/hourly-top", func(r *http.Request, p *params) (*HourlyTop, error) {
		s, m := p.spec(query.Last4w), p.metric()
		if p.err != nil {
			return nil, nil
		}
		return b.HourlyTop(r.Context(), s, m)
	})
	get(mux, v1+"/stats/artists-over-time", func(r *http.Request, p *params) (*ArtistsOverTime, error) {
		sp := p.series()
		if p.err != nil {
			return nil, nil
		}
		return b.ArtistsOverTime(r.Context(), sp)
	})
	get(mux, v1+"/stats/genres-over-time", func(r *http.Request, p *params) (*GenresOverTime, error) {
		sp := p.series()
		if p.err != nil {
			return nil, nil
		}
		return b.GenresOverTime(r.Context(), sp)
	})
	get(mux, v1+"/stats/release-years", func(r *http.Request, p *params) (*AverageSeries, error) {
		sp := p.series()
		if p.err != nil {
			return nil, nil
		}
		return b.ReleaseYears(r.Context(), sp)
	})
	get(mux, v1+"/stats/artists-per-track", func(r *http.Request, p *params) (*AverageSeries, error) {
		sp := p.series()
		if p.err != nil {
			return nil, nil
		}
		return b.ArtistsPerTrack(r.Context(), sp)
	})
	get(mux, v1+"/artists/{id}", func(r *http.Request, p *params) (*ArtistDetail, error) {
		s, m := p.spec(query.Last4w), p.metric()
		if p.err != nil {
			return nil, nil
		}
		return b.Artist(r.Context(), r.PathValue("id"), s, m)
	})
	get(mux, v1+"/albums/{id}", func(r *http.Request, p *params) (*AlbumDetail, error) {
		s, m := p.spec(query.Last4w), p.metric()
		if p.err != nil {
			return nil, nil
		}
		return b.Album(r.Context(), r.PathValue("id"), s, m)
	})
	get(mux, v1+"/tracks/{id}", func(r *http.Request, p *params) (*TrackDetail, error) {
		s, m := p.spec(query.Last4w), p.metric()
		if p.err != nil {
			return nil, nil
		}
		return b.Track(r.Context(), r.PathValue("id"), s, m)
	})
	get(mux, v1+"/genres/{id}", func(r *http.Request, p *params) (*GenreDetail, error) {
		s, m := p.spec(query.Last4w), p.metric()
		if p.err != nil {
			return nil, nil
		}
		return b.Genre(r.Context(), r.PathValue("id"), s, m)
	})
	get(mux, v1+"/contexts", func(r *http.Request, p *params) (*Contexts, error) {
		tp := p.top(true, true, true)
		if p.err != nil {
			return nil, nil
		}
		return b.Contexts(r.Context(), tp)
	})
	mux.HandleFunc("PUT "+v1+"/playlists/{id}/name", func(w http.ResponseWriter, r *http.Request) {
		var in PlaylistNameInput
		if err := decodeBody(r, &in); err != nil {
			writeResult[ContextRef](w, r, nil, err)
			return
		}
		if in.Name != nil && (len(*in.Name) == 0 || len(*in.Name) > 200) {
			writeResult[ContextRef](w, r, nil, fmt.Errorf("%w: name must be 1-200 characters or null", ErrBadRequest))
			return
		}
		v, err := b.SetPlaylistName(r.Context(), r.PathValue("id"), in.Name)
		writeResult(w, r, v, err)
	})
	get(mux, v1+"/history", func(r *http.Request, p *params) (*History, error) {
		hp := HistoryParams{
			Range: p.spec(query.All), Scope: p.scope(true, true, true),
			Skipped: p.bool("skipped"), Shuffle: p.bool("shuffle"), Counted: p.bool("counted"),
			Device: p.q.Get("device"), Platform: p.q.Get("platform"),
			Cursor: p.q.Get("cursor"), Limit: p.int("limit", 50, 1, 200),
		}
		if p.err != nil {
			return nil, nil
		}
		return b.History(r.Context(), hp)
	})
	get(mux, v1+"/devices", func(r *http.Request, p *params) (*Devices, error) {
		s := p.spec(query.All)
		if p.err != nil {
			return nil, nil
		}
		return b.Devices(r.Context(), s)
	})
	get(mux, v1+"/sessions", func(r *http.Request, p *params) (*Sessions, error) {
		s, pg := p.spec(query.Last4w), p.page()
		if p.err != nil {
			return nil, nil
		}
		return b.Sessions(r.Context(), s, pg)
	})
	get(mux, v1+"/sessions/{id}", func(r *http.Request, _ *params) (*SessionDetail, error) {
		return b.Session(r.Context(), r.PathValue("id"))
	})
	get(mux, v1+"/search", func(r *http.Request, p *params) (*SearchResults, error) {
		q := p.q.Get("q")
		if q == "" || len(q) > 200 {
			p.fail("q must be 1-200 characters")
		}
		limit := p.int("limit", 5, 1, 50)
		if p.err != nil {
			return nil, nil
		}
		return b.Search(r.Context(), q, limit)
	})
	get(mux, v1+"/settings", func(r *http.Request, _ *params) (*Settings, error) {
		return b.Settings(r.Context())
	})
	mux.HandleFunc("PATCH "+v1+"/settings", func(w http.ResponseWriter, r *http.Request) {
		var in SettingsPatch
		if err := decodeBody(r, &in); err != nil {
			writeResult[Settings](w, r, nil, err)
			return
		}
		v, err := b.UpdateSettings(r.Context(), in)
		writeResult(w, r, v, err)
	})
	get(mux, v1+"/exclusions", func(r *http.Request, _ *params) (*Exclusions, error) {
		return b.Exclusions(r.Context())
	})
	mux.HandleFunc("PUT "+v1+"/exclusions/{kind}/{id}", func(w http.ResponseWriter, r *http.Request) {
		k, err := exclusionKind(r)
		if err != nil {
			writeResult[Exclusions](w, r, nil, err)
			return
		}
		v, err := b.AddExclusion(r.Context(), k, r.PathValue("id"))
		writeResult(w, r, v, err)
	})
	mux.HandleFunc("DELETE "+v1+"/exclusions/{kind}/{id}", func(w http.ResponseWriter, r *http.Request) {
		k, err := exclusionKind(r)
		if err != nil {
			writeResult[Exclusions](w, r, nil, err)
			return
		}
		v, err := b.RemoveExclusion(r.Context(), k, r.PathValue("id"))
		writeResult(w, r, v, err)
	})
}
