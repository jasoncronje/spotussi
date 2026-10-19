package refdata

import (
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"time"
)

// Listening model, in brief:
//
//   - Artists have Zipf popularity, a genre family, a career start and a
//     "discovery" month after which the listener's interest decays (except
//     for a few core favourites).
//   - Taste eras (1.5-3 years) weight genre families; months blend
//     between eras, so the top genres drift over the years.
//   - Daily volume grows over the first years, is higher at weekends, drops
//     during holidays, and is noisy. Volume is split into sessions whose
//     start hours follow weekday/weekend rhythms; a few are marathons.
//   - Sessions play segments: an album in order, a user playlist, a
//     Spotify-owned playlist (discovery), Liked Songs, or an artist.
//     Shuffle and skips depend on the mode. Binges focus a week or two on
//     one artist or a new album.
//   - Provenance follows the product story: export-only history, then the
//     poller starts ~11 months before End, and the export ends ~20 days
//     before End (newer rows are poller and recently-played only).

var familyWeight = []float64{1.0, 0.8, 0.9, 0.8, 0.8, 0.5, 0.3, 0.2, 0.3, 0.4, 0.3, 0.3, 0.2, 0.2, 0.3, 0.5}

var hourWeekday = []float64{0.2, 0.1, 0.05, 0.02, 0.02, 0.05, 0.3, 1.0, 1.4, 1.2, 1.1, 1.0,
	1.0, 1.1, 1.1, 1.0, 1.0, 1.3, 1.2, 1.1, 1.2, 1.3, 1.0, 0.6}
var hourWeekend = []float64{0.5, 0.4, 0.25, 0.1, 0.05, 0.03, 0.05, 0.1, 0.3, 0.6, 0.9, 1.1,
	1.2, 1.2, 1.2, 1.2, 1.2, 1.1, 1.0, 1.0, 1.2, 1.3, 1.2, 0.9}

// Device classes, mapped to concrete devices (poller) and platforms (export).
const (
	clsPhone = iota
	clsComputer
	clsSpeaker
	clsCast
	clsWeb
)

type gen struct {
	r   *rand.Rand
	p   sizeParams
	loc *time.Location
	ds  *Dataset

	startDay  time.Time // local midnight, day 0
	days      int
	months    int
	startYear int

	artistPop      []float64
	artistFamily   []int
	artistFirstDay []int32 // first release, days since epoch
	artistDiscover []int   // month index
	artistTau      []float64
	artistCore     []bool
	familyArtists  [][]int32

	// Per-artist tracks sorted by release day, with cumulative weights.
	artTracks [][]int32
	artDays   [][]int32
	artCum    [][]float64
	albumDay  []int32
	artAlbums [][]int32 // albums per artist sorted by release day

	eras      [][]float64 // family weights per era
	eraStarts []int       // month index where each era starts

	month    int
	monthCum []float64

	binge *binge

	userPlaylists  []userPlaylist
	spotifyLists   []int32 // context indices
	collectionCtx  int32
	contextByRef   map[int64]int32
	rowsByDay      []int
	cursor         int64 // ms; no session may start before this
	endMs          int64
	pollerSince    int64
	exportUntil    int64
	lastContext    int32
	lastDevice     int8
	lastShuffle    bool
	playlistsByDay map[int][]int // day -> indices into userPlaylists to build
}

type userPlaylist struct {
	ctx    int32
	day    int
	tracks []int32
	built  bool
}

type binge struct {
	untilDay int
	artist   int32
	album    int32 // -1 for artist binges
}

const dayMs = 24 * 60 * 60 * 1000

// Generate builds a dataset. It is deterministic in cfg.
func Generate(cfg Config) *Dataset {
	p, ok := sizes[cfg.Size]
	if !ok {
		p = sizes[Full]
	}
	tz := cfg.Timezone
	if tz == "" {
		tz = "Europe/London"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, tz = time.UTC, "UTC"
	}
	end := cfg.End
	if end.IsZero() {
		end = time.Date(2026, 10, 17, 18, 0, 0, 0, loc)
	}
	end = end.Truncate(time.Second)
	endLocal := end.In(loc)
	endDay := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), 0, 0, 0, 0, loc)
	startDay := endDay.AddDate(-p.years, 0, 1)

	g := &gen{
		r:        rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15)),
		p:        p,
		loc:      loc,
		startDay: startDay,
		endMs:    end.UnixMilli(),
		ds: &Dataset{
			Timezone: tz, Start: startDay, End: end,
		},
		contextByRef:   map[int64]int32{},
		playlistsByDay: map[int][]int{},
	}
	g.days = daysBetween(startDay, endDay) + 1
	g.startYear = startDay.Year()
	g.months = monthIndex(endDay, g.startYear) - monthIndex(startDay, g.startYear) + 1
	g.pollerSince = end.AddDate(0, -11, 0).UnixMilli()
	g.exportUntil = end.AddDate(0, 0, -20).UnixMilli()
	if g.pollerSince < startDay.UnixMilli() {
		g.pollerSince = startDay.UnixMilli() + (g.endMs-startDay.UnixMilli())/4
	}
	g.ds.PollerSince, g.ds.ExportUntil = g.pollerSince, g.exportUntil

	g.genres()
	g.artists()
	g.catalog()
	g.hydration()
	g.devices()
	g.contexts()
	g.eraPlan()
	g.dayPlan()
	g.listen()
	g.nowPlaying()
	g.prune()
	g.library(cfg.Seed)
	return g.ds
}

func daysBetween(a, b time.Time) int {
	ua := time.Date(a.Year(), a.Month(), a.Day(), 12, 0, 0, 0, time.UTC)
	ub := time.Date(b.Year(), b.Month(), b.Day(), 12, 0, 0, 0, time.UTC)
	return int(math.Round(ub.Sub(ua).Hours() / 24))
}

func monthIndex(t time.Time, baseYear int) int { return (t.Year()-baseYear)*12 + int(t.Month()) - 1 }

func epochDay(t time.Time) int32 {
	return int32(daysBetween(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), t))
}

func (g *gen) dayDate(d int) time.Time { return g.startDay.AddDate(0, 0, d) }

func (g *gen) normal(mean, sd float64) float64 { return mean + sd*g.r.NormFloat64() }

func weightedIndex(r *rand.Rand, w []float64) int {
	var sum float64
	for _, x := range w {
		sum += x
	}
	u := r.Float64() * sum
	for i, x := range w {
		if u < x {
			return i
		}
		u -= x
	}
	return len(w) - 1
}

func searchCum(cum []float64, u float64) int {
	i := sort.SearchFloat64s(cum, u)
	if i >= len(cum) {
		i = len(cum) - 1
	}
	return i
}

// ---------- catalog ----------

func (g *gen) genres() {
	seen := map[string]bool{}
	for f, name := range familyNames {
		g.ds.Genres = append(g.ds.Genres, Genre{ID: slug(name), Name: name, Family: f})
		seen[name] = true
	}
	for tries := 0; len(g.ds.Genres) < g.p.genres && tries < g.p.genres*50; tries++ {
		f := weightedIndex(g.r, familyWeight)
		name := pick(g.r, genreModifiers) + " " + familyNames[f]
		if seen[name] {
			continue
		}
		seen[name] = true
		g.ds.Genres = append(g.ds.Genres, Genre{ID: slug(name), Name: name, Family: f})
	}
}

func (g *gen) artists() {
	n := g.p.artists * artistOversize / 100
	byFamily := make([][]int32, len(familyNames))
	for i, ge := range g.ds.Genres {
		byFamily[ge.Family] = append(byFamily[ge.Family], int32(i))
	}
	endYear := g.ds.End.In(g.loc).Year()
	g.familyArtists = make([][]int32, len(familyNames))
	g.ds.Artists = make([]Artist, n)
	g.artistPop = make([]float64, n)
	g.artistFamily = make([]int, n)
	g.artistFirstDay = make([]int32, n)
	g.artistDiscover = make([]int, n)
	g.artistTau = make([]float64, n)
	g.artistCore = make([]bool, n)
	names := map[string]bool{}
	for i := range n {
		name := artistName(g.r)
		for names[name] {
			name = artistName(g.r)
		}
		names[name] = true
		fam := weightedIndex(g.r, familyWeight)
		ng := []int{0, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4}[g.r.IntN(19)]
		if i < n/50 && ng == 0 {
			ng = 2 // popular artists are well tagged
		}
		var gs []int32
		for range ng {
			pool := byFamily[fam]
			if g.r.Float64() < 0.15 {
				pool = byFamily[weightedIndex(g.r, familyWeight)]
			}
			gi := pick(g.r, pool)
			if !slices.Contains(gs, gi) {
				gs = append(gs, gi)
			}
		}
		g.ds.Artists[i] = Artist{ID: spotifyID(g.r), Name: name, Genres: gs, Hydrated: true}
		g.artistPop[i] = 1 / math.Pow(float64(i)+30, 0.9)
		g.artistFamily[i] = fam
		g.familyArtists[fam] = append(g.familyArtists[fam], int32(i))
		// Career start: recent careers are more common; popular artists
		// skew older.
		age := g.r.ExpFloat64() * 9
		if i < n/20 {
			age += 6
		}
		year := max(endYear-int(age), 1958)
		start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, g.r.IntN(365))
		if lim := g.ds.End.AddDate(0, 0, -7); start.After(lim) {
			start = lim
		}
		g.artistFirstDay[i] = epochDay(start)
		g.artistTau[i] = 4 + g.r.ExpFloat64()*14
		g.artistCore[i] = i < max(3, n/250) || g.r.Float64() < 0.004
	}
}

func (g *gen) addAlbum(artist int32, first bool) {
	var typ string
	var nt int
	u := g.r.Float64()
	switch {
	case first && u < 0.65, !first && u < 0.25:
		typ, nt = "single", 1+g.r.IntN(2)
	case first && u < 0.85, !first && u < 0.40:
		typ, nt = "single", 3+g.r.IntN(3) // EP
	default:
		typ, nt = "album", 7+g.r.IntN(7)
	}
	if !first && g.r.Float64() < 0.04 {
		typ, nt = "compilation", 12+g.r.IntN(8)
	}
	lo := g.artistFirstDay[artist]
	hi := epochDay(g.ds.End)
	day := lo
	if !first && hi > lo {
		day = lo + int32(g.r.IntN(int(hi-lo)+1))
	}
	rel := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(day))
	date := rel.Format("2006-01-02")
	if rel.Year() < 1995 && g.r.Float64() < 0.4 {
		date = rel.Format("2006")
	}
	ai := int32(len(g.ds.Albums))
	alb := Album{ID: spotifyID(g.r), Name: albumName(g.r), Type: typ, ReleaseDate: date,
		ReleaseYear: rel.Year(), Artists: []int32{artist}}
	if nt == 1 || (typ == "single" && g.r.Float64() < 0.5) {
		alb.Name = trackName(g.r) // singles are named after their lead track
	}
	albumWeight := math.Exp(g.normal(0, 0.8))
	ranks := g.r.Perm(nt)
	fam := g.artistFamily[artist]
	for t := range nt {
		ti := int32(len(g.ds.Tracks))
		name := trackName(g.r)
		if t == 0 && nt <= 2 {
			name = alb.Name
		}
		dur := g.normal(215_000, 55_000)
		switch {
		case g.r.Float64() < 0.015:
			dur = 12_000 + g.r.Float64()*17_000 // interlude
		case fam == 7 || fam == 10: // classical, ambient
			dur = g.normal(380_000, 160_000)
		case g.r.Float64() < 0.02:
			dur = 420_000 + g.r.Float64()*300_000
		}
		dur = math.Max(dur, 10_000)
		if dur > 1_500_000 {
			dur = 1_500_000
		}
		if nt > 2 || dur >= 70_000 {
			dur = math.Max(dur, 70_000)
		}
		artists := []int32{artist}
		if u := g.r.Float64(); u < 0.15 {
			pool := g.familyArtists[fam]
			for k := 0; k < 1+boolInt(u < 0.03); k++ {
				if f := pick(g.r, pool); !slices.Contains(artists, f) {
					artists = append(artists, f)
				}
			}
		}
		explicit := g.r.Float64() < 0.08 || (fam == 4 && g.r.Float64() < 0.6)
		g.ds.Tracks = append(g.ds.Tracks, Track{ID: spotifyID(g.r), Name: name, DurationMs: int32(dur),
			Explicit: explicit, Album: ai, Artists: artists, Hydrated: true})
		alb.Tracks = append(alb.Tracks, ti)
		w := albumWeight / math.Pow(1+float64(ranks[t]), 0.9)
		if typ == "single" {
			w *= 1.5
		}
		g.artTracks[artist] = append(g.artTracks[artist], ti)
		g.artDays[artist] = append(g.artDays[artist], day)
		g.artCum[artist] = append(g.artCum[artist], w) // weights for now; cumulated later
		// Featured artists can surface the track too, at a lower weight.
		for _, f := range artists[1:] {
			g.artTracks[f] = append(g.artTracks[f], ti)
			g.artDays[f] = append(g.artDays[f], day)
			g.artCum[f] = append(g.artCum[f], w*0.3)
		}
	}
	g.ds.Albums = append(g.ds.Albums, alb)
	g.albumDay = append(g.albumDay, day)
	g.artAlbums[artist] = append(g.artAlbums[artist], ai)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (g *gen) catalog() {
	n := len(g.ds.Artists)
	g.artTracks = make([][]int32, n)
	g.artDays = make([][]int32, n)
	g.artCum = make([][]float64, n)
	g.artAlbums = make([][]int32, n)
	for i := range n {
		g.addAlbum(int32(i), true)
	}
	// Extra releases go to popular artists more often.
	cum := make([]float64, n)
	var s float64
	for i := range n {
		s += math.Pow(g.artistPop[i], 0.7)
		cum[i] = s
	}
	for len(g.ds.Tracks) < g.p.tracks*trackOversize/100 {
		g.addAlbum(int32(searchCum(cum, g.r.Float64()*s)), false)
	}
	// Sort each artist's tracks by release day and cumulate weights.
	for a := range n {
		idx := make([]int, len(g.artTracks[a]))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(i, j int) bool { return g.artDays[a][idx[i]] < g.artDays[a][idx[j]] })
		tr := make([]int32, len(idx))
		dy := make([]int32, len(idx))
		cw := make([]float64, len(idx))
		var c float64
		for k, i := range idx {
			tr[k], dy[k] = g.artTracks[a][i], g.artDays[a][i]
			c += g.artCum[a][i]
			cw[k] = c
		}
		g.artTracks[a], g.artDays[a], g.artCum[a] = tr, dy, cw
		sort.SliceStable(g.artAlbums[a], func(i, j int) bool {
			return g.albumDay[g.artAlbums[a][i]] < g.albumDay[g.artAlbums[a][j]]
		})
	}
	// Discovery month: never before the first release or the start.
	first := monthIndex(g.startDay, g.startYear)
	for i := range n {
		rel := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(g.artistFirstDay[i]))
		lo := max(monthIndex(rel, g.startYear)-first, 0)
		hi := g.months
		if lo >= hi {
			lo = hi - 1
		}
		g.artistDiscover[i] = lo + int(math.Pow(g.r.Float64(), 1.3)*float64(hi-lo))
		if g.artistCore[i] {
			g.artistDiscover[i] = lo
		}
	}
}

// hydration marks a tail of the catalog as not yet hydrated (D9).
func (g *gen) hydration() {
	n := len(g.ds.Artists)
	for i := n * 7 / 10; i < n; i++ {
		if g.r.Float64() < 0.06 {
			g.ds.Artists[i].Hydrated = false
		}
	}
	for i := range g.ds.Tracks {
		t := &g.ds.Tracks[i]
		if int(t.Artists[0]) > n*6/10 && g.r.Float64() < 0.07 {
			t.Hydrated = false
		}
	}
}

func (g *gen) devices() {
	g.ds.Devices = []Device{
		{"iPhone", "Smartphone"},
		{"MacBook Pro", "Computer"},
		{"Living Room", "Speaker"},
		{"Kitchen", "CastAudio"},
		{"Web Player (Firefox)", "Computer"},
		{"Work Laptop", "Computer"},
	}
	g.ds.Platforms = []string{"android", "windows", "ios", "osx", "web_player", "sonos", "cast"}
}

func (g *gen) contexts() {
	add := func(c Context) int32 {
		g.ds.Contexts = append(g.ds.Contexts, c)
		return int32(len(g.ds.Contexts) - 1)
	}
	g.collectionCtx = add(Context{ID: "collection", URI: "spotify:user:listener:collection", Type: "collection",
		Name: "Liked Songs", NameSource: "api", Ref: -1})
	spotify := []string{"Discover Weekly", "Release Radar", "Daily Mix 1", "Daily Mix 2", "Daily Mix 3",
		"Daily Mix 4", "On Repeat", "Time Capsule"}
	for i, name := range spotify {
		id := "37i9dQZF1" + spotifyID(g.r)[:13]
		c := Context{ID: id, URI: "spotify:playlist:" + id, Type: "playlist", SpotifyOwned: true,
			NameSource: "unresolved", Ref: -1}
		if i == 0 || i == 2 {
			c.Name, c.NameSource = name, "manual" // the owner named these (D17)
		}
		g.spotifyLists = append(g.spotifyLists, add(c))
	}
	names := map[string]bool{}
	for range g.p.playlists {
		name := playlistName(g.r)
		for names[name] {
			name = playlistName(g.r) + " " + title(pick(g.r, adjectives))
		}
		names[name] = true
		id := spotifyID(g.r)
		ctx := add(Context{ID: id, URI: "spotify:playlist:" + id, Type: "playlist", Name: name, NameSource: "api", Ref: -1})
		day := g.r.IntN(g.days)
		g.userPlaylists = append(g.userPlaylists, userPlaylist{ctx: ctx, day: day})
		g.playlistsByDay[day] = append(g.playlistsByDay[day], len(g.userPlaylists)-1)
	}
}

func (g *gen) catalogContext(kind string, idx int32) int32 {
	key := int64(idx) << 1
	if kind == "artist" {
		key |= 1
	}
	if c, ok := g.contextByRef[key]; ok {
		return c
	}
	var c Context
	if kind == "album" {
		a := g.ds.Albums[idx]
		c = Context{ID: a.ID, URI: "spotify:album:" + a.ID, Type: "album", Name: a.Name, NameSource: "catalog", Ref: idx}
	} else {
		a := g.ds.Artists[idx]
		c = Context{ID: a.ID, URI: "spotify:artist:" + a.ID, Type: "artist", Name: a.Name, NameSource: "catalog", Ref: idx}
	}
	g.ds.Contexts = append(g.ds.Contexts, c)
	g.contextByRef[key] = int32(len(g.ds.Contexts) - 1)
	return g.contextByRef[key]
}

// ---------- taste over time ----------

func (g *gen) eraPlan() {
	m := 0
	for m < g.months {
		w := make([]float64, len(familyNames))
		for f := range w {
			w[f] = familyWeight[f] * (0.2 + 0.8*g.r.Float64())
		}
		for range 2 + g.r.IntN(2) {
			w[weightedIndex(g.r, familyWeight)] *= 4 + 4*g.r.Float64()
		}
		g.eras = append(g.eras, w)
		g.eraStarts = append(g.eraStarts, m)
		m += 18 + g.r.IntN(19)
	}
}

// familyWeights blends eras over a 4-month transition.
func (g *gen) familyWeights(m int) []float64 {
	e := sort.SearchInts(g.eraStarts, m+1) - 1
	cur := g.eras[e]
	if e == 0 || m-g.eraStarts[e] >= 4 {
		return cur
	}
	t := float64(m-g.eraStarts[e]+1) / 5
	prev := g.eras[e-1]
	out := make([]float64, len(cur))
	for i := range cur {
		out[i] = prev[i]*(1-t) + cur[i]*t
	}
	return out
}

func (g *gen) setMonth(m int) {
	if g.monthCum != nil && m == g.month {
		return
	}
	g.month = m
	fw := g.familyWeights(m)
	if g.monthCum == nil {
		g.monthCum = make([]float64, len(g.ds.Artists))
	}
	monthStartDay := epochDay(g.startDay.AddDate(0, m, 0))
	var s float64
	for i := range g.ds.Artists {
		w := 0.0
		if m >= g.artistDiscover[i] && g.artistFirstDay[i] <= monthStartDay+31 {
			age := float64(m - g.artistDiscover[i])
			interest := 0.12 + 0.88*math.Exp(-age/g.artistTau[i])
			if g.artistCore[i] {
				interest = 0.45 + 0.55*math.Exp(-age/(3*g.artistTau[i]))
			}
			w = g.artistPop[i] * fw[g.artistFamily[i]] * interest
		}
		s += w
		g.monthCum[i] = s
	}
}

func (g *gen) pickArtist() int32 {
	return int32(searchCum(g.monthCum, g.r.Float64()*g.monthCum[len(g.monthCum)-1]))
}

// pickTrack picks a released track of artist a on epoch day d (-1 if none).
func (g *gen) pickTrack(a int32, d int32) int32 {
	days := g.artDays[a]
	k := sort.Search(len(days), func(i int) bool { return days[i] > d })
	if k == 0 {
		return -1
	}
	cum := g.artCum[a][:k]
	return g.artTracks[a][searchCum(cum, g.r.Float64()*cum[k-1])]
}

// tasteTrack picks a track from the current taste (with binges).
func (g *gen) tasteTrack(day int, d int32, discovery bool) int32 {
	if b := g.binge; b != nil && day <= b.untilDay && !discovery && g.r.Float64() < 0.45 {
		if t := g.pickTrack(b.artist, d); t >= 0 {
			return t
		}
	}
	for range 8 {
		a := g.pickArtist()
		if g.r.Float64() < 0.12 {
			a = int32(g.r.IntN(len(g.ds.Artists))) // radio-style exploration of the long tail
		}
		if discovery && int(a) < len(g.ds.Artists)/30 && g.r.Float64() < 0.85 {
			continue
		}
		if t := g.pickTrack(a, d); t >= 0 {
			return t
		}
	}
	// Fall back to anything released.
	for {
		a := int32(g.r.IntN(len(g.ds.Artists)))
		if t := g.pickTrack(a, d); t >= 0 {
			return t
		}
	}
}

// ---------- daily volume ----------

func (g *gen) dayPlan() {
	w := make([]float64, g.days)
	holidayUntil := -1
	for d := range g.days {
		date := g.dayDate(d)
		if d > holidayUntil && g.r.Float64() < 1.5/365 {
			holidayUntil = d + 5 + g.r.IntN(10)
		}
		years := float64(d) / 365
		x := 0.45 + 0.55*math.Min(1, years/4) // listening grows over the first years
		if wd := date.Weekday(); wd == time.Saturday || wd == time.Sunday {
			x *= 1.15
		}
		if d <= holidayUntil {
			x *= 0.15
		}
		x *= math.Exp(g.normal(0, 0.45))
		if g.r.Float64() < 0.04 {
			x = 0
		}
		w[d] = x
	}
	var sum float64
	for _, x := range w {
		sum += x
	}
	g.rowsByDay = make([]int, g.days)
	for d, x := range w {
		v := float64(g.p.rows) * x / sum
		n := int(v)
		if g.r.Float64() < v-float64(n) {
			n++
		}
		g.rowsByDay[d] = n
	}
}

// ---------- listening ----------

type segment struct {
	ctx     int32
	shuffle bool
	tracks  []int32 // fixed sequence (albums), or nil to draw from next
	next    func() int32
	left    int
	skipP   float64
}

func (g *gen) newSegment(day int, d int32) segment {
	u := g.r.Float64()
	// Album binge: play the binge album start to finish.
	if b := g.binge; b != nil && b.album >= 0 && day <= b.untilDay && u < 0.3 {
		return segment{ctx: g.catalogContext("album", b.album), tracks: slices.Clone(g.ds.Albums[b.album].Tracks), skipP: 0.06}
	}
	switch {
	case u < 0.20: // album
		a := g.pickArtist()
		albums := g.artAlbums[a]
		k := sort.Search(len(albums), func(i int) bool { return g.albumDay[albums[i]] > d })
		if k == 0 {
			break
		}
		alb := albums[g.r.IntN(k)]
		tr := g.ds.Albums[alb].Tracks
		if g.r.Float64() < 0.2 {
			tr = tr[g.r.IntN(len(tr)):]
		}
		return segment{ctx: g.catalogContext("album", alb), tracks: slices.Clone(tr), skipP: 0.08}
	case u < 0.48: // user playlist
		var avail []int
		for i, pl := range g.userPlaylists {
			if pl.built && len(pl.tracks) > 0 {
				avail = append(avail, i)
			}
		}
		if len(avail) == 0 {
			break
		}
		// Recently created playlists are played more.
		ws := make([]float64, len(avail))
		for i, pi := range avail {
			age := float64(day - g.userPlaylists[pi].day)
			ws[i] = 0.15 + math.Exp(-age/120)
		}
		pl := g.userPlaylists[avail[weightedIndex(g.r, ws)]]
		shuffle := g.r.Float64() < 0.7
		pos := g.r.IntN(len(pl.tracks))
		next := func() int32 {
			if shuffle {
				return pl.tracks[g.r.IntN(len(pl.tracks))]
			}
			t := pl.tracks[pos%len(pl.tracks)]
			pos++
			return t
		}
		return segment{ctx: pl.ctx, shuffle: shuffle, next: next, left: 5 + g.r.IntN(25), skipP: 0.12}
	case u < 0.58: // Spotify-owned playlist (discovery)
		ctx := g.spotifyLists[g.r.IntN(len(g.spotifyLists))]
		return segment{ctx: ctx, next: func() int32 { return g.tasteTrack(day, d, true) },
			left: 5 + g.r.IntN(25), skipP: 0.3}
	case u < 0.68: // artist page
		a := g.pickArtist()
		if g.pickTrack(a, d) < 0 {
			break
		}
		shuffle := g.r.Float64() < 0.5
		return segment{ctx: g.catalogContext("artist", a), shuffle: shuffle,
			next: func() int32 {
				if t := g.pickTrack(a, d); t >= 0 {
					return t
				}
				return g.tasteTrack(day, d, false)
			}, left: 3 + g.r.IntN(15), skipP: 0.1}
	}
	// Liked Songs, usually shuffled.
	shuffle := g.r.Float64() < 0.85
	return segment{ctx: g.collectionCtx, shuffle: shuffle,
		next: func() int32 { return g.tasteTrack(day, d, false) }, left: 5 + g.r.IntN(40), skipP: 0.14}
}

func (g *gen) buildPlaylists(day int, d int32) {
	for _, pi := range g.playlistsByDay[day] {
		pl := &g.userPlaylists[pi]
		n := 15 + g.r.IntN(100)
		for range n {
			pl.tracks = append(pl.tracks, g.tasteTrack(day, d, false))
		}
		pl.built = true
	}
}

func (g *gen) deviceClass(hour int, weekend bool) int {
	switch {
	case weekend:
		return weightedIndex(g.r, []float64{0.4, 0.15, 0.35, 0.1, 0})
	case hour >= 9 && hour < 17:
		return weightedIndex(g.r, []float64{0.25, 0.55, 0.05, 0.05, 0.1})
	case hour >= 7 && hour < 9, hour >= 17 && hour < 19:
		return weightedIndex(g.r, []float64{0.8, 0.1, 0.05, 0.05, 0})
	default:
		return weightedIndex(g.r, []float64{0.35, 0.2, 0.35, 0.1, 0})
	}
}

// device and platform for a class at a given point in history.
func (g *gen) devicePlatform(cls int, year int, workHours bool) (int8, int8) {
	const (
		android, windows, ios, osx, web, sonos, cast = 0, 1, 2, 3, 4, 5, 6
	)
	switch cls {
	case clsPhone:
		if year < 4 {
			return 0, android
		}
		return 0, ios
	case clsComputer:
		if workHours {
			return 5, windows
		}
		if year < 6 {
			return 1, windows
		}
		return 1, osx
	case clsSpeaker:
		if year < 7 {
			return 0, ios // phone into a speaker
		}
		return 2, sonos
	case clsCast:
		return 3, cast
	default:
		return 4, web
	}
}

func (g *gen) listen() {
	plays := make([]Play, 0, g.p.rows+g.p.rows/50)
	firstMonth := monthIndex(g.startDay, g.startYear)
	g.cursor = g.startDay.UnixMilli()
	nextBinge := 30 + g.r.IntN(60)
	for day := range g.days {
		date := g.dayDate(day)
		d := epochDay(date)
		g.setMonth(monthIndex(date, g.startYear) - firstMonth)
		g.buildPlaylists(day, d)
		if day >= nextBinge {
			g.startBinge(day, d)
			nextBinge = day + 45 + g.r.IntN(80)
		}
		rows := g.rowsByDay[day]
		if rows == 0 {
			continue
		}
		weekend := date.Weekday() == time.Saturday || date.Weekday() == time.Sunday
		profile := hourWeekday
		if weekend {
			profile = hourWeekend
		}
		// Split the day's rows into sessions.
		var sizes []int
		for left := rows; left > 0; {
			n := int(math.Round(math.Exp(g.normal(2.3, 0.7))))
			if g.r.Float64() < 0.02 {
				n = 50 + g.r.IntN(110)
			}
			n = max(1, min(n, left))
			sizes = append(sizes, n)
			left -= n
		}
		starts := make([]int64, len(sizes))
		for i := range starts {
			h := weightedIndex(g.r, profile)
			starts[i] = date.Add(time.Duration(h)*time.Hour + time.Duration(g.r.IntN(3600))*time.Second).UnixMilli()
		}
		slices.Sort(starts)
		year := day / 365
		for i, n := range sizes {
			start := max(starts[i], g.cursor+int64(10+g.r.IntN(30))*60_000)
			if start >= g.endMs-60_000 {
				break
			}
			hour := time.UnixMilli(start).In(g.loc).Hour()
			cls := g.deviceClass(hour, weekend)
			dev, plat := g.devicePlatform(cls, year, !weekend && hour >= 9 && hour < 17)
			plays = g.session(plays, day, d, start, n, dev, plat)
		}
	}
	g.ds.Plays = plays
}

func (g *gen) startBinge(day int, d int32) {
	b := &binge{untilDay: day + 5 + g.r.IntN(16), album: -1}
	b.artist = g.pickArtist()
	if g.r.Float64() < 0.4 {
		// A new release: the newest album released in the last 60 days.
		albums := g.artAlbums[b.artist]
		k := sort.Search(len(albums), func(i int) bool { return g.albumDay[albums[i]] > d })
		if k > 0 && g.albumDay[albums[k-1]] > d-60 && len(g.ds.Albums[albums[k-1]].Tracks) > 3 {
			b.album = albums[k-1]
		}
	}
	if g.pickTrack(b.artist, d) < 0 {
		return
	}
	g.binge = b
}

func (g *gen) session(plays []Play, day int, d int32, t int64, n int, dev, plat int8) []Play {
	seg := g.newSegment(day, d)
	for i := range n {
		if (seg.tracks != nil && len(seg.tracks) == 0) || (seg.tracks == nil && seg.left <= 0) ||
			(n > 20 && g.r.Float64() < 0.03) {
			seg = g.newSegment(day, d)
		}
		var tr int32
		if seg.tracks != nil {
			tr, seg.tracks = seg.tracks[0], seg.tracks[1:]
		} else {
			tr = seg.next()
			seg.left--
		}
		track := g.ds.Tracks[tr]
		dur := int64(track.DurationMs)
		skipP := seg.skipP
		if seg.shuffle {
			skipP += 0.12
		}
		var ms int64
		skipped := g.r.Float64() < skipP
		switch {
		case skipped && g.r.Float64() < 0.55:
			ms = 500 + g.r.Int64N(29_000)
		case skipped:
			ms = 30_000 + g.r.Int64N(max(1, dur*6/10))
		case i == n-1 && g.r.Float64() < 0.35:
			ms = int64(float64(dur) * (0.1 + 0.8*g.r.Float64())) // stopped listening
		default:
			ms = dur - g.r.Int64N(1500)
		}
		ms = max(0, min(ms, dur))
		if t >= g.endMs-30_000 {
			break
		}
		p := Play{StartMs: t, MsPlayed: int32(ms), Track: tr, Context: seg.ctx, Device: dev, Platform: plat}
		if skipped {
			p.Flags |= FlagSkipped
		}
		p.Flags |= FlagShuffleKnown
		if seg.shuffle {
			p.Flags |= FlagShuffle
		}
		g.provenance(&p, dur)
		plays = append(plays, p)
		g.lastContext, g.lastDevice, g.lastShuffle = seg.ctx, dev, seg.shuffle

		gap := g.r.Int64N(2500)
		if skipped {
			gap = 200 + g.r.Int64N(1300)
		}
		if g.r.Float64() < 0.015 {
			gap += 60_000 + g.r.Int64N(9*60_000) // a pause
		}
		t += int64(p.MsPlayed) + gap
	}
	g.cursor = t
	return plays
}

// provenance sets the source flags and blanks fields a source lacks.
func (g *gen) provenance(p *Play, dur int64) {
	spotifyCounted := p.MsPlayed >= 30_000 // what recently-played reports
	switch {
	case p.StartMs < g.pollerSince:
		p.Flags |= FlagSrcExport
		p.Context, p.Device = -1, -1
	case p.StartMs < g.exportUntil:
		u := g.r.Float64()
		switch {
		case u < 0.03 && spotifyCounted: // poller down; recently-played backfilled it
			p.Flags |= FlagSrcExport | FlagSrcRecent
			p.Device = -1
		case u < 0.04: // poller down, too long for recently-played
			p.Flags |= FlagSrcExport
			p.Context, p.Device = -1, -1
		case u < 0.043: // poller saw something the export does not confirm
			p.Flags |= FlagSrcPoller | FlagContradicted
			p.Platform = -1
		default:
			p.Flags |= FlagSrcExport | FlagSrcPoller
			if spotifyCounted {
				p.Flags |= FlagSrcRecent
			}
		}
	default: // after the export: live sources only
		p.Platform = -1
		if g.r.Float64() < 0.04 && spotifyCounted {
			// Only recently-played saw it: duration estimated, no skip or
			// shuffle data (D7).
			p.Flags = FlagSrcRecent | FlagEstimated
			p.MsPlayed = int32(dur)
			p.Device = -1
		} else {
			p.Flags |= FlagSrcPoller
			if spotifyCounted {
				p.Flags |= FlagSrcRecent
			}
		}
	}
}

func (g *gen) nowPlaying() {
	d := epochDay(g.ds.End)
	t := g.tasteTrack(g.days-1, d, false)
	g.ds.Now = NowPlaying{Track: t, Context: g.lastContext, Device: g.lastDevice,
		StartedMs: g.endMs - 47_000, Shuffle: g.lastShuffle}
	if g.ds.Now.Device < 0 {
		g.ds.Now.Device = 0
	}
}

// The catalog is over-generated (percent) because the app's catalog only
// ever contains entities that were played; unplayed ones are pruned after
// listening, which lands near the target sizes.
const (
	artistOversize = 112
	trackOversize  = 250
)

// prune drops catalog entities and contexts that no play references and
// re-indexes everything, so the catalog looks like one built from history.
func (g *gen) prune() {
	ds := g.ds
	usedT := make([]int32, len(ds.Tracks))
	usedC := make([]int32, len(ds.Contexts))
	for i := range usedT {
		usedT[i] = -1
	}
	for i := range usedC {
		usedC[i] = -1
	}
	mark := func(t, c int32) {
		usedT[t] = 0
		if c >= 0 {
			usedC[c] = 0
		}
	}
	for _, p := range ds.Plays {
		mark(p.Track, p.Context)
	}
	mark(ds.Now.Track, ds.Now.Context)

	usedA := make([]int32, len(ds.Artists))
	usedAl := make([]int32, len(ds.Albums))
	for i := range usedA {
		usedA[i] = -1
	}
	for i := range usedAl {
		usedAl[i] = -1
	}
	for t, u := range usedT {
		if u < 0 {
			continue
		}
		tr := ds.Tracks[t]
		usedAl[tr.Album] = 0
		for _, a := range tr.Artists {
			usedA[a] = 0
		}
	}
	renumber := func(used []int32) int {
		n := 0
		for i, u := range used {
			if u >= 0 {
				used[i] = int32(n)
				n++
			}
		}
		return n
	}
	renumber(usedA)
	renumber(usedAl)
	renumber(usedT)
	renumber(usedC)
	remap := func(xs []int32, m []int32) []int32 {
		out := xs[:0:0]
		for _, x := range xs {
			if m[x] >= 0 {
				out = append(out, m[x])
			}
		}
		return out
	}

	var artists []Artist
	for i, a := range ds.Artists {
		if usedA[i] >= 0 {
			artists = append(artists, a)
		}
	}
	var albums []Album
	for i, al := range ds.Albums {
		if usedAl[i] >= 0 {
			al.TotalTracks = len(al.Tracks)
			al.Artists = remap(al.Artists, usedA)
			al.Tracks = remap(al.Tracks, usedT)
			albums = append(albums, al)
		}
	}
	var tracks []Track
	for i, t := range ds.Tracks {
		if usedT[i] >= 0 {
			t.Album = usedAl[t.Album]
			t.Artists = remap(t.Artists, usedA)
			tracks = append(tracks, t)
		}
	}
	var contexts []Context
	for i, c := range ds.Contexts {
		if usedC[i] < 0 {
			continue
		}
		switch c.Type {
		case "album":
			c.Ref = usedAl[c.Ref]
		case "artist":
			c.Ref = usedA[c.Ref]
		}
		contexts = append(contexts, c)
	}
	for i := range ds.Plays {
		p := &ds.Plays[i]
		p.Track = usedT[p.Track]
		if p.Context >= 0 {
			p.Context = usedC[p.Context]
		}
	}
	ds.Now.Track = usedT[ds.Now.Track]
	if ds.Now.Context >= 0 {
		ds.Now.Context = usedC[ds.Now.Context]
	}
	ds.Artists, ds.Albums, ds.Tracks, ds.Contexts = artists, albums, tracks, contexts
}
