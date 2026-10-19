// Package mock implements api.Backend over an in-memory refdata.Dataset by
// naive aggregation: every request scans the plays it needs. There are no
// rollups or caches beyond per-play derived fields, so any range and
// parameter combination works and the code stays obviously correct.
//
// This is deliberately the correctness oracle for the M5 rollup
// implementation: prefer clarity over speed here. The play rule lives only
// in internal/derive.
package mock

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/query"
	"spotussi/internal/refdata"
)

// ImagePath is where generated placeholder artwork is served.
const ImagePath = "/_mock/img/"

// Backend serves the mock API.
type Backend struct {
	ds    *refdata.Dataset
	clock func() time.Time

	mu       sync.RWMutex
	settings api.Settings
	loc      *time.Location
	rule     derive.Rule

	exclArtists map[int32]bool
	exclTracks  map[int32]bool
	exclOrder   []exclusion // insertion order for stable listing

	ctxNames map[int32]*string // manual playlist names (nil value = cleared)

	imports mockImports // simulated uploads (own lock)

	// Derived per play, rebuilt when settings or exclusions change.
	counted []bool  // passes the play rule
	hour    []uint8 // local hour of the start
	vis     []int32 // indices of non-excluded plays, sorted by start
	// All-time counted plays per entity (search ordering).
	trackPlays, artistPlays, albumPlays []int64
	sessions                            []session
	sessionByID                         map[string]int

	lastRebuildAt, lastRebuildMs int64

	byID struct {
		artist, album, track, playlist, genre map[string]int32
	}
	// Where each entity's details come from (M7 provenance); a scenario
	// (scenario.go) can thin the catalog out.
	trackMeta, artistMeta, albumMeta []derive.Meta

	scenario Scenario
}

type exclusion struct {
	kind api.ExclusionKind
	idx  int32
}

// New builds a backend over ds with default settings in the dataset's
// timezone.
func New(ds *refdata.Dataset) *Backend {
	return NewWithClock(ds, time.Now)
}

// NewWithClock is New with an injectable clock (tests pin "now" to ds.End).
func NewWithClock(ds *refdata.Dataset, clock func() time.Time) *Backend {
	b := &Backend{
		ds:          ds,
		clock:       clock,
		settings:    api.DefaultSettings(ds.Timezone),
		exclArtists: map[int32]bool{},
		exclTracks:  map[int32]bool{},
		ctxNames:    map[int32]*string{},
	}
	b.byID.artist = map[string]int32{}
	b.byID.album = map[string]int32{}
	b.byID.track = map[string]int32{}
	b.byID.playlist = map[string]int32{}
	b.byID.genre = map[string]int32{}
	for i, g := range ds.Genres {
		b.byID.genre[g.ID] = int32(i)
	}
	for i, a := range ds.Artists {
		b.byID.artist[a.ID] = int32(i)
	}
	for i, a := range ds.Albums {
		b.byID.album[a.ID] = int32(i)
	}
	for i, t := range ds.Tracks {
		b.byID.track[t.ID] = int32(i)
	}
	b.applyCatalog()
	for i, c := range ds.Contexts {
		if c.Type == "playlist" {
			b.byID.playlist[c.ID] = int32(i)
		}
	}
	b.applySettings()
	return b
}

// applySettings recomputes everything derived from settings and exclusions.
// Callers hold the write lock (or own b exclusively).
func (b *Backend) applySettings() {
	start := time.Now()
	s := b.settings
	b.loc, _ = time.LoadLocation(s.Timezone)
	if b.loc == nil {
		b.loc = time.UTC
	}
	b.rule = derive.Rule{Kind: derive.RuleKind(s.PlayRule), ThresholdMs: s.PlayRuleThresholdMs}

	ds := b.ds
	n := len(ds.Plays)
	b.counted = make([]bool, n)
	b.hour = make([]uint8, n)
	b.vis = b.vis[:0]
	b.trackPlays = make([]int64, len(ds.Tracks))
	b.artistPlays = make([]int64, len(ds.Artists))
	b.albumPlays = make([]int64, len(ds.Albums))
	for i, p := range ds.Plays {
		b.counted[i] = b.rule.CountsAsPlay(b.ruleRow(p))
		b.hour[i] = uint8(time.UnixMilli(p.StartMs).In(b.loc).Hour())
		if b.excluded(p) {
			continue
		}
		b.vis = append(b.vis, int32(i))
		if b.counted[i] {
			t := ds.Tracks[p.Track]
			b.trackPlays[p.Track]++
			b.albumPlays[t.Album]++
			for _, a := range t.Artists {
				b.artistPlays[a]++
			}
		}
	}
	b.buildSessions()
	b.lastRebuildAt = time.Now().UnixMilli()
	b.lastRebuildMs = time.Since(start).Milliseconds()
}

// ruleRow is the only place play fields are mapped to the play rule input.
func (b *Backend) ruleRow(p refdata.Play) derive.Row {
	t := b.ds.Tracks[p.Track]
	row := derive.Row{MsPlayed: int64(p.MsPlayed), RecentOnly: p.Flags&(refdata.FlagSrcExport|refdata.FlagSrcPoller) == 0}
	if b.trackMeta[p.Track] != derive.MetaPending {
		row.DurationMs = int64(t.DurationMs)
	}
	return row
}

func (b *Backend) excluded(p refdata.Play) bool {
	if b.exclTracks[p.Track] {
		return true
	}
	for _, a := range b.ds.Tracks[p.Track].Artists {
		if b.exclArtists[a] {
			return true
		}
	}
	return false
}

// ---------- ranges ----------

func (b *Backend) env() query.Env {
	ws := time.Monday
	if b.settings.WeekStart == "sunday" {
		ws = time.Sunday
	}
	var first time.Time
	if len(b.vis) > 0 {
		first = time.UnixMilli(b.ds.Plays[b.vis[0]].StartMs)
	}
	return query.Env{Now: b.clock(), Loc: b.loc, WeekStart: ws, FirstPlay: first}
}

func (b *Backend) weekStart() time.Weekday { return b.env().WeekStart }

func (b *Backend) resolve(spec query.Spec) (query.Range, *query.Range, error) {
	return query.Resolve(spec, b.env())
}

func wireRange(r query.Range) api.ResolvedRange {
	w := api.ResolvedRange{Preset: string(r.Preset), From: r.From, To: r.To,
		StartAt: r.Start.UnixMilli(), EndAt: r.End.UnixMilli(), Timezone: r.Loc.String()}
	if d := r.FirstPlayDate(); d != "" {
		w.FirstPlayDate = &d
	}
	return w
}

func wireRangePtr(r *query.Range) *api.ResolvedRange {
	if r == nil {
		return nil
	}
	w := wireRange(*r)
	return &w
}

// inRange returns the visible plays starting in [r.Start, r.End), in order.
func (b *Backend) inRange(r query.Range) []int32 {
	return b.between(r.Start.UnixMilli(), r.End.UnixMilli())
}

func (b *Backend) between(startMs, endMs int64) []int32 {
	plays := b.ds.Plays
	lo := searchVis(b.vis, plays, startMs)
	hi := searchVis(b.vis, plays, endMs)
	return b.vis[lo:hi]
}

func searchVis(vis []int32, plays []refdata.Play, ms int64) int {
	lo, hi := 0, len(vis)
	for lo < hi {
		m := (lo + hi) / 2
		if plays[vis[m]].StartMs < ms {
			lo = m + 1
		} else {
			hi = m
		}
	}
	return lo
}

// ---------- scopes ----------

type scopeFn func(p refdata.Play) bool

func (b *Backend) scope(s api.Scope) (scopeFn, error) {
	switch {
	case s.TrackID != "":
		t, ok := b.byID.track[s.TrackID]
		if !ok {
			return nil, fmt.Errorf("%w: track %q", api.ErrNotFound, s.TrackID)
		}
		return func(p refdata.Play) bool { return p.Track == t }, nil
	case s.AlbumID != "":
		al, ok := b.byID.album[s.AlbumID]
		if !ok {
			return nil, fmt.Errorf("%w: album %q", api.ErrNotFound, s.AlbumID)
		}
		return func(p refdata.Play) bool { return b.ds.Tracks[p.Track].Album == al }, nil
	case s.GenreID != "":
		g, ok := b.byID.genre[s.GenreID]
		if !ok {
			return nil, fmt.Errorf("%w: genre %q", api.ErrNotFound, s.GenreID)
		}
		return func(p refdata.Play) bool {
			for _, a := range b.ds.Tracks[p.Track].Artists {
				if slices.Contains(b.genres(a), g) {
					return true
				}
			}
			return false
		}, nil
	case s.ArtistID != "":
		a, ok := b.byID.artist[s.ArtistID]
		if !ok {
			return nil, fmt.Errorf("%w: artist %q", api.ErrNotFound, s.ArtistID)
		}
		return func(p refdata.Play) bool {
			for _, x := range b.ds.Tracks[p.Track].Artists {
				if x == a {
					return true
				}
			}
			return false
		}, nil
	}
	return func(refdata.Play) bool { return true }, nil
}

// ---------- measures ----------

type measure struct {
	plays, ms, est, skips int64
}

func (b *Backend) add(m *measure, i int32) {
	p := b.ds.Plays[i]
	if b.counted[i] {
		m.plays++
	}
	m.ms += int64(p.MsPlayed)
	if p.Has(refdata.FlagEstimated) {
		m.est += int64(p.MsPlayed)
	}
	if p.Has(refdata.FlagSkipped) {
		m.skips++
	}
}

// value is the ranking value of m under metric.
func (m measure) value(metric query.Metric) int64 {
	if metric == query.MetricTime {
		return m.ms
	}
	return m.plays
}

// weight is the per-play weight under metric (for weighted averages).
func (b *Backend) weight(i int32, metric query.Metric) int64 {
	if metric == query.MetricTime {
		return int64(b.ds.Plays[i].MsPlayed)
	}
	if b.counted[i] {
		return 1
	}
	return 0
}

// ---------- wire references ----------

func ptr[T any](v T) *T { return &v }

func imageURL(kind, id string) *string { return ptr(ImagePath + kind + "/" + id + ".svg") }

func (b *Backend) artistLite(a int32) api.ArtistLite {
	x := b.ds.Artists[a]
	return api.ArtistLite{ID: x.ID, Name: x.Name}
}

func (b *Backend) artistLites(as []int32) []api.ArtistLite {
	out := make([]api.ArtistLite, len(as))
	for i, a := range as {
		out[i] = b.artistLite(a)
	}
	return out
}

func (b *Backend) artistRef(a int32) api.ArtistRef {
	x := b.ds.Artists[a]
	r := api.ArtistRef{ID: x.ID, Name: x.Name}
	if b.artistMeta[a] != derive.MetaPending {
		r.ImageURL = imageURL("artist", x.ID)
	}
	return r
}

func (b *Backend) albumImage(al int32) *string {
	if b.albumMeta[al] == derive.MetaPending {
		return nil
	}
	return imageURL("album", b.ds.Albums[al].ID)
}

func (b *Backend) albumRef(al int32) api.AlbumRef {
	x := b.ds.Albums[al]
	return api.AlbumRef{ID: x.ID, Name: x.Name, ImageURL: b.albumImage(al),
		ReleaseYear: b.releaseYear(al), Artists: b.artistLites(x.Artists)}
}

func (b *Backend) releaseYear(al int32) *int {
	if b.albumMeta[al] == derive.MetaPending {
		return nil
	}
	return ptr(b.ds.Albums[al].ReleaseYear)
}

func (b *Backend) trackRef(t int32) api.TrackRef {
	x := b.ds.Tracks[t]
	al := b.ds.Albums[x.Album]
	r := api.TrackRef{ID: x.ID, Name: x.Name, Artists: b.artistLites(x.Artists),
		Album: &api.AlbumLite{ID: al.ID, Name: al.Name}, ImageURL: b.albumImage(x.Album)}
	if b.trackMeta[t] != derive.MetaPending {
		r.DurationMs = ptr(int64(x.DurationMs))
	}
	return r
}

func (b *Backend) genreRef(g int32) api.GenreRef {
	x := b.ds.Genres[g]
	return api.GenreRef{ID: x.ID, Name: x.Name}
}

// genres returns the genres of an artist, empty until hydrated.
func (b *Backend) genres(a int32) []int32 {
	if b.artistMeta[a] == derive.MetaPending {
		return nil
	}
	return b.ds.Artists[a].Genres
}

func (b *Backend) contextRef(c int32) api.ContextRef {
	x := b.ds.Contexts[c]
	r := api.ContextRef{URI: x.URI, Type: x.Type, ID: x.ID, NameSource: x.NameSource, SpotifyOwned: x.SpotifyOwned}
	if x.Name != "" {
		r.Name = ptr(x.Name)
	}
	if name, ok := b.ctxNames[c]; ok {
		if name == nil {
			r.Name, r.NameSource = nil, "unresolved"
		} else {
			r.Name, r.NameSource = ptr(*name), "manual"
		}
	}
	switch x.Type {
	case "album":
		r.ImageURL = b.albumImage(x.Ref)
	case "artist":
		r.ImageURL = b.artistRef(x.Ref).ImageURL
	case "playlist":
		if !x.SpotifyOwned {
			r.ImageURL = imageURL("playlist", x.ID)
		}
	}
	return r
}

func (b *Backend) playID(i int32) string { return strconv.Itoa(int(i) + 1) }

func (b *Backend) parsePlayID(id string) (int32, bool) {
	n, err := strconv.Atoi(id)
	if err != nil || n < 1 || n > len(b.ds.Plays) {
		return 0, false
	}
	return int32(n - 1), true
}

func (b *Backend) wirePlay(i int32) api.Play {
	p := b.ds.Plays[i]
	w := api.Play{
		ID: b.playID(i), PlayedAt: p.StartMs, EndedAt: p.EndMs(), MsPlayed: int64(p.MsPlayed),
		MsEstimated: p.Has(refdata.FlagEstimated), CountsAsPlay: b.counted[i],
		Skipped: p.Has(refdata.FlagSkipped), Contradicted: p.Has(refdata.FlagContradicted),
		Track: b.trackRef(p.Track),
	}
	if p.Has(refdata.FlagShuffleKnown) {
		w.Shuffle = ptr(p.Has(refdata.FlagShuffle))
	}
	if p.Platform >= 0 {
		w.Platform = ptr(b.ds.Platforms[p.Platform])
	}
	if p.Device >= 0 {
		d := b.ds.Devices[p.Device]
		w.Device = &api.Device{Name: d.Name, Type: d.Type}
	}
	if p.Context >= 0 {
		c := b.contextRef(p.Context)
		w.Context = &c
	}
	for _, s := range []struct {
		f    uint8
		name string
	}{{refdata.FlagSrcExport, "export"}, {refdata.FlagSrcPoller, "poller"}, {refdata.FlagSrcRecent, "recent"}} {
		if p.Has(s.f) {
			w.Sources = append(w.Sources, s.name)
		}
	}
	return w
}

// ---------- system ----------

func (b *Backend) SystemStatus(ctx context.Context) (*api.SystemStatus, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ds := b.ds
	now := b.clock().UnixMilli()
	var tracksHyd, artistsHyd, ctxResolved int64
	for _, m := range b.trackMeta {
		if m == derive.MetaSpotify {
			tracksHyd++
		}
	}
	for _, m := range b.artistMeta {
		if m == derive.MetaSpotify {
			artistsHyd++
		}
	}
	for i := range ds.Contexts {
		if b.contextRef(int32(i)).Name != nil {
			ctxResolved++
		}
	}
	var plays int64
	for _, i := range b.vis {
		if b.counted[i] {
			plays++
		}
	}
	st := &api.SystemStatus{
		Spotify: api.SpotifyStatus{State: "connected", AccountName: ptr("listener"), ConnectedAt: ptr(ds.PollerSince)},
		Capture: api.CaptureStatus{State: "playing", LastPollAt: ptr(now - 4_000), LastPlayAt: nil},
		Imports: b.importList(5),
		Catalog: mockCatalog(now, int64(len(ds.Tracks)), tracksHyd, int64(len(ds.Albums)), int64(len(ds.Artists)), artistsHyd,
			int64(len(ds.Contexts)), ctxResolved),
		Derive: api.DeriveStatus{State: "idle", LastRebuildAt: ptr(b.lastRebuildAt), LastRebuildMs: ptr(b.lastRebuildMs)},
		Data:   api.DataStatus{Rows: int64(len(b.vis)), Plays: plays},
	}
	if n := len(b.vis); n > 0 {
		st.Data.FirstPlayAt = ptr(ds.Plays[b.vis[0]].StartMs)
		last := ds.Plays[b.vis[n-1]].StartMs
		st.Data.LastPlayAt = &last
		st.Capture.LastPlayAt = &last
	}
	switch b.scenario.Spotify {
	case "reconnect_required":
		st.Spotify.State, st.Capture.State = "reconnect_required", "stopped"
		st.Capture.LastError, st.Capture.LastErrorAt = ptr("Spotify revoked access (invalid_grant)."), ptr(now-3_600_000)
		st.Catalog.State = "waiting"
	case "not_configured":
		st.Spotify = api.SpotifyStatus{State: "not_configured"}
		st.Capture.State, st.Capture.LastPollAt = "stopped", nil
		st.Catalog.State = "waiting"
	}
	if b.scenario.Catalog == "sparse" {
		var fbTracks, pendTracks, fbArtists int64
		for _, m := range b.trackMeta {
			switch m {
			case derive.MetaFallback:
				fbTracks++
			case derive.MetaPending:
				pendTracks++
			}
		}
		for _, m := range b.artistMeta {
			if m == derive.MetaFallback {
				fbArtists++
			}
		}
		c := &st.Catalog
		c.State, c.ResumesAt, c.EtaAt, c.RatePerSec = "quota_exceeded", ptr(now+9*3_600_000), ptr(now+40*86_400_000), nil
		c.QuotaLookups = ptr(int64(150))
		rate := 1.9
		c.Fallbacks = []api.CatalogFallback{
			{Source: "oembed", State: "running", Matched: fbTracks, Pending: pendTracks, ArtistsMatched: fbArtists, RatePerSec: &rate},
			{Source: "deezer", State: "running", Matched: fbTracks * 9 / 10, Unmatched: fbTracks / 10, Pending: pendTracks, ArtistsMatched: fbArtists, RatePerSec: &rate},
			{Source: "listenbrainz", State: "idle", Matched: fbTracks * 6 / 10, Unmatched: fbTracks * 4 / 10, ArtistsMatched: fbArtists / 2},
			{Source: "musicbrainz", State: "running", Matched: fbArtists * 2 / 3, Pending: pendTracks, ArtistsMatched: fbArtists / 6},
		}
	}
	return st, nil
}

// mockCatalog describes hydration in progress at the planning rate of 2
// lookups per second (D9).
func mockCatalog(now, tracks, tracksHyd, albums, artists, artistsHyd, contexts, ctxResolved int64) api.CatalogStatus {
	c := api.CatalogStatus{State: "idle", TracksTotal: tracks, TracksHydrated: tracksHyd, AlbumsTotal: albums,
		AlbumsHydrated: albums, ArtistsTotal: artists, ArtistsHydrated: artistsHyd, ContextsTotal: contexts,
		ContextsResolved: ctxResolved,
		Queue:            api.CatalogQueueCounts{Track: tracks - tracksHyd, Artist: artists - artistsHyd}}
	c.QueueLength = c.Queue.Track + c.Queue.Artist
	for _, src := range []string{"oembed", "deezer", "listenbrainz", "musicbrainz"} {
		c.Fallbacks = append(c.Fallbacks, api.CatalogFallback{Source: src, State: "paused", Pending: c.Queue.Track})
	}
	if c.QueueLength > 0 {
		rate, at, last := 2.0, now, now-400
		c.State, c.RatePerSec, c.LastHydratedAt = "hydrating", &rate, &last
		at += c.QueueLength * 1000 / 2
		c.EtaAt = &at
	}
	return c
}

func (b *Backend) NowPlaying(ctx context.Context) (*api.NowPlaying, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	np := b.ds.Now
	now := b.clock().UnixMilli()
	t := b.ds.Tracks[np.Track]
	dur := int64(t.DurationMs)
	progress := (now - np.StartedMs) % dur
	if progress < 0 {
		progress += dur
	}
	tr := b.trackRef(np.Track)
	d := b.ds.Devices[np.Device]
	out := &api.NowPlaying{
		IsPlaying: true, Track: &tr, ProgressMs: &progress, StartedAt: ptr(now - progress),
		Device: &api.Device{Name: d.Name, Type: d.Type}, Shuffle: ptr(np.Shuffle), ObservedAt: ptr(now),
	}
	if np.Context >= 0 {
		c := b.contextRef(np.Context)
		out.Context = &c
	}
	return out, nil
}

// ---------- settings ----------

func (b *Backend) Settings(ctx context.Context) (*api.Settings, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s := b.settings
	return &s, nil
}

func (b *Backend) UpdateSettings(ctx context.Context, p api.SettingsPatch) (*api.Settings, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, err := b.settings.Apply(p)
	if err != nil {
		return nil, err
	}
	rebuild := s.PlayRule != b.settings.PlayRule || s.PlayRuleThresholdMs != b.settings.PlayRuleThresholdMs ||
		s.Timezone != b.settings.Timezone || s.SessionGapMinutes != b.settings.SessionGapMinutes
	b.settings = s
	if rebuild {
		b.applySettings()
	}
	return &s, nil
}

func (b *Backend) exclusionsLocked() *api.Exclusions {
	out := &api.Exclusions{Artists: []api.ArtistRef{}, Tracks: []api.TrackRef{}}
	for _, e := range b.exclOrder {
		if e.kind == api.ExcludeArtists {
			out.Artists = append(out.Artists, b.artistRef(e.idx))
		} else {
			out.Tracks = append(out.Tracks, b.trackRef(e.idx))
		}
	}
	return out
}

func (b *Backend) Exclusions(ctx context.Context) (*api.Exclusions, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.exclusionsLocked(), nil
}

func (b *Backend) lookupExclusion(kind api.ExclusionKind, id string) (int32, map[int32]bool, error) {
	if kind == api.ExcludeArtists {
		i, ok := b.byID.artist[id]
		if !ok {
			return 0, nil, fmt.Errorf("%w: artist %q", api.ErrNotFound, id)
		}
		return i, b.exclArtists, nil
	}
	i, ok := b.byID.track[id]
	if !ok {
		return 0, nil, fmt.Errorf("%w: track %q", api.ErrNotFound, id)
	}
	return i, b.exclTracks, nil
}

func (b *Backend) AddExclusion(ctx context.Context, kind api.ExclusionKind, id string) (*api.Exclusions, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	i, set, err := b.lookupExclusion(kind, id)
	if err != nil {
		return nil, err
	}
	if !set[i] {
		set[i] = true
		b.exclOrder = append(b.exclOrder, exclusion{kind, i})
		b.applySettings()
	}
	return b.exclusionsLocked(), nil
}

func (b *Backend) RemoveExclusion(ctx context.Context, kind api.ExclusionKind, id string) (*api.Exclusions, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	i, set, err := b.lookupExclusion(kind, id)
	if err != nil {
		return b.exclusionsLocked(), nil // idempotent: unknown ids are simply not excluded
	}
	if set[i] {
		delete(set, i)
		for k, e := range b.exclOrder {
			if e.kind == kind && e.idx == i {
				b.exclOrder = append(b.exclOrder[:k], b.exclOrder[k+1:]...)
				break
			}
		}
		b.applySettings()
	}
	return b.exclusionsLocked(), nil
}

func (b *Backend) SetPlaylistName(ctx context.Context, id string, name *string) (*api.ContextRef, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.byID.playlist[id]
	if !ok {
		return nil, fmt.Errorf("%w: playlist %q", api.ErrNotFound, id)
	}
	if !b.ds.Contexts[c].SpotifyOwned {
		return nil, fmt.Errorf("%w: only Spotify-owned playlists can be named manually", api.ErrBadRequest)
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return nil, fmt.Errorf("%w: name must not be blank", api.ErrBadRequest)
		}
		name = &n
	}
	b.ctxNames[c] = name
	r := b.contextRef(c)
	return &r, nil
}
