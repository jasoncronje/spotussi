package live

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/query"
)

// ---------- entity pages ----------

// entityStats computes the in-range stats and rank of one entity, plus
// its all-time totals.
func (x *q) entityStats(spec query.Spec, metric query.Metric, k kind, key int32, s api.Scope) (query.Range, *query.Range, api.EntityStats, api.AllTimeStats, error) {
	cur, prev, plays, _, err := x.rangePlays(spec, api.Scope{})
	if err != nil {
		return cur, prev, api.EntityStats{}, api.AllTimeStats{}, err
	}
	items := x.rank(plays, k, metric)
	es := api.EntityStats{RankedTotal: len(items)}
	for n, it := range items {
		if it.key == key {
			es.Rank = ptr(n + 1)
			break
		}
	}
	sc, err := x.scope(s)
	if err != nil {
		return cur, prev, es, api.AllTimeStats{}, err
	}
	var m measure
	for _, i := range plays {
		if sc.has(x, i) {
			x.add(&m, i)
		}
	}
	es.Plays, es.MsPlayed, es.MsEstimated, es.Skips = m.plays, m.ms, m.est, m.skips

	var all measure
	var at api.AllTimeStats
	for _, i := range x.Vis {
		if !sc.has(x, i) {
			continue
		}
		x.add(&all, i)
		if at.FirstPlayedAt == nil {
			at.FirstPlayedAt = ptr(x.Start[i])
		}
		at.LastPlayedAt = ptr(x.Start[i])
	}
	at.Plays, at.MsPlayed, at.Skips = all.plays, all.ms, all.skips
	return cur, prev, es, at, nil
}

func (b *Backend) Artist(ctx context.Context, id string, spec query.Spec, metric query.Metric) (*api.ArtistDetail, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	a, ok := x.e.ArtistByID[id]
	if !ok {
		return nil, fmt.Errorf("%w: artist %q", api.ErrNotFound, id)
	}
	cur, prev, es, at, err := x.entityStats(spec, metric, kArtist, a, api.Scope{ArtistID: id})
	if err != nil {
		return nil, err
	}
	ref := x.artistRef(a)
	full := api.ArtistFull{ID: ref.ID, Name: ref.Name, ImageURL: ref.ImageURL, Genres: []api.GenreRef{}}
	for _, g := range x.e.Artists[a].Genres {
		full.Genres = append(full.Genres, x.genreRef(g))
	}
	if full.Metadata, err = b.metadata(ctx, "artist", ref.ID, x.e.Artists[a].Meta); err != nil {
		return nil, err
	}
	return &api.ArtistDetail{Artist: full, Excluded: x.ArtistExcluded(a), Range: wireRange(cur), Previous: wireRangePtr(prev),
		Metric: string(metric), InRange: es, AllTime: at}, nil
}

func (b *Backend) Album(ctx context.Context, id string, spec query.Spec, metric query.Metric) (*api.AlbumDetail, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	al, ok := x.e.AlbumByID[id]
	if !ok {
		return nil, fmt.Errorf("%w: album %q", api.ErrNotFound, id)
	}
	cur, prev, es, at, err := x.entityStats(spec, metric, kAlbum, al, api.Scope{AlbumID: id})
	if err != nil {
		return nil, err
	}
	r := &x.e.Albums[al]
	full := api.AlbumFull{ID: r.ID, Name: r.Name, ImageURL: strPtr(r.ImageURL), Artists: x.artistLites(r.Artists),
		AlbumType: strPtr(r.Type), ReleaseDate: strPtr(r.ReleaseDate)}
	if r.ReleaseYear > 0 {
		full.ReleaseYear = ptr(r.ReleaseYear)
	}
	if r.TotalTracks > 0 {
		full.TotalTracks = ptr(r.TotalTracks)
	}
	if full.Metadata, err = b.metadata(ctx, "album", r.ID, r.Meta); err != nil {
		return nil, err
	}
	return &api.AlbumDetail{Album: full, Range: wireRange(cur), Previous: wireRangePtr(prev), Metric: string(metric), InRange: es, AllTime: at}, nil
}

func (b *Backend) Track(ctx context.Context, id string, spec query.Spec, metric query.Metric) (*api.TrackDetail, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	t, ok := x.e.TrackByID[id]
	if !ok {
		return nil, fmt.Errorf("%w: track %q", api.ErrNotFound, id)
	}
	cur, prev, es, at, err := x.entityStats(spec, metric, kTrack, t, api.Scope{TrackID: id})
	if err != nil {
		return nil, err
	}
	r := &x.e.Tracks[t]
	ref := x.trackRef(t)
	full := api.TrackFull{ID: r.ID, Name: r.Name, ImageURL: ref.ImageURL, DurationMs: ref.DurationMs, Artists: []api.ArtistRef{}}
	if r.Explicit != derive.Unknown {
		full.Explicit = ptr(r.Explicit == derive.True)
	}
	excluded := x.TrackExcluded(t)
	for _, a := range r.Artists {
		full.Artists = append(full.Artists, x.artistRef(a))
		excluded = excluded || x.ArtistExcluded(a)
	}
	if r.Album >= 0 {
		al := x.albumRef(r.Album)
		full.Album = &al
	}
	if full.Metadata, err = b.metadata(ctx, "track", r.ID, r.Meta); err != nil {
		return nil, err
	}
	return &api.TrackDetail{Track: full, Excluded: excluded, Range: wireRange(cur), Previous: wireRangePtr(prev),
		Metric: string(metric), InRange: es, AllTime: at}, nil
}

func (b *Backend) Genre(ctx context.Context, id string, spec query.Spec, metric query.Metric) (*api.GenreDetail, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	g, ok := x.e.GenreByID[id]
	if !ok {
		return nil, fmt.Errorf("%w: genre %q", api.ErrNotFound, id)
	}
	cur, prev, es, at, err := x.entityStats(spec, metric, kGenre, g, api.Scope{GenreID: id})
	if err != nil {
		return nil, err
	}
	artists := 0
	for a := range x.e.Artists {
		if slices.Contains(x.e.Artists[a].Genres, g) {
			artists++
		}
	}
	return &api.GenreDetail{Genre: x.genreRef(g), Artists: artists, Range: wireRange(cur), Previous: wireRangePtr(prev),
		Metric: string(metric), InRange: es, AllTime: at}, nil
}

// ---------- contexts ----------

func (b *Backend) Contexts(ctx context.Context, p api.TopParams) (*api.Contexts, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, _, plays, _, err := x.rangePlays(p.Range, p.Scope)
	if err != nil {
		return nil, err
	}
	acc := make([]measure, len(x.e.Contexts))
	var unknown measure
	for _, i := range plays {
		if c := x.Ctx[i]; c >= 0 {
			x.add(&acc[c], i)
		} else {
			x.add(&unknown, i)
		}
	}
	var items []rankedItem
	for c, m := range acc {
		if m.value(p.Metric) > 0 {
			items = append(items, rankedItem{int32(c), m})
		}
	}
	slices.SortFunc(items, func(a, c rankedItem) int {
		if r := cmp.Compare(c.m.value(p.Metric), a.m.value(p.Metric)); r != 0 {
			return r
		}
		return cmp.Compare(x.e.Contexts[a.key].URI, x.e.Contexts[c.key].URI)
	})
	out := &api.Contexts{Range: wireRange(cur), Metric: string(p.Metric), Total: len(items), Offset: p.Page.Offset,
		Limit: p.Page.Limit, Items: []api.RankedContext{}, Unknown: api.Measure{Plays: unknown.plays, MsPlayed: unknown.ms}}
	for n, it := range page(items, p.Page) {
		out.Items = append(out.Items, api.RankedContext{Rank: p.Page.Offset + n + 1, Plays: it.m.plays, MsPlayed: it.m.ms, Context: x.contextRef(it.key)})
	}
	return out, nil
}

// SetPlaylistName names (or un-names) a Spotify-owned playlist the API
// cannot read (D17). The name is a manual contexts row, which hydration
// leaves alone.
func (b *Backend) SetPlaylistName(ctx context.Context, id string, name *string) (*api.ContextRef, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	var c *derive.Context
	for i := range x.e.Contexts {
		if x.e.Contexts[i].Type == "playlist" && x.e.Contexts[i].ID == id {
			c = &x.e.Contexts[i]
			break
		}
	}
	if c == nil {
		return nil, fmt.Errorf("%w: playlist %q", api.ErrNotFound, id)
	}
	if !c.SpotifyOwned {
		return nil, fmt.Errorf("%w: only Spotify-owned playlists can be named manually", api.ErrBadRequest)
	}
	var stored any
	source := "unresolved"
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return nil, fmt.Errorf("%w: name must not be blank", api.ErrBadRequest)
		}
		stored, source = n, "manual"
	}
	if _, err := b.db.Write.ExecContext(ctx, `INSERT INTO contexts (uri, type, name, name_source, resolved_at)
		VALUES (?, 'playlist', ?, ?, ?)
		ON CONFLICT (uri) DO UPDATE SET name = excluded.name, name_source = excluded.name_source,
			resolved_at = excluded.resolved_at`, c.URI, stored, source, b.now().UnixMilli()); err != nil {
		return nil, err
	}
	if err := b.engine.ContextsChanged(ctx); err != nil {
		return nil, err
	}
	if x, err = b.q(ctx); err != nil {
		return nil, err
	}
	r := x.contextRef(x.e.ContextByID[c.URI])
	return &r, nil
}

// ---------- search ----------

func (b *Backend) Search(ctx context.Context, qs string, limit int) (*api.SearchResults, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	qs = strings.ToLower(strings.TrimSpace(qs))
	match := func(n int, name, id func(int) string, plays []int64) []int32 {
		var hits []int32
		for i := range n {
			if containsLower(name(i), qs) {
				hits = append(hits, int32(i))
			}
		}
		slices.SortFunc(hits, func(a, c int32) int {
			return cmp.Or(cmp.Compare(plays[c], plays[a]), strings.Compare(id(int(a)), id(int(c))))
		})
		return hits[:min(len(hits), limit)]
	}
	e := x.e
	out := &api.SearchResults{Artists: []api.ArtistRef{}, Albums: []api.AlbumRef{}, Tracks: []api.TrackRef{}}
	for _, a := range match(len(e.Artists), func(i int) string { return e.Artists[i].Name },
		func(i int) string { return e.Artists[i].ID }, x.ArtistPlays) {
		out.Artists = append(out.Artists, x.artistRef(a))
	}
	for _, a := range match(len(e.Albums), func(i int) string { return e.Albums[i].Name },
		func(i int) string { return e.Albums[i].ID }, x.AlbumPlays) {
		out.Albums = append(out.Albums, x.albumRef(a))
	}
	for _, t := range match(len(e.Tracks), func(i int) string { return e.Tracks[i].Name },
		func(i int) string { return e.Tracks[i].ID }, x.TrackPlays) {
		out.Tracks = append(out.Tracks, x.trackRef(t))
	}
	return out, nil
}

// containsLower reports whether s contains the lower-case q, ignoring
// case, without allocating for ASCII names (a search scans every name).
func containsLower(s, q string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return strings.Contains(strings.ToLower(s), q)
		}
	}
	for i := 0; i+len(q) <= len(s); i++ {
		j := 0
		for ; j < len(q); j++ {
			c := s[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != q[j] {
				break
			}
		}
		if j == len(q) {
			return true
		}
	}
	return false
}

// ---------- exclusions ----------

func (b *Backend) Exclusions(ctx context.Context) (*api.Exclusions, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	out := &api.Exclusions{Artists: []api.ArtistRef{}, Tracks: []api.TrackRef{}}
	for n, ex := range x.View.Exclusions {
		i := x.ExclIndex[n]
		switch {
		case ex.Kind == "artist" && i >= 0:
			out.Artists = append(out.Artists, x.artistRef(i))
		case ex.Kind == "artist":
			out.Artists = append(out.Artists, api.ArtistRef{ID: ex.ID, Name: ex.Name})
		case i >= 0:
			out.Tracks = append(out.Tracks, x.trackRef(i))
		default:
			out.Tracks = append(out.Tracks, api.TrackRef{ID: ex.ID, Name: ex.Name, Artists: []api.ArtistLite{}})
		}
	}
	return out, nil
}

// exclusionTarget finds the entity an exclusion names: its canonical ID
// (a provisional ID merged into a real artist excludes the real one) and
// name.
func (x *q) exclusionTarget(kind api.ExclusionKind, id string) (string, string, string, bool) {
	if kind == api.ExcludeArtists {
		a, ok := x.e.ArtistByID[id]
		if !ok {
			return "", "", "", false
		}
		return "artist", x.e.Artists[a].ID, x.e.Artists[a].Name, true
	}
	t, ok := x.e.TrackByID[id]
	if !ok {
		return "", "", "", false
	}
	return "track", x.e.Tracks[t].ID, x.e.Tracks[t].Name, true
}

func (b *Backend) AddExclusion(ctx context.Context, kind api.ExclusionKind, id string) (*api.Exclusions, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	k, canon, name, ok := x.exclusionTarget(kind, id)
	if !ok {
		return nil, fmt.Errorf("%w: %s %q", api.ErrNotFound, strings.TrimSuffix(string(kind), "s"), id)
	}
	if _, err := b.db.Write.ExecContext(ctx, `INSERT INTO exclusions (kind, id, name, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (kind, id) DO NOTHING`, k, canon, name, b.now().UnixMilli()); err != nil {
		return nil, err
	}
	if err := b.exclusionsChanged(ctx); err != nil {
		return nil, err
	}
	return b.Exclusions(ctx)
}

func (b *Backend) RemoveExclusion(ctx context.Context, kind api.ExclusionKind, id string) (*api.Exclusions, error) {
	k := "track"
	if kind == api.ExcludeArtists {
		k = "artist"
	}
	ids := []any{k, id, id}
	if x, err := b.q(ctx); err == nil {
		if _, canon, _, ok := x.exclusionTarget(kind, id); ok {
			ids[2] = canon
		}
	}
	res, err := b.db.Write.ExecContext(ctx, `DELETE FROM exclusions WHERE kind = ? AND id IN (?, ?)`, ids...)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if err := b.exclusionsChanged(ctx); err != nil {
			return nil, err
		}
	}
	return b.Exclusions(ctx) // idempotent: unknown ids are simply not excluded
}

func (b *Backend) exclusionsChanged(ctx context.Context) error {
	rs, err := b.db.Write.QueryContext(ctx, `SELECT kind, id, name FROM exclusions ORDER BY created_at, kind, id`)
	if err != nil {
		return err
	}
	defer rs.Close()
	var excl []derive.Exclusion
	for rs.Next() {
		var x derive.Exclusion
		if err := rs.Scan(&x.Kind, &x.ID, &x.Name); err != nil {
			return err
		}
		excl = append(excl, x)
	}
	if err := rs.Err(); err != nil {
		return err
	}
	b.engine.SetExclusions(excl)
	return nil
}

// metadata reports where an entity's details come from, with the fallback
// sources that wrote its fields (catalog_provenance, D27) in stage order.
func (b *Backend) metadata(ctx context.Context, entityType, id string, m derive.Meta) (api.EntityMetadata, error) {
	out := api.EntityMetadata{State: api.MetaPending, Sources: []string{}}
	switch m {
	case derive.MetaSpotify:
		out.State = api.MetaSpotify
		return out, nil
	case derive.MetaPending:
		return out, nil
	}
	out.State = api.MetaFallback
	// 'borrowed' fields (copied from a same-name provisional artist, D27)
	// name no source of their own: they're left out, so an artist with only
	// borrowed details reports no sources (the UI says "public music
	// catalogs") and the API's source enum stays the four stage names.
	rs, err := b.db.Read.QueryContext(ctx, `SELECT DISTINCT source FROM catalog_provenance WHERE entity_type = ? AND entity_id = ?
		AND source != 'borrowed'
		ORDER BY CASE source WHEN 'oembed' THEN 0 WHEN 'deezer' THEN 1 WHEN 'listenbrainz' THEN 2 ELSE 3 END`, entityType, id)
	if err != nil {
		return out, err
	}
	defer rs.Close()
	for rs.Next() {
		var src string
		if err := rs.Scan(&src); err != nil {
			return out, err
		}
		out.Sources = append(out.Sources, src)
	}
	return out, rs.Err()
}
