package derive

import (
	"slices"
	"sort"
	"sync"
	"time"
)

// A Snapshot is the derived data the query layer reads: every play in
// columnar form with its resolved entities (Base), plus everything that
// depends on settings and exclusions (View). Snapshots are immutable and
// swapped atomically, so readers never see a half-built state.
//
// Why in memory rather than SQL rollups (D24): ranking 40k tracks over 15
// years from per-day rollups takes ~300 ms in SQLite (modernc), while a
// scan of 500k plays held in arrays takes about a millisecond, so every
// range and scope combination meets the 25 ms budget without a rollup
// per query shape. The plays and sessions tables stay the durable,
// SQL-readable derived layer, and are what a restart loads.
type Snapshot struct {
	*Base
	*View
	BuiltAt time.Time

	flatOnce sync.Once
	flat     Flat
}

// Flat holds per-track attributions in flat arrays, for loops over
// plays: a Track is large, and reading its slices per play misses the
// cache. Track t's artists are Artists[ArtistFrom[t]:ArtistFrom[t+1]],
// likewise genres; Album[t] is -1 when unknown.
type Flat struct {
	ArtistFrom, Artists []int32
	GenreFrom, Genres   []int32
	Album               []int32
}

// Flat builds (once) and returns the snapshot's flat attributions.
func (s *Snapshot) Flat() *Flat {
	s.flatOnce.Do(func() {
		ts := s.Ent.Tracks
		f := &s.flat
		f.ArtistFrom, f.GenreFrom = make([]int32, len(ts)+1), make([]int32, len(ts)+1)
		f.Album = make([]int32, len(ts))
		for t := range ts {
			f.ArtistFrom[t], f.GenreFrom[t] = int32(len(f.Artists)), int32(len(f.Genres))
			f.Artists = append(f.Artists, ts[t].Artists...)
			f.Genres = append(f.Genres, ts[t].Genres...)
			f.Album[t] = ts[t].Album
		}
		f.ArtistFrom[len(ts)], f.GenreFrom[len(ts)] = int32(len(f.Artists)), int32(len(f.Genres))
	})
	return &s.flat
}

// Play flags in Base.Flags.
const (
	FSkipped uint16 = 1 << iota
	FShuffleKnown
	FShuffle
	FEstimated
	FContradicted
	FSrcExport
	FSrcPoller
	FSrcRecent
)

// Base holds every merged play, sorted by start, column by column.
type Base struct {
	ID       []int64
	Start    []int64
	Wall     []int32 // end - start
	Ms       []int32 // ms played
	Track    []int32
	Ctx      []int32 // -1: unknown
	Device   []int16 // -1: unknown
	Platform []int16 // -1: unknown
	Flags    []uint16

	Ent *Entities
	// ExportSpan is what Merge was given.
	ExportSpan [2]int64
	// MaxID is the highest play ID (new plays append above it).
	MaxID int64
}

// Len is the number of plays.
func (b *Base) Len() int { return len(b.ID) }

// End is when play i ended.
func (b *Base) End(i int32) int64 { return b.Start[i] + int64(b.Wall[i]) }

// Has reports whether play i has every flag in f.
func (b *Base) Has(i int32, f uint16) bool { return b.Flags[i]&f == f }

// SourceFlags maps Play.Sources to flags.
func sourceFlags(s uint8) uint16 {
	var f uint16
	if s&SrcExport != 0 {
		f |= FSrcExport
	}
	if s&SrcPoller != 0 {
		f |= FSrcPoller
	}
	if s&SrcRecent != 0 {
		f |= FSrcRecent
	}
	return f
}

// NewBase builds a Base over plays (sorted, with IDs). ents must already
// resolve every play's track; contexts, devices and platforms are added
// (to ents itself when owned, else to a copy).
func NewBase(plays []Play, ents *Entities, in *ResolverInput, owned bool, span [2]int64) *Base {
	if !owned && needsPlayEntities(plays, ents) {
		ents = ents.clone()
	}
	n := len(plays)
	b := &Base{ID: make([]int64, n), Start: make([]int64, n), Wall: make([]int32, n), Ms: make([]int32, n),
		Track: make([]int32, n), Ctx: make([]int32, n), Device: make([]int16, n), Platform: make([]int16, n),
		Flags: make([]uint16, n), Ent: ents, ExportSpan: span}
	for i := range plays {
		p := &plays[i]
		b.ID[i], b.Start[i] = p.ID, p.StartedMs
		b.Wall[i] = int32(max(0, min(p.EndedMs-p.StartedMs, 1<<31-1)))
		b.Ms[i] = int32(max(0, min(p.MsPlayed, 1<<31-1)))
		b.Track[i] = ents.TrackByID[p.TrackID]
		b.Ctx[i] = -1
		if p.ContextURI != "" {
			b.Ctx[i] = ents.context(in, p.ContextURI)
		}
		b.Device[i] = ents.device(p.DeviceName, p.DeviceType)
		b.Platform[i] = ents.platform(p.Platform)
		f := sourceFlags(p.Sources)
		if p.Skipped {
			f |= FSkipped
		}
		if p.Shuffle != Unknown {
			f |= FShuffleKnown
			if p.Shuffle == True {
				f |= FShuffle
			}
		}
		if p.Estimated {
			f |= FEstimated
		}
		if p.Contradicted {
			f |= FContradicted
		}
		b.Flags[i] = f
		b.MaxID = max(b.MaxID, p.ID)
	}
	return b
}

func needsPlayEntities(plays []Play, e *Entities) bool {
	for i := range plays {
		p := &plays[i]
		if _, ok := e.ContextByID[p.ContextURI]; p.ContextURI != "" && !ok {
			return true
		}
		if _, ok := e.deviceKey[Device{p.DeviceName, p.DeviceType}]; p.DeviceName != "" && !ok {
			return true
		}
		if _, ok := e.platformKey[p.Platform]; p.Platform != "" && !ok {
			return true
		}
	}
	return false
}

// Settings are the derivation settings (D7, D14).
type Settings struct {
	Rule              Rule
	Loc               *time.Location
	SessionGapMinutes int
}

// Equal compares settings (locations by name).
func (s Settings) Equal(o Settings) bool {
	return s.Rule == o.Rule && s.SessionGapMinutes == o.SessionGapMinutes && s.Loc.String() == o.Loc.String()
}

// Exclusion is an excluded artist or track (D14).
type Exclusion struct {
	Kind string // artist, track
	ID   string
	Name string
}

// View is everything that depends on settings and exclusions.
type View struct {
	Settings Settings
	// Exclusions as configured, with the entities they resolve to (-1:
	// not in the data).
	Exclusions []Exclusion
	ExclIndex  []int32
	exclArtist []bool
	exclTrack  []bool

	Counted []bool  // passes the play rule
	Hour    []uint8 // local hour of the start
	// Vis are the indices of plays not excluded, in start order.
	Vis []int32

	Sessions  []Session
	SessionOf map[int64]int // first play ID -> index

	// All-time counted plays of visible plays per entity (search order).
	TrackPlays, ArtistPlays, AlbumPlays []int64
}

// Session is a run of visible plays [From, To) (indices into Vis) where
// each starts within the gap of the run's end so far.
type Session struct {
	From, To   int
	Start, End int64
}

// ArtistExcluded and TrackExcluded report exclusions by entity.
func (v *View) ArtistExcluded(a int32) bool { return int(a) < len(v.exclArtist) && v.exclArtist[a] }
func (v *View) TrackExcluded(t int32) bool  { return int(t) < len(v.exclTrack) && v.exclTrack[t] }

// NewView applies settings and exclusions to b.
func NewView(b *Base, s Settings, excl []Exclusion) *View {
	if s.Loc == nil {
		s.Loc = time.UTC
	}
	e := b.Ent
	n := b.Len()
	v := &View{Settings: s, Exclusions: excl, Counted: make([]bool, n), Hour: make([]uint8, n),
		exclArtist: make([]bool, len(e.Artists)), exclTrack: make([]bool, len(e.Tracks)),
		TrackPlays: make([]int64, len(e.Tracks)), ArtistPlays: make([]int64, len(e.Artists)),
		AlbumPlays: make([]int64, len(e.Albums)), SessionOf: map[int64]int{}}
	for _, x := range excl {
		v.ExclIndex = append(v.ExclIndex, resolveExclusion(e, x))
		if i := v.ExclIndex[len(v.ExclIndex)-1]; i >= 0 {
			if x.Kind == "artist" {
				v.exclArtist[i] = true
			} else {
				v.exclTrack[i] = true
			}
		}
	}
	trackExcl := slices.Clone(v.exclTrack)
	for t := range e.Tracks {
		for _, a := range e.Tracks[t].Artists {
			trackExcl[t] = trackExcl[t] || v.exclArtist[a]
		}
	}
	var zone localZone
	v.Vis = make([]int32, 0, n)
	for i := range n {
		t := b.Track[i]
		tr := &e.Tracks[t]
		v.Counted[i] = s.Rule.CountsAsPlay(Row{MsPlayed: int64(b.Ms[i]), DurationMs: tr.DurationMs,
			RecentOnly: b.Flags[i]&(FSrcExport|FSrcPoller|FSrcRecent) == FSrcRecent})
		v.Hour[i] = uint8(zone.secondOfDay(s.Loc, b.Start[i]) / 3600)
		if trackExcl[t] {
			continue
		}
		v.Vis = append(v.Vis, int32(i))
		if v.Counted[i] {
			v.TrackPlays[t]++
			if tr.Album >= 0 {
				v.AlbumPlays[tr.Album]++
			}
			for _, a := range tr.Artists {
				v.ArtistPlays[a]++
			}
		}
	}
	v.buildSessions(b)
	return v
}

// resolveExclusion finds the entity of an exclusion: by ID, else (for
// provisional artists replaced by hydration) by name when unambiguous.
func resolveExclusion(e *Entities, x Exclusion) int32 {
	if x.Kind == "track" {
		if i, ok := e.TrackByID[x.ID]; ok {
			return i
		}
		return -1
	}
	if i, ok := e.ArtistByID[x.ID]; ok {
		return i
	}
	if real := e.realByName[NormName(x.Name)]; len(real) == 1 {
		return real[0]
	}
	return -1
}

func (v *View) buildSessions(b *Base) {
	gap := int64(v.Settings.SessionGapMinutes) * 60_000
	start := 0
	var end int64
	for k, i := range v.Vis {
		if k > 0 && b.Start[i]-end > gap {
			v.addSession(b, start, k, end)
			start = k
		}
		if k == 0 {
			end = b.End(i)
		}
		end = max(end, b.End(i))
	}
	if len(v.Vis) > 0 {
		v.addSession(b, start, len(v.Vis), end)
	}
}

func (v *View) addSession(b *Base, from, to int, end int64) {
	v.SessionOf[b.ID[v.Vis[from]]] = len(v.Sessions)
	v.Sessions = append(v.Sessions, Session{From: from, To: to, Start: b.Start[v.Vis[from]], End: end})
}

// localZone caches the UTC offset of the zone period containing the last
// conversion, so converting sorted times costs one zone lookup per
// offset change instead of one per play.
type localZone struct {
	from, to int64 // ms, [from, to)
	offset   int64 // seconds
	ok       bool
}

func (z *localZone) offsetAt(loc *time.Location, ms int64) int64 {
	if !z.ok || ms < z.from || ms >= z.to {
		t := time.UnixMilli(ms).In(loc)
		_, off := t.Zone()
		s, e := t.ZoneBounds()
		z.offset, z.ok = int64(off), true
		z.from, z.to = -1<<62, 1<<62
		if !s.IsZero() {
			z.from = s.UnixMilli()
		}
		if !e.IsZero() {
			z.to = e.UnixMilli()
		}
	}
	return z.offset
}

// secondOfDay is the local second of the day of ms.
func (z *localZone) secondOfDay(loc *time.Location, ms int64) int64 {
	s := floorDiv(ms, 1000) + z.offsetAt(loc, ms)
	return s - floorDiv(s, 86400)*86400
}

// LocalDay is the local date of ms (YYYY-MM-DD).
func (z *localZone) localDay(loc *time.Location, ms int64) string {
	s := floorDiv(ms, 1000) + z.offsetAt(loc, ms)
	return time.Unix(floorDiv(s, 86400)*86400, 0).UTC().Format("2006-01-02")
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// SearchStart returns the first index in [0, len(b.Start)) whose start
// is ≥ ms.
func (b *Base) SearchStart(ms int64) int {
	return sort.Search(len(b.Start), func(i int) bool { return b.Start[i] >= ms })
}
