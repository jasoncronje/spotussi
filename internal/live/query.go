package live

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/query"
)

// q is one request's view of the derived data: a snapshot (immutable)
// and the range environment.
type q struct {
	*derive.Snapshot
	e   *derive.Entities
	env query.Env
	// inCatalog reports whether the catalog table has a row with id: an
	// entity the snapshot lacks because it was saved but never played.
	inCatalog func(table, id string) (bool, error)
}

// snapshotWait bounds how long a request waits for the first snapshot
// after startup.
const snapshotWait = 30 * time.Second

func (b *Backend) q(ctx context.Context) (*q, error) {
	wctx, cancel := context.WithTimeout(ctx, snapshotWait)
	defer cancel()
	s, err := b.engine.Snapshot(wctx)
	if err != nil {
		return nil, err
	}
	set, err := b.loadSettings(wctx)
	if err != nil {
		return nil, err
	}
	ws := time.Monday
	if set.WeekStart == "sunday" {
		ws = time.Sunday
	}
	x := &q{Snapshot: s, e: s.Ent, env: query.Env{Now: b.now(), Loc: s.Settings.Loc, WeekStart: ws},
		inCatalog: func(table, id string) (bool, error) {
			var one int
			err := b.db.Read.QueryRowContext(ctx, `SELECT 1 FROM `+table+` WHERE id = ?`, id).Scan(&one)
			if err == sql.ErrNoRows {
				return false, nil
			}
			return err == nil, err
		}}
	if len(s.Vis) > 0 {
		x.env.FirstPlay = time.UnixMilli(s.Start[s.Vis[0]])
	}
	return x, nil
}

func (x *q) resolve(spec query.Spec) (query.Range, *query.Range, error) {
	return query.Resolve(spec, x.env)
}

func wireRange(r query.Range) api.ResolvedRange {
	out := api.ResolvedRange{Preset: string(r.Preset), From: r.From, To: r.To,
		StartAt: r.Start.UnixMilli(), EndAt: r.End.UnixMilli(), Timezone: r.Loc.String()}
	if d := r.FirstPlayDate(); d != "" {
		out.FirstPlayDate = &d
	}
	return out
}

func wireRangePtr(r *query.Range) *api.ResolvedRange {
	if r == nil {
		return nil
	}
	w := wireRange(*r)
	return &w
}

// between returns the visible plays starting in [startMs, endMs).
func (x *q) between(startMs, endMs int64) []int32 {
	start := x.Start
	vis := x.Vis
	lo := sort.Search(len(vis), func(i int) bool { return start[vis[i]] >= startMs })
	hi := sort.Search(len(vis), func(i int) bool { return start[vis[i]] >= endMs })
	return vis[lo:hi]
}

func (x *q) inRange(r query.Range) []int32 { return x.between(r.Start.UnixMilli(), r.End.UnixMilli()) }

// ---------- scopes ----------

// scope is a filter over plays by track; nil matches every play.
type scope []bool

func (s scope) has(x *q, i int32) bool { return s == nil || s[x.Track[i]] }

// scope resolves an entity scope to the tracks it covers.
func (x *q) scope(s api.Scope) (scope, error) {
	e := x.e
	mask := func(f func(t *derive.Track) bool) scope {
		m := make(scope, len(e.Tracks))
		for i := range e.Tracks {
			m[i] = f(&e.Tracks[i])
		}
		return m
	}
	switch {
	case s.TrackID != "":
		t, ok := e.TrackByID[s.TrackID]
		if !ok {
			return x.unplayed("tracks", s.TrackID)
		}
		m := make(scope, len(e.Tracks))
		m[t] = true
		return m, nil
	case s.AlbumID != "":
		al, ok := e.AlbumByID[s.AlbumID]
		if !ok {
			return x.unplayed("albums", s.AlbumID)
		}
		return mask(func(t *derive.Track) bool { return t.Album == al }), nil
	case s.GenreID != "":
		g, ok := e.GenreByID[s.GenreID]
		if !ok {
			return nil, fmt.Errorf("%w: genre %q", api.ErrNotFound, s.GenreID)
		}
		return mask(func(t *derive.Track) bool { return slices.Contains(t.Genres, g) }), nil
	case s.ArtistID != "":
		a, ok := e.ArtistByID[s.ArtistID]
		if !ok {
			return x.unplayed("artists", s.ArtistID)
		}
		return mask(func(t *derive.Track) bool { return slices.Contains(t.Artists, a) }), nil
	}
	return nil, nil
}

// unplayed resolves a scope on an entity the snapshot lacks: one the
// catalog has (saved, never played) matches no play, others are not found.
func (x *q) unplayed(table, id string) (scope, error) {
	ok, err := x.inCatalog(table, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s %q", api.ErrNotFound, strings.TrimSuffix(table, "s"), id)
	}
	return make(scope, len(x.e.Tracks)), nil
}

func (x *q) filter(plays []int32, s scope) []int32 {
	if s == nil {
		return plays
	}
	out := make([]int32, 0, len(plays)/4)
	for _, i := range plays {
		if s[x.Track[i]] {
			out = append(out, i)
		}
	}
	return out
}

// rangePlays resolves spec and returns the scoped plays of the range and
// of the previous period (nil if none).
func (x *q) rangePlays(spec query.Spec, s api.Scope) (query.Range, *query.Range, []int32, []int32, error) {
	cur, prev, err := x.resolve(spec)
	if err != nil {
		return cur, prev, nil, nil, err
	}
	sc, err := x.scope(s)
	if err != nil {
		return cur, prev, nil, nil, err
	}
	plays := x.filter(x.inRange(cur), sc)
	var prevPlays []int32
	if prev != nil {
		prevPlays = x.filter(x.inRange(*prev), sc)
	}
	return cur, prev, plays, prevPlays, nil
}

// ---------- measures ----------

type measure struct {
	plays, ms, est, skips int64
}

func (x *q) add(m *measure, i int32) {
	if x.Counted[i] {
		m.plays++
	}
	ms := int64(x.Ms[i])
	m.ms += ms
	f := x.Flags[i]
	if f&derive.FEstimated != 0 {
		m.est += ms
	}
	if f&derive.FSkipped != 0 {
		m.skips++
	}
}

func (m measure) value(metric query.Metric) int64 {
	if metric == query.MetricTime {
		return m.ms
	}
	return m.plays
}

// weight is the per-play weight under metric (weighted averages,
// breakdowns).
func (x *q) weight(i int32, metric query.Metric) int64 {
	if metric == query.MetricTime {
		return int64(x.Ms[i])
	}
	if x.Counted[i] {
		return 1
	}
	return 0
}

// ---------- ranking ----------

type kind int

const (
	kTrack kind = iota
	kArtist
	kAlbum
	kGenre
)

func (x *q) kindSize(k kind) int {
	switch k {
	case kTrack:
		return len(x.e.Tracks)
	case kArtist:
		return len(x.e.Artists)
	case kAlbum:
		return len(x.e.Albums)
	}
	return len(x.e.Genres)
}

func (x *q) kindID(k kind, key int32) string {
	switch k {
	case kTrack:
		return x.e.Tracks[key].ID
	case kArtist:
		return x.e.Artists[key].ID
	case kAlbum:
		return x.e.Albums[key].ID
	}
	return x.e.Genres[key].ID
}

type rankedItem struct {
	key int32
	m   measure
}

// tally is the per-entity accumulator of one ranking. Accumulators are
// as large as the catalog (40k tracks: ~1.5 MB), so they are pooled and
// only the touched entries are reset.
type tally struct {
	acc     []measure
	seen    []bool
	touched []int32
}

var tallies sync.Pool

func newTally(n int) *tally {
	t, _ := tallies.Get().(*tally)
	if t == nil || len(t.acc) < n {
		t = &tally{acc: make([]measure, n), seen: make([]bool, n)}
	}
	return t
}

func (t *tally) touch(key int32) *measure {
	if !t.seen[key] {
		t.seen[key] = true
		t.touched = append(t.touched, key)
	}
	return &t.acc[key]
}

// tracks tallies plays by track.
func (x *q) tracks(plays []int32) *tally {
	t := newTally(len(x.e.Tracks))
	counted, msPlayed, flags, track := x.Counted, x.Ms, x.Flags, x.Track
	for _, i := range plays {
		m := t.touch(track[i])
		if counted[i] {
			m.plays++
		}
		ms := int64(msPlayed[i])
		m.ms += ms
		f := flags[i]
		if f&derive.FEstimated != 0 {
			m.est += ms
		}
		if f&derive.FSkipped != 0 {
			m.skips++
		}
	}
	return t
}

// spread credits each track's measure to the entities of kind k it is
// attributed to (its album, every credited artist, or the union of
// their genres). Measures add up, so this equals tallying each play.
func (x *q) spread(tt *tally, k kind) *tally {
	t := newTally(x.kindSize(k))
	f := x.Flat()
	from, keys := f.ArtistFrom, f.Artists
	if k == kGenre {
		from, keys = f.GenreFrom, f.Genres
	}
	for _, tr := range tt.touched {
		m := tt.acc[tr]
		if k == kAlbum {
			if al := f.Album[tr]; al >= 0 {
				t.touch(al).plus(m)
			}
			continue
		}
		for _, key := range keys[from[tr]:from[tr+1]] {
			t.touch(key).plus(m)
		}
	}
	return t
}

func (m *measure) plus(o measure) {
	m.plays += o.plays
	m.ms += o.ms
	m.est += o.est
	m.skips += o.skips
}

func (x *q) tally(plays []int32, k kind) *tally {
	tt := x.tracks(plays)
	if k == kTrack {
		return tt
	}
	t := x.spread(tt, k)
	tt.release()
	return t
}

func (t *tally) release() {
	for _, key := range t.touched {
		t.acc[key], t.seen[key] = measure{}, false
	}
	t.touched = t.touched[:0]
	tallies.Put(t)
}

// order is the ranking order: metric, then the other measure, then ID.
func (x *q) order(k kind, metric query.Metric) func(a, c rankedItem) int {
	other := query.MetricTime
	if metric == query.MetricTime {
		other = query.MetricPlays
	}
	return func(a, c rankedItem) int {
		if r := cmp.Compare(c.m.value(metric), a.m.value(metric)); r != 0 {
			return r
		}
		if r := cmp.Compare(c.m.value(other), a.m.value(other)); r != 0 {
			return r
		}
		return cmp.Compare(x.kindID(k, a.key), x.kindID(k, c.key))
	}
}

// rank aggregates plays by kind and sorts by metric, then the other
// measure, then ID, dropping entities whose metric value is zero.
func (x *q) rank(plays []int32, k kind, metric query.Metric) []rankedItem {
	t := x.tally(plays, k)
	out := make([]rankedItem, 0, len(t.touched))
	for _, key := range t.touched {
		if t.acc[key].value(metric) > 0 {
			out = append(out, rankedItem{key, t.acc[key]})
		}
	}
	t.release()
	slices.SortFunc(out, x.order(k, metric))
	return out
}

// first returns the top entity of a ranking (without sorting it), and
// how many entities have at least one counted play.
func (x *q) first(plays []int32, k kind, metric query.Metric) (best rankedItem, ok bool, played int64) {
	return x.firstOf(x.tally(plays, k), k, metric)
}

// firstOf is first over a tally, which it releases.
func (x *q) firstOf(t *tally, k kind, metric query.Metric) (best rankedItem, ok bool, played int64) {
	less := x.order(k, metric)
	for _, key := range t.touched {
		m := t.acc[key]
		if m.plays > 0 {
			played++
		}
		if m.value(metric) <= 0 {
			continue
		}
		if it := (rankedItem{key, m}); !ok || less(it, best) < 0 {
			best, ok = it, true
		}
	}
	t.release()
	return best, ok, played
}

// rankOf is key's rank in plays (nil if it has none there), without
// sorting the ranking.
func (x *q) rankOf(plays []int32, k kind, metric query.Metric, key int32) *int {
	t := x.tally(plays, k)
	defer t.release()
	if !t.seen[key] || t.acc[key].value(metric) <= 0 {
		return nil
	}
	less := x.order(k, metric)
	it := rankedItem{key, t.acc[key]}
	r := 1
	for _, o := range t.touched {
		if o != key && t.acc[o].value(metric) > 0 && less(rankedItem{o, t.acc[o]}, it) < 0 {
			r++
		}
	}
	return &r
}

// ranks maps entities to their 1-based rank (0: unranked).
type ranks []int32

func (x *q) rankIndex(k kind, items []rankedItem) ranks {
	r := make(ranks, x.kindSize(k))
	for i, it := range items {
		r[it.key] = int32(i + 1)
	}
	return r
}

func prevRank(prev ranks, key int32) *int {
	if prev == nil || prev[key] == 0 {
		return nil
	}
	return ptr(int(prev[key]))
}

func page[T any](items []T, p api.Page) []T {
	if p.Offset >= len(items) {
		return []T{}
	}
	return items[p.Offset:min(len(items), p.Offset+p.Limit)]
}

// ---------- wire references ----------

func ptr[T any](v T) *T { return &v }

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (x *q) artistLite(a int32) api.ArtistLite {
	r := &x.e.Artists[a]
	return api.ArtistLite{ID: r.ID, Name: r.Name}
}

func (x *q) artistLites(as []int32) []api.ArtistLite {
	out := make([]api.ArtistLite, len(as))
	for i, a := range as {
		out[i] = x.artistLite(a)
	}
	return out
}

func (x *q) artistRef(a int32) api.ArtistRef {
	r := &x.e.Artists[a]
	return api.ArtistRef{ID: r.ID, Name: r.Name, ImageURL: strPtr(r.ImageURL)}
}

func (x *q) albumRef(al int32) api.AlbumRef {
	r := &x.e.Albums[al]
	out := api.AlbumRef{ID: r.ID, Name: r.Name, ImageURL: strPtr(r.ImageURL), Artists: x.artistLites(r.Artists)}
	if r.ReleaseYear > 0 {
		out.ReleaseYear = ptr(r.ReleaseYear)
	}
	return out
}

func (x *q) trackRef(t int32) api.TrackRef {
	r := &x.e.Tracks[t]
	out := api.TrackRef{ID: r.ID, Name: r.Name, Artists: x.artistLites(r.Artists)}
	if r.Album >= 0 {
		al := &x.e.Albums[r.Album]
		out.Album = &api.AlbumLite{ID: al.ID, Name: al.Name}
		out.ImageURL = strPtr(al.ImageURL)
	}
	if r.DurationMs > 0 {
		out.DurationMs = ptr(r.DurationMs)
	}
	return out
}

func (x *q) genreRef(g int32) api.GenreRef {
	return api.GenreRef{ID: x.e.Genres[g].ID, Name: x.e.Genres[g].Name}
}

func (x *q) contextRef(c int32) api.ContextRef {
	return contextRefOf(&x.e.Contexts[c])
}

func contextRefOf(c *derive.Context) api.ContextRef {
	r := api.ContextRef{URI: c.URI, Type: c.Type, ID: c.ID, NameSource: c.NameSource, SpotifyOwned: c.SpotifyOwned,
		ImageURL: strPtr(c.ImageURL)}
	if c.HasName {
		r.Name = ptr(c.Name)
	}
	return r
}

func (x *q) wirePlay(i int32) api.Play {
	f := x.Flags[i]
	w := api.Play{
		ID: fmt.Sprint(x.ID[i]), PlayedAt: x.Start[i], EndedAt: x.End(i), MsPlayed: int64(x.Ms[i]),
		MsEstimated: f&derive.FEstimated != 0, CountsAsPlay: x.Counted[i], Skipped: f&derive.FSkipped != 0,
		Contradicted: f&derive.FContradicted != 0, Track: x.trackRef(x.Track[i]), Sources: []string{},
	}
	if f&derive.FShuffleKnown != 0 {
		w.Shuffle = ptr(f&derive.FShuffle != 0)
	}
	if p := x.Platform[i]; p >= 0 {
		w.Platform = ptr(x.e.Platforms[p])
	}
	if d := x.Device[i]; d >= 0 {
		w.Device = &api.Device{Name: x.e.Devices[d].Name, Type: x.e.Devices[d].Type}
	}
	if c := x.Ctx[i]; c >= 0 {
		r := x.contextRef(c)
		w.Context = &r
	}
	for _, s := range []struct {
		f    uint16
		name string
	}{{derive.FSrcExport, "export"}, {derive.FSrcPoller, "poller"}, {derive.FSrcRecent, "recent"}} {
		if f&s.f != 0 {
			w.Sources = append(w.Sources, s.name)
		}
	}
	return w
}

func (x *q) dataStatus() api.DataStatus {
	var d api.DataStatus
	d.Rows = int64(len(x.Vis))
	for _, i := range x.Vis {
		if x.Counted[i] {
			d.Plays++
		}
	}
	if n := len(x.Vis); n > 0 {
		d.FirstPlayAt = ptr(x.Start[x.Vis[0]])
		d.LastPlayAt = ptr(x.Start[x.Vis[n-1]])
	}
	return d
}
