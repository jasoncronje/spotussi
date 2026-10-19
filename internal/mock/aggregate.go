package mock

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/query"
	"spotussi/internal/refdata"
)

// kind is an entity kind that plays can be ranked by.
type kind int

const (
	kTrack kind = iota
	kArtist
	kAlbum
	kGenre
)

// kindID is the API ID of an entity (ranking ties break on it, as on a
// live instance).
func (b *Backend) kindID(k kind, key int32) string {
	switch k {
	case kTrack:
		return b.ds.Tracks[key].ID
	case kArtist:
		return b.ds.Artists[key].ID
	case kAlbum:
		return b.ds.Albums[key].ID
	default:
		return b.ds.Genres[key].ID
	}
}

func (b *Backend) kindSize(k kind) int {
	switch k {
	case kTrack:
		return len(b.ds.Tracks)
	case kArtist:
		return len(b.ds.Artists)
	case kAlbum:
		return len(b.ds.Albums)
	default:
		return len(b.ds.Genres)
	}
}

// keys appends the entities of kind k that play p is attributed to:
// the track, its album, every credited artist, or the union of the
// credited artists' genres (each once).
func (b *Backend) keys(k kind, p refdata.Play, buf []int32) []int32 {
	t := b.ds.Tracks[p.Track]
	switch k {
	case kTrack:
		return append(buf, p.Track)
	case kAlbum:
		return append(buf, t.Album)
	case kArtist:
		return append(buf, t.Artists...)
	default:
		for _, a := range t.Artists {
			for _, g := range b.genres(a) {
				if !slices.Contains(buf, g) {
					buf = append(buf, g)
				}
			}
		}
		return buf
	}
}

type rankedItem struct {
	key int32
	m   measure
}

// rank aggregates plays by kind and sorts by metric (then the other
// measure, then ID) dropping entities whose metric value is zero.
func (b *Backend) rank(plays []int32, k kind, metric query.Metric) []rankedItem {
	acc := make([]measure, b.kindSize(k))
	seen := make([]bool, len(acc))
	var touched []int32
	var buf []int32
	for _, i := range plays {
		buf = b.keys(k, b.ds.Plays[i], buf[:0])
		for _, key := range buf {
			b.add(&acc[key], i)
			if !seen[key] {
				seen[key] = true
				touched = append(touched, key)
			}
		}
	}
	out := make([]rankedItem, 0, len(touched))
	for _, key := range touched {
		if acc[key].value(metric) > 0 {
			out = append(out, rankedItem{key, acc[key]})
		}
	}
	other := query.MetricTime
	if metric == query.MetricTime {
		other = query.MetricPlays
	}
	sort.Slice(out, func(i, j int) bool {
		a, c := out[i].m, out[j].m
		if a.value(metric) != c.value(metric) {
			return a.value(metric) > c.value(metric)
		}
		if a.value(other) != c.value(other) {
			return a.value(other) > c.value(other)
		}
		return b.kindID(k, out[i].key) < b.kindID(k, out[j].key)
	})
	return out
}

func rankIndex(items []rankedItem) map[int32]int {
	m := make(map[int32]int, len(items))
	for i, it := range items {
		m[it.key] = i + 1
	}
	return m
}

func (b *Backend) filter(plays []int32, f scopeFn) []int32 {
	out := make([]int32, 0, len(plays)/4)
	for _, i := range plays {
		if f(b.ds.Plays[i]) {
			out = append(out, i)
		}
	}
	return out
}

// rangePlays resolves spec and returns the scoped plays of the range and of
// the previous period (nil if none).
func (b *Backend) rangePlays(spec query.Spec, s api.Scope) (query.Range, *query.Range, []int32, []int32, error) {
	cur, prev, err := b.resolve(spec)
	if err != nil {
		return cur, prev, nil, nil, err
	}
	f, err := b.scope(s)
	if err != nil {
		return cur, prev, nil, nil, err
	}
	plays := b.filter(b.inRange(cur), f)
	var prevPlays []int32
	if prev != nil {
		prevPlays = b.filter(b.inRange(*prev), f)
	}
	return cur, prev, plays, prevPlays, nil
}

// ---------- overview ----------

func (b *Backend) summary(plays []int32, metric query.Metric, prevRanks [2]map[int32]int) (api.Summary, [2]map[int32]int) {
	var m measure
	for _, i := range plays {
		b.add(&m, i)
	}
	s := api.Summary{Plays: m.plays, MsPlayed: m.ms, MsEstimated: m.est, Skips: m.skips}
	countPlayed := func(items []rankedItem) int64 {
		var n int64
		for _, it := range items {
			if it.m.plays > 0 {
				n++
			}
		}
		return n
	}
	tracks := b.rank(plays, kTrack, metric)
	artists := b.rank(plays, kArtist, metric)
	s.Tracks, s.Artists = countPlayed(tracks), countPlayed(artists)
	s.Albums = countPlayed(b.rank(plays, kAlbum, metric))
	if len(artists) > 0 {
		r := b.rankedArtist(artists[0], 1, prevRanks[0])
		s.TopArtist = &r
	}
	if len(tracks) > 0 {
		r := b.rankedTrack(tracks[0], 1, prevRanks[1])
		s.TopTrack = &r
	}
	return s, [2]map[int32]int{rankIndex(artists), rankIndex(tracks)}
}

func prevRank(prev map[int32]int, key int32) *int {
	if r, ok := prev[key]; ok {
		return &r
	}
	return nil
}

func (b *Backend) rankedArtist(it rankedItem, rank int, prev map[int32]int) api.RankedArtist {
	return api.RankedArtist{Rank: rank, PrevRank: prevRank(prev, it.key), Plays: it.m.plays, MsPlayed: it.m.ms, Artist: b.artistRef(it.key)}
}

func (b *Backend) rankedTrack(it rankedItem, rank int, prev map[int32]int) api.RankedTrack {
	return api.RankedTrack{Rank: rank, PrevRank: prevRank(prev, it.key), Plays: it.m.plays, MsPlayed: it.m.ms, Track: b.trackRef(it.key)}
}

func (b *Backend) OverviewSummary(ctx context.Context, spec query.Spec, metric query.Metric) (*api.OverviewSummary, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, prev, plays, prevPlays, err := b.rangePlays(spec, api.Scope{})
	if err != nil {
		return nil, err
	}
	out := &api.OverviewSummary{Range: wireRange(cur), Previous: wireRangePtr(prev), Metric: string(metric)}
	var prevRanks [2]map[int32]int
	if prev != nil {
		ps, ranks := b.summary(prevPlays, metric, [2]map[int32]int{})
		out.PreviousSummary = &ps
		prevRanks = ranks
	}
	out.Current, _ = b.summary(plays, metric, prevRanks)
	var src, cat [3]measure
	for _, i := range plays {
		p := b.ds.Plays[i]
		export, live := p.Has(refdata.FlagSrcExport), p.Flags&(refdata.FlagSrcPoller|refdata.FlagSrcRecent) != 0
		switch {
		case export && live:
			b.add(&src[2], i)
		case export:
			b.add(&src[0], i)
		default:
			b.add(&src[1], i)
		}
		b.add(&cat[b.trackMeta[p.Track]], i)
	}
	m := func(v measure) api.Measure { return api.Measure{Plays: v.plays, MsPlayed: v.ms} }
	out.Sources = api.SourceMix{Export: m(src[0]), Live: m(src[1]), Both: m(src[2])}
	out.Catalog = api.CatalogMix{Pending: m(cat[derive.MetaPending]), Spotify: m(cat[derive.MetaSpotify]), Fallback: m(cat[derive.MetaFallback])}
	return out, nil
}

// buckets assigns plays (sorted by start) to spans; fn is called per play
// with its bucket index.
func (b *Backend) buckets(spans []query.Span, plays []int32, fn func(bucket int, i int32)) {
	k := 0
	for _, i := range plays {
		ms := b.ds.Plays[i].StartMs
		for k < len(spans) && ms >= spans[k].End.UnixMilli() {
			k++
		}
		if k == len(spans) {
			return
		}
		if ms >= spans[k].Start.UnixMilli() {
			fn(k, i)
		}
	}
}

func (b *Backend) Timeline(ctx context.Context, spec query.Spec, bucket query.Bucket, s api.Scope, compare bool) (*api.Timeline, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, prev, plays, prevPlays, err := b.rangePlays(spec, s)
	if err != nil {
		return nil, err
	}
	bk, spans, err := query.Buckets(cur, bucket, b.weekStart())
	if err != nil {
		return nil, err
	}
	out := &api.Timeline{Range: wireRange(cur), Bucket: string(bk), Points: b.timelinePoints(spans, plays)}
	if compare {
		out.Compared = true
		if prev != nil {
			pspans, err := query.AlignedBuckets(*prev, bk, b.weekStart(), len(spans))
			if err != nil {
				return nil, err
			}
			out.Previous = wireRangePtr(prev)
			out.PreviousPoints = b.timelinePoints(pspans, prevPlays)
		}
	}
	return out, nil
}

// timelinePoints aggregates plays (sorted, already clipped to a range) into
// spans.
func (b *Backend) timelinePoints(spans []query.Span, plays []int32) []api.TimelinePoint {
	ms := make([]measure, len(spans))
	artists := make([]int64, len(spans))
	lastBucket := make([]int, len(b.ds.Artists)) // bucket index + 1 an artist was last counted in
	b.buckets(spans, plays, func(k int, i int32) {
		b.add(&ms[k], i)
		if !b.counted[i] {
			return
		}
		for _, a := range b.ds.Tracks[b.ds.Plays[i].Track].Artists {
			if lastBucket[a] != k+1 {
				lastBucket[a] = k + 1
				artists[k]++
			}
		}
	})
	out := make([]api.TimelinePoint, len(spans))
	for k, sp := range spans {
		m := ms[k]
		out[k] = api.TimelinePoint{Key: sp.Key, StartAt: sp.Start.UnixMilli(), Plays: m.plays, MsPlayed: m.ms,
			MsEstimated: m.est, Skips: m.skips, Artists: artists[k]}
	}
	return out
}

func (b *Backend) Clock(ctx context.Context, spec query.Spec, s api.Scope) (*api.Clock, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, _, plays, _, err := b.rangePlays(spec, s)
	if err != nil {
		return nil, err
	}
	var hours [24]measure
	for _, i := range plays {
		b.add(&hours[b.hour[i]], i)
	}
	out := &api.Clock{Range: wireRange(cur), Hours: make([]api.ClockHour, 24)}
	for h, m := range hours {
		out.Hours[h] = api.ClockHour{Hour: h, Plays: m.plays, MsPlayed: m.ms}
	}
	return out, nil
}

// ---------- top lists ----------

func page[T any](items []T, p api.Page) []T {
	if p.Offset >= len(items) {
		return []T{}
	}
	return items[p.Offset:min(len(items), p.Offset+p.Limit)]
}

func top[T any](b *Backend, p api.TopParams, k kind, wire func(it rankedItem, rank int, prev map[int32]int) T) (*api.TopList[T], error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, prev, plays, prevPlays, err := b.rangePlays(p.Range, p.Scope)
	if err != nil {
		return nil, err
	}
	items := b.rank(plays, k, p.Metric)
	var prevRanks map[int32]int
	if prev != nil {
		prevRanks = rankIndex(b.rank(prevPlays, k, p.Metric))
	}
	out := &api.TopList[T]{Range: wireRange(cur), Previous: wireRangePtr(prev), Metric: string(p.Metric),
		Total: len(items), Offset: p.Page.Offset, Limit: p.Page.Limit, Items: []T{}}
	for n, it := range page(items, p.Page) {
		out.Items = append(out.Items, wire(it, p.Page.Offset+n+1, prevRanks))
	}
	return out, nil
}

func (b *Backend) TopTracks(ctx context.Context, p api.TopParams) (*api.TopTracks, error) {
	return top(b, p, kTrack, b.rankedTrack)
}

func (b *Backend) TopArtists(ctx context.Context, p api.TopParams) (*api.TopArtists, error) {
	return top(b, p, kArtist, b.rankedArtist)
}

func (b *Backend) TopAlbums(ctx context.Context, p api.TopParams) (*api.TopAlbums, error) {
	return top(b, p, kAlbum, func(it rankedItem, rank int, prev map[int32]int) api.RankedAlbum {
		return api.RankedAlbum{Rank: rank, PrevRank: prevRank(prev, it.key), Plays: it.m.plays, MsPlayed: it.m.ms, Album: b.albumRef(it.key)}
	})
}

func (b *Backend) TopGenres(ctx context.Context, p api.TopParams) (*api.TopGenres, error) {
	return top(b, p, kGenre, func(it rankedItem, rank int, prev map[int32]int) api.RankedGenre {
		return api.RankedGenre{Rank: rank, PrevRank: prevRank(prev, it.key), Plays: it.m.plays, MsPlayed: it.m.ms, Genre: b.genreRef(it.key)}
	})
}

// ---------- stats ----------

func (b *Backend) HourlyTop(ctx context.Context, spec query.Spec, metric query.Metric) (*api.HourlyTop, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, _, plays, _, err := b.rangePlays(spec, api.Scope{})
	if err != nil {
		return nil, err
	}
	var byHour [24][]int32
	for _, i := range plays {
		byHour[b.hour[i]] = append(byHour[b.hour[i]], i)
	}
	out := &api.HourlyTop{Range: wireRange(cur), Metric: string(metric), Hours: make([]api.HourTop, 24)}
	for h, hp := range byHour {
		var m measure
		for _, i := range hp {
			b.add(&m, i)
		}
		ht := api.HourTop{Hour: h, Plays: m.plays, MsPlayed: m.ms}
		if a := b.rank(hp, kArtist, metric); len(a) > 0 {
			ht.TopArtist = &api.HourTopArtist{Plays: a[0].m.plays, MsPlayed: a[0].m.ms, Artist: b.artistRef(a[0].key)}
		}
		if t := b.rank(hp, kTrack, metric); len(t) > 0 {
			ht.TopTrack = &api.HourTopTrack{Plays: t[0].m.plays, MsPlayed: t[0].m.ms, Track: b.trackRef(t[0].key)}
		}
		out.Hours[h] = ht
	}
	return out, nil
}

// breakdown computes per-bucket values of the range's top n entities of
// kind k. other = max(0, total - sum(values)): plays crediting several
// artists or genres are counted once per entity, so values can overlap.
func (b *Backend) breakdown(p api.SeriesParams, k kind) (query.Range, query.Bucket, []int32, []api.BreakdownPoint, error) {
	cur, _, plays, _, err := b.rangePlays(p.Range, api.Scope{})
	if err != nil {
		return cur, "", nil, nil, err
	}
	bk, spans, err := query.Buckets(cur, p.Bucket, b.weekStart())
	if err != nil {
		return cur, "", nil, nil, err
	}
	items := b.rank(plays, k, p.Metric)
	series := make([]int32, 0, p.Series)
	pos := map[int32]int{}
	for _, it := range items[:min(len(items), p.Series)] {
		pos[it.key] = len(series)
		series = append(series, it.key)
	}
	points := make([]api.BreakdownPoint, len(spans))
	for n, sp := range spans {
		points[n] = api.BreakdownPoint{Key: sp.Key, StartAt: sp.Start.UnixMilli(), Values: make([]int64, len(series))}
	}
	var buf []int32
	b.buckets(spans, plays, func(n int, i int32) {
		w := b.weight(i, p.Metric)
		points[n].Total += w
		buf = b.keys(k, b.ds.Plays[i], buf[:0])
		for _, key := range buf {
			if s, ok := pos[key]; ok {
				points[n].Values[s] += w
			}
		}
	})
	for n := range points {
		var sum int64
		for _, v := range points[n].Values {
			sum += v
		}
		points[n].Other = max(0, points[n].Total-sum)
	}
	return cur, bk, series, points, nil
}

func (b *Backend) ArtistsOverTime(ctx context.Context, p api.SeriesParams) (*api.ArtistsOverTime, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, bk, series, points, err := b.breakdown(p, kArtist)
	if err != nil {
		return nil, err
	}
	out := &api.ArtistsOverTime{Range: wireRange(cur), Metric: string(p.Metric), Bucket: string(bk), Series: []api.ArtistRef{}, Points: points}
	for _, a := range series {
		out.Series = append(out.Series, b.artistRef(a))
	}
	return out, nil
}

func (b *Backend) GenresOverTime(ctx context.Context, p api.SeriesParams) (*api.GenresOverTime, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, bk, series, points, err := b.breakdown(p, kGenre)
	if err != nil {
		return nil, err
	}
	out := &api.GenresOverTime{Range: wireRange(cur), Metric: string(p.Metric), Bucket: string(bk), Series: []api.GenreRef{}, Points: points}
	for _, g := range series {
		out.Series = append(out.Series, b.genreRef(g))
	}
	return out, nil
}

// average computes a metric-weighted average of attr per bucket. attr
// returns the value and whether it is known for a play.
func (b *Backend) average(p api.SeriesParams, attr func(i int32) (float64, bool)) (*api.AverageSeries, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, _, plays, _, err := b.rangePlays(p.Range, api.Scope{})
	if err != nil {
		return nil, err
	}
	bk, spans, err := query.Buckets(cur, p.Bucket, b.weekStart())
	if err != nil {
		return nil, err
	}
	type acc struct{ sum, known, all float64 }
	accs := make([]acc, len(spans))
	var total acc
	b.buckets(spans, plays, func(n int, i int32) {
		w := float64(b.weight(i, p.Metric))
		accs[n].all += w
		total.all += w
		if v, ok := attr(i); ok {
			accs[n].sum += v * w
			accs[n].known += w
			total.sum += v * w
			total.known += w
		}
	})
	avg := func(a acc) (*float64, float64) {
		if a.known == 0 {
			return nil, 0
		}
		return ptr(a.sum / a.known), a.known / a.all
	}
	out := &api.AverageSeries{Range: wireRange(cur), Metric: string(p.Metric), Bucket: string(bk), Points: make([]api.AveragePoint, len(spans))}
	out.Overall, _ = avg(total)
	for n, sp := range spans {
		v, c := avg(accs[n])
		out.Points[n] = api.AveragePoint{Key: sp.Key, StartAt: sp.Start.UnixMilli(), Value: v, Coverage: c}
	}
	return out, nil
}

func (b *Backend) ReleaseYears(ctx context.Context, p api.SeriesParams) (*api.AverageSeries, error) {
	return b.average(p, func(i int32) (float64, bool) {
		t := b.ds.Tracks[b.ds.Plays[i].Track]
		if b.trackMeta[b.ds.Plays[i].Track] != derive.MetaSpotify {
			return 0, false
		}
		return float64(b.ds.Albums[t.Album].ReleaseYear), true
	})
}

func (b *Backend) ArtistsPerTrack(ctx context.Context, p api.SeriesParams) (*api.AverageSeries, error) {
	return b.average(p, func(i int32) (float64, bool) {
		t := b.ds.Tracks[b.ds.Plays[i].Track]
		if b.trackMeta[b.ds.Plays[i].Track] != derive.MetaSpotify {
			return 0, false
		}
		return float64(len(t.Artists)), true
	})
}

// ---------- entity pages ----------

// entityStats computes the in-range stats and rank of one entity, plus its
// all-time totals.
func (b *Backend) entityStats(spec query.Spec, metric query.Metric, k kind, key int32, s api.Scope) (query.Range, *query.Range, api.EntityStats, api.AllTimeStats, error) {
	cur, prev, plays, _, err := b.rangePlays(spec, api.Scope{})
	if err != nil {
		return cur, prev, api.EntityStats{}, api.AllTimeStats{}, err
	}
	items := b.rank(plays, k, metric)
	es := api.EntityStats{RankedTotal: len(items)}
	for n, it := range items {
		if it.key == key {
			es.Rank = ptr(n + 1)
			break
		}
	}
	f, err := b.scope(s)
	if err != nil {
		return cur, prev, es, api.AllTimeStats{}, err
	}
	var m measure
	for _, i := range plays {
		if f(b.ds.Plays[i]) {
			b.add(&m, i)
		}
	}
	es.Plays, es.MsPlayed, es.MsEstimated, es.Skips = m.plays, m.ms, m.est, m.skips

	var all measure
	var at api.AllTimeStats
	for _, i := range b.vis {
		p := b.ds.Plays[i]
		if !f(p) {
			continue
		}
		b.add(&all, i)
		if at.FirstPlayedAt == nil {
			at.FirstPlayedAt = ptr(p.StartMs)
		}
		at.LastPlayedAt = ptr(p.StartMs)
	}
	at.Plays, at.MsPlayed, at.Skips = all.plays, all.ms, all.skips
	return cur, prev, es, at, nil
}

func (b *Backend) Artist(ctx context.Context, id string, spec query.Spec, metric query.Metric) (*api.ArtistDetail, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	a, ok := b.byID.artist[id]
	if !ok {
		return nil, fmt.Errorf("%w: artist %q", api.ErrNotFound, id)
	}
	cur, prev, es, at, err := b.entityStats(spec, metric, kArtist, a, api.Scope{ArtistID: id})
	if err != nil {
		return nil, err
	}
	ref := b.artistRef(a)
	full := api.ArtistFull{ID: ref.ID, Name: ref.Name, ImageURL: ref.ImageURL, Genres: []api.GenreRef{},
		Metadata: metadata(b.artistMeta[a], "oembed", "deezer", "musicbrainz")}
	for _, g := range b.genres(a) {
		full.Genres = append(full.Genres, b.genreRef(g))
	}
	return &api.ArtistDetail{Artist: full, Excluded: b.exclArtists[a], Range: wireRange(cur), Previous: wireRangePtr(prev),
		Metric: string(metric), InRange: es, AllTime: at}, nil
}

func (b *Backend) Album(ctx context.Context, id string, spec query.Spec, metric query.Metric) (*api.AlbumDetail, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	al, ok := b.byID.album[id]
	if !ok {
		return nil, fmt.Errorf("%w: album %q", api.ErrNotFound, id)
	}
	cur, prev, es, at, err := b.entityStats(spec, metric, kAlbum, al, api.Scope{AlbumID: id})
	if err != nil {
		return nil, err
	}
	x := b.ds.Albums[al]
	full := api.AlbumFull{ID: x.ID, Name: x.Name, ImageURL: b.albumImage(al), Artists: b.artistLites(x.Artists),
		Metadata: metadata(b.albumMeta[al], "deezer")}
	if b.albumMeta[al] != derive.MetaPending {
		full.AlbumType, full.ReleaseDate = ptr(x.Type), ptr(x.ReleaseDate)
		full.ReleaseYear, full.TotalTracks = ptr(x.ReleaseYear), ptr(x.TotalTracks)
	}
	return &api.AlbumDetail{Album: full, Range: wireRange(cur), Previous: wireRangePtr(prev), Metric: string(metric), InRange: es, AllTime: at}, nil
}

func (b *Backend) Track(ctx context.Context, id string, spec query.Spec, metric query.Metric) (*api.TrackDetail, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	t, ok := b.byID.track[id]
	if !ok {
		return nil, fmt.Errorf("%w: track %q", api.ErrNotFound, id)
	}
	cur, prev, es, at, err := b.entityStats(spec, metric, kTrack, t, api.Scope{TrackID: id})
	if err != nil {
		return nil, err
	}
	x := b.ds.Tracks[t]
	ref := b.trackRef(t)
	full := api.TrackFull{ID: x.ID, Name: x.Name, ImageURL: ref.ImageURL, DurationMs: ref.DurationMs,
		Metadata: metadata(b.trackMeta[t], "oembed", "deezer")}
	if b.trackMeta[t] == derive.MetaSpotify {
		full.Explicit = ptr(x.Explicit)
	}
	for _, a := range x.Artists {
		full.Artists = append(full.Artists, b.artistRef(a))
	}
	al := b.albumRef(x.Album)
	full.Album = &al
	excluded := b.exclTracks[t]
	for _, a := range x.Artists {
		excluded = excluded || b.exclArtists[a]
	}
	return &api.TrackDetail{Track: full, Excluded: excluded, Range: wireRange(cur), Previous: wireRangePtr(prev),
		Metric: string(metric), InRange: es, AllTime: at}, nil
}

func (b *Backend) Genre(ctx context.Context, id string, spec query.Spec, metric query.Metric) (*api.GenreDetail, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	g, ok := b.byID.genre[id]
	if !ok {
		return nil, fmt.Errorf("%w: genre %q", api.ErrNotFound, id)
	}
	cur, prev, es, at, err := b.entityStats(spec, metric, kGenre, g, api.Scope{GenreID: id})
	if err != nil {
		return nil, err
	}
	artists := 0
	for a := range b.ds.Artists {
		if slices.Contains(b.genres(int32(a)), g) {
			artists++
		}
	}
	return &api.GenreDetail{Genre: b.genreRef(g), Artists: artists, Range: wireRange(cur), Previous: wireRangePtr(prev),
		Metric: string(metric), InRange: es, AllTime: at}, nil
}

func (b *Backend) Contexts(ctx context.Context, p api.TopParams) (*api.Contexts, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, _, plays, _, err := b.rangePlays(p.Range, p.Scope)
	if err != nil {
		return nil, err
	}
	acc := make([]measure, len(b.ds.Contexts))
	var unknown measure
	for _, i := range plays {
		if c := b.ds.Plays[i].Context; c >= 0 {
			b.add(&acc[c], i)
		} else {
			b.add(&unknown, i)
		}
	}
	var items []rankedItem
	for c, m := range acc {
		if m.value(p.Metric) > 0 {
			items = append(items, rankedItem{int32(c), m})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		a, c := items[i].m.value(p.Metric), items[j].m.value(p.Metric)
		if a != c {
			return a > c
		}
		return b.ds.Contexts[items[i].key].URI < b.ds.Contexts[items[j].key].URI
	})
	out := &api.Contexts{Range: wireRange(cur), Metric: string(p.Metric), Total: len(items), Offset: p.Page.Offset,
		Limit: p.Page.Limit, Items: []api.RankedContext{}, Unknown: api.Measure{Plays: unknown.plays, MsPlayed: unknown.ms}}
	for n, it := range page(items, p.Page) {
		out.Items = append(out.Items, api.RankedContext{Rank: p.Page.Offset + n + 1, Plays: it.m.plays, MsPlayed: it.m.ms, Context: b.contextRef(it.key)})
	}
	return out, nil
}

// ---------- search ----------

func (b *Backend) Search(ctx context.Context, q string, limit int) (*api.SearchResults, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	q = strings.ToLower(strings.TrimSpace(q))
	match := func(k kind, n int, name func(int) string, plays []int64) []int32 {
		var hits []int32
		for i := range n {
			if strings.Contains(strings.ToLower(name(i)), q) {
				hits = append(hits, int32(i))
			}
		}
		sort.Slice(hits, func(x, y int) bool {
			if plays[hits[x]] != plays[hits[y]] {
				return plays[hits[x]] > plays[hits[y]]
			}
			return b.kindID(k, hits[x]) < b.kindID(k, hits[y])
		})
		return hits[:min(len(hits), limit)]
	}
	ds := b.ds
	out := &api.SearchResults{Artists: []api.ArtistRef{}, Albums: []api.AlbumRef{}, Tracks: []api.TrackRef{}}
	for _, a := range match(kArtist, len(ds.Artists), func(i int) string { return ds.Artists[i].Name }, b.artistPlays) {
		out.Artists = append(out.Artists, b.artistRef(a))
	}
	for _, a := range match(kAlbum, len(ds.Albums), func(i int) string { return ds.Albums[i].Name }, b.albumPlays) {
		out.Albums = append(out.Albums, b.albumRef(a))
	}
	for _, t := range match(kTrack, len(ds.Tracks), func(i int) string { return ds.Tracks[i].Name }, b.trackPlays) {
		out.Tracks = append(out.Tracks, b.trackRef(t))
	}
	return out, nil
}
