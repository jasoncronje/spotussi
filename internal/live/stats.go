package live

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/query"
)

// ---------- overview ----------

// summary measures plays. Its top artist and track carry their rank in
// prev (nil: no previous period).
func (x *q) summary(plays, prev []int32, metric query.Metric) api.Summary {
	var m measure
	for _, i := range plays {
		x.add(&m, i)
	}
	s := api.Summary{Plays: m.plays, MsPlayed: m.ms, MsEstimated: m.est, Skips: m.skips}
	var artist, track rankedItem
	var okA, okT bool
	tt := x.tracks(plays)
	artist, okA, s.Artists = x.firstOf(x.spread(tt, kArtist), kArtist, metric)
	_, _, s.Albums = x.firstOf(x.spread(tt, kAlbum), kAlbum, metric)
	track, okT, s.Tracks = x.firstOf(tt, kTrack, metric)
	if okA {
		r := x.rankedArtist(artist, 1, nil)
		if prev != nil {
			r.PrevRank = x.rankOf(prev, kArtist, metric, artist.key)
		}
		s.TopArtist = &r
	}
	if okT {
		r := x.rankedTrack(track, 1, nil)
		if prev != nil {
			r.PrevRank = x.rankOf(prev, kTrack, metric, track.key)
		}
		s.TopTrack = &r
	}
	return s
}

func (x *q) rankedArtist(it rankedItem, rank int, prev ranks) api.RankedArtist {
	return api.RankedArtist{Rank: rank, PrevRank: prevRank(prev, it.key), Plays: it.m.plays, MsPlayed: it.m.ms, Artist: x.artistRef(it.key)}
}

func (x *q) rankedTrack(it rankedItem, rank int, prev ranks) api.RankedTrack {
	return api.RankedTrack{Rank: rank, PrevRank: prevRank(prev, it.key), Plays: it.m.plays, MsPlayed: it.m.ms, Track: x.trackRef(it.key)}
}

func (b *Backend) OverviewSummary(ctx context.Context, spec query.Spec, metric query.Metric) (*api.OverviewSummary, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, prev, plays, prevPlays, err := x.rangePlays(spec, api.Scope{})
	if err != nil {
		return nil, err
	}
	out := &api.OverviewSummary{Range: wireRange(cur), Previous: wireRangePtr(prev), Metric: string(metric)}
	if prev != nil {
		ps := x.summary(prevPlays, nil, metric)
		out.PreviousSummary = &ps
		if prevPlays == nil {
			prevPlays = []int32{} // a previous period without plays still ranks nothing
		}
	}
	out.Current = x.summary(plays, prevPlays, metric)
	out.Sources, out.Catalog = x.mixes(plays)
	return out, nil
}

// mixes splits plays by source (D8) and by where their tracks' details
// come from (M7 provenance).
func (x *q) mixes(plays []int32) (api.SourceMix, api.CatalogMix) {
	var src, cat [3]measure
	const live = derive.FSrcPoller | derive.FSrcRecent
	for _, i := range plays {
		f := x.Flags[i]
		switch {
		case f&derive.FSrcExport != 0 && f&live != 0:
			x.add(&src[2], i)
		case f&derive.FSrcExport != 0:
			x.add(&src[0], i)
		default:
			x.add(&src[1], i)
		}
		x.add(&cat[x.e.Tracks[x.Track[i]].Meta], i)
	}
	m := func(v measure) api.Measure { return api.Measure{Plays: v.plays, MsPlayed: v.ms} }
	return api.SourceMix{Export: m(src[0]), Live: m(src[1]), Both: m(src[2])},
		api.CatalogMix{Pending: m(cat[derive.MetaPending]), Spotify: m(cat[derive.MetaSpotify]), Fallback: m(cat[derive.MetaFallback])}
}

// buckets assigns plays (sorted by start) to spans; fn gets each play's
// bucket index.
func (x *q) buckets(spans []query.Span, plays []int32, fn func(bucket int, i int32)) {
	k := 0
	for _, i := range plays {
		ms := x.Start[i]
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
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, prev, plays, prevPlays, err := x.rangePlays(spec, s)
	if err != nil {
		return nil, err
	}
	bk, spans, err := query.Buckets(cur, bucket, x.env.WeekStart)
	if err != nil {
		return nil, err
	}
	out := &api.Timeline{Range: wireRange(cur), Bucket: string(bk), Points: x.timelinePoints(spans, plays)}
	if compare {
		out.Compared = true
		if prev != nil {
			pspans, err := query.AlignedBuckets(*prev, bk, x.env.WeekStart, len(spans))
			if err != nil {
				return nil, err
			}
			out.Previous = wireRangePtr(prev)
			out.PreviousPoints = x.timelinePoints(pspans, prevPlays)
		}
	}
	return out, nil
}

// timelinePoints aggregates plays into spans; artists counts the distinct
// artists of counted plays per bucket.
func (x *q) timelinePoints(spans []query.Span, plays []int32) []api.TimelinePoint {
	ms := make([]measure, len(spans))
	artists := make([]int64, len(spans))
	lastBucket := make([]int32, len(x.e.Artists)) // bucket index + 1 an artist was last counted in
	f := x.Flat()
	counted, msPlayed, flags, track, start := x.Counted, x.Ms, x.Flags, x.Track, x.Start
	starts, ends := spanBounds(spans)
	k := 0
	for _, i := range plays {
		st := start[i]
		for k < len(spans) && st >= ends[k] {
			k++
		}
		if k == len(spans) {
			break
		}
		if st < starts[k] {
			continue
		}
		m := &ms[k]
		played := int64(msPlayed[i])
		m.ms += played
		fl := flags[i]
		if fl&derive.FEstimated != 0 {
			m.est += played
		}
		if fl&derive.FSkipped != 0 {
			m.skips++
		}
		if !counted[i] {
			continue
		}
		m.plays++
		t := track[i]
		for _, a := range f.Artists[f.ArtistFrom[t]:f.ArtistFrom[t+1]] {
			if lastBucket[a] != int32(k+1) {
				lastBucket[a] = int32(k + 1)
				artists[k]++
			}
		}
	}
	out := make([]api.TimelinePoint, len(spans))
	for k, sp := range spans {
		m := ms[k]
		out[k] = api.TimelinePoint{Key: sp.Key, StartAt: starts[k], Plays: m.plays, MsPlayed: m.ms,
			MsEstimated: m.est, Skips: m.skips, Artists: artists[k]}
	}
	return out
}

func spanBounds(spans []query.Span) (starts, ends []int64) {
	starts, ends = make([]int64, len(spans)), make([]int64, len(spans))
	for k, sp := range spans {
		starts[k], ends[k] = sp.Start.UnixMilli(), sp.End.UnixMilli()
	}
	return starts, ends
}

func (b *Backend) Clock(ctx context.Context, spec query.Spec, s api.Scope) (*api.Clock, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, _, plays, _, err := x.rangePlays(spec, s)
	if err != nil {
		return nil, err
	}
	var hours [24]measure
	for _, i := range plays {
		x.add(&hours[x.Hour[i]], i)
	}
	out := &api.Clock{Range: wireRange(cur), Hours: make([]api.ClockHour, 24)}
	for h, m := range hours {
		out.Hours[h] = api.ClockHour{Hour: h, Plays: m.plays, MsPlayed: m.ms}
	}
	return out, nil
}

// ---------- top lists ----------

func top[T any](b *Backend, ctx context.Context, p api.TopParams, k kind, wire func(x *q, it rankedItem, rank int, prev ranks) T) (*api.TopList[T], error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, prev, plays, prevPlays, err := x.rangePlays(p.Range, p.Scope)
	if err != nil {
		return nil, err
	}
	items := x.rank(plays, k, p.Metric)
	var prevRanks ranks
	if prev != nil {
		prevRanks = x.rankIndex(k, x.rank(prevPlays, k, p.Metric))
	}
	out := &api.TopList[T]{Range: wireRange(cur), Previous: wireRangePtr(prev), Metric: string(p.Metric),
		Total: len(items), Offset: p.Page.Offset, Limit: p.Page.Limit, Items: []T{}}
	for n, it := range page(items, p.Page) {
		out.Items = append(out.Items, wire(x, it, p.Page.Offset+n+1, prevRanks))
	}
	return out, nil
}

func (b *Backend) TopTracks(ctx context.Context, p api.TopParams) (*api.TopTracks, error) {
	return top(b, ctx, p, kTrack, (*q).rankedTrack)
}

func (b *Backend) TopArtists(ctx context.Context, p api.TopParams) (*api.TopArtists, error) {
	return top(b, ctx, p, kArtist, (*q).rankedArtist)
}

func (b *Backend) TopAlbums(ctx context.Context, p api.TopParams) (*api.TopAlbums, error) {
	return top(b, ctx, p, kAlbum, func(x *q, it rankedItem, rank int, prev ranks) api.RankedAlbum {
		return api.RankedAlbum{Rank: rank, PrevRank: prevRank(prev, it.key), Plays: it.m.plays, MsPlayed: it.m.ms, Album: x.albumRef(it.key)}
	})
}

func (b *Backend) TopGenres(ctx context.Context, p api.TopParams) (*api.TopGenres, error) {
	return top(b, ctx, p, kGenre, func(x *q, it rankedItem, rank int, prev ranks) api.RankedGenre {
		return api.RankedGenre{Rank: rank, PrevRank: prevRank(prev, it.key), Plays: it.m.plays, MsPlayed: it.m.ms, Genre: x.genreRef(it.key)}
	})
}

// ---------- stats ----------

func (b *Backend) HourlyTop(ctx context.Context, spec query.Spec, metric query.Metric) (*api.HourlyTop, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, _, plays, _, err := x.rangePlays(spec, api.Scope{})
	if err != nil {
		return nil, err
	}
	var byHour [24][]int32
	for _, i := range plays {
		byHour[x.Hour[i]] = append(byHour[x.Hour[i]], i)
	}
	out := &api.HourlyTop{Range: wireRange(cur), Metric: string(metric), Hours: make([]api.HourTop, 24)}
	// Hours are independent: rank them in parallel (each ranks a 24th of
	// the plays twice, which adds up for all-time ranges).
	parallel(24, func(h int) {
		hp := byHour[h]
		var m measure
		for _, i := range hp {
			x.add(&m, i)
		}
		ht := api.HourTop{Hour: h, Plays: m.plays, MsPlayed: m.ms}
		tt := x.tracks(hp)
		if a, ok, _ := x.firstOf(x.spread(tt, kArtist), kArtist, metric); ok {
			ht.TopArtist = &api.HourTopArtist{Plays: a.m.plays, MsPlayed: a.m.ms, Artist: x.artistRef(a.key)}
		}
		if t, ok, _ := x.firstOf(tt, kTrack, metric); ok {
			ht.TopTrack = &api.HourTopTrack{Plays: t.m.plays, MsPlayed: t.m.ms, Track: x.trackRef(t.key)}
		}
		out.Hours[h] = ht
	})
	return out, nil
}

// parallel runs fn(0..n-1) on up to GOMAXPROCS goroutines (at most 8).
func parallel(n int, fn func(int)) {
	workers := min(n, runtime.GOMAXPROCS(0), 8)
	var next atomic.Int32
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := int(next.Add(1)) - 1; i < n; i = int(next.Add(1)) - 1 {
				fn(i)
			}
		})
	}
	wg.Wait()
}

// breakdown computes per-bucket values of the range's top n entities of
// kind k. other = max(0, total - sum(values)): plays crediting several
// artists or genres count once per entity, so values can overlap.
func (x *q) breakdown(p api.SeriesParams, k kind) (query.Range, query.Bucket, []int32, []api.BreakdownPoint, error) {
	cur, _, plays, _, err := x.rangePlays(p.Range, api.Scope{})
	if err != nil {
		return cur, "", nil, nil, err
	}
	bk, spans, err := query.Buckets(cur, p.Bucket, x.env.WeekStart)
	if err != nil {
		return cur, "", nil, nil, err
	}
	items := x.rank(plays, k, p.Metric)
	series := make([]int32, 0, p.Series)
	slot := make([]int32, x.kindSize(k)) // series index + 1 (0: not a series)
	for _, it := range items[:min(len(items), p.Series)] {
		series = append(series, it.key)
		slot[it.key] = int32(len(series))
	}
	// The series each track is attributed to, flattened: track t's are
	// slots[from[t]:from[t+1]].
	from := make([]int32, len(x.e.Tracks)+1)
	var slots []int32
	var buf []int32
	for t := range x.e.Tracks {
		from[t] = int32(len(slots))
		switch k {
		case kArtist:
			buf = x.e.Tracks[t].Artists
		default:
			buf = x.e.Tracks[t].Genres
		}
		for _, key := range buf {
			if s := slot[key]; s > 0 {
				slots = append(slots, s-1)
			}
		}
	}
	from[len(x.e.Tracks)] = int32(len(slots))
	points := make([]api.BreakdownPoint, len(spans))
	for n, sp := range spans {
		points[n] = api.BreakdownPoint{Key: sp.Key, StartAt: sp.Start.UnixMilli(), Values: make([]int64, len(series))}
	}
	track := x.Track
	x.buckets(spans, plays, func(n int, i int32) {
		w := x.weight(i, p.Metric)
		pt := &points[n]
		pt.Total += w
		t := track[i]
		for _, s := range slots[from[t]:from[t+1]] {
			pt.Values[s] += w
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
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, bk, series, points, err := x.breakdown(p, kArtist)
	if err != nil {
		return nil, err
	}
	out := &api.ArtistsOverTime{Range: wireRange(cur), Metric: string(p.Metric), Bucket: string(bk), Series: []api.ArtistRef{}, Points: points}
	for _, a := range series {
		out.Series = append(out.Series, x.artistRef(a))
	}
	return out, nil
}

func (b *Backend) GenresOverTime(ctx context.Context, p api.SeriesParams) (*api.GenresOverTime, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, bk, series, points, err := x.breakdown(p, kGenre)
	if err != nil {
		return nil, err
	}
	out := &api.GenresOverTime{Range: wireRange(cur), Metric: string(p.Metric), Bucket: string(bk), Series: []api.GenreRef{}, Points: points}
	for _, g := range series {
		out.Series = append(out.Series, x.genreRef(g))
	}
	return out, nil
}

// average computes a metric-weighted average of attr per bucket; attr
// reports the value and whether it is known for a play.
func (b *Backend) average(ctx context.Context, p api.SeriesParams, attr func(x *q, i int32) (float64, bool)) (*api.AverageSeries, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, _, plays, _, err := x.rangePlays(p.Range, api.Scope{})
	if err != nil {
		return nil, err
	}
	bk, spans, err := query.Buckets(cur, p.Bucket, x.env.WeekStart)
	if err != nil {
		return nil, err
	}
	type acc struct{ sum, known, all float64 }
	accs := make([]acc, len(spans))
	var total acc
	x.buckets(spans, plays, func(n int, i int32) {
		w := float64(x.weight(i, p.Metric))
		accs[n].all += w
		total.all += w
		if v, ok := attr(x, i); ok {
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

// ReleaseYears and ArtistsPerTrack only use hydrated tracks: an export
// row names one artist and no release date, so unhydrated tracks would
// skew both averages.
func (b *Backend) ReleaseYears(ctx context.Context, p api.SeriesParams) (*api.AverageSeries, error) {
	return b.average(ctx, p, func(x *q, i int32) (float64, bool) {
		t := &x.e.Tracks[x.Track[i]]
		if !t.Hydrated || t.Album < 0 || x.e.Albums[t.Album].ReleaseYear == 0 {
			return 0, false
		}
		return float64(x.e.Albums[t.Album].ReleaseYear), true
	})
}

func (b *Backend) ArtistsPerTrack(ctx context.Context, p api.SeriesParams) (*api.AverageSeries, error) {
	return b.average(ctx, p, func(x *q, i int32) (float64, bool) {
		t := &x.e.Tracks[x.Track[i]]
		if !t.Hydrated {
			return 0, false
		}
		return float64(len(t.Artists)), true
	})
}
