package derive

import (
	"encoding/base64"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unsafe"
)

// Entity resolution: every track of a play gets a name, album, artists,
// duration and genres, from the best source that knows it:
//
//  1. the catalog (D9), filled by hydration (M4);
//  2. the track object of the newest live row (poller or recently-played),
//     which carries real Spotify artist and album IDs;
//  3. the export row's names: provisional artist and album entities keyed
//     by normalized name (D20), with opaque stable IDs.
//
// A provisional artist whose name matches exactly one real artist is that
// artist (likewise albums, by album and artist name), so export-only and
// live plays of one artist rank together before hydration finishes. When
// hydration fills the catalog, the next rebuild switches entities over.

// Meta says where an entity's details come from (M7 provenance).
type Meta uint8

const (
	// MetaPending: known by name only; details are still being looked up.
	MetaPending Meta = iota
	// MetaSpotify: details from Spotify (a catalog lookup or a live track object).
	MetaSpotify
	// MetaFallback: details from the fallback sources (D27) while Spotify waits.
	MetaFallback
)

// Track is a resolved track.
type Track struct {
	ID, Name   string
	DurationMs int64 // 0: unknown
	Explicit   Tri
	Hydrated   bool // in the catalog with hydrated_at set
	Meta       Meta
	Album      int32 // -1: unknown
	Artists    []int32
	Genres     []int32 // union of the artists' genres, each once
}

// Artist is a resolved artist.
type Artist struct {
	ID, Name    string
	ImageURL    string
	Hydrated    bool
	Provisional bool
	Meta        Meta
	Genres      []int32
}

// Album is a resolved album.
type Album struct {
	ID, Name    string
	ImageURL    string
	Type        string
	ReleaseDate string
	ReleaseYear int // 0: unknown
	TotalTracks int // 0: unknown
	Artists     []int32
	Hydrated    bool
	Provisional bool
	Meta        Meta
}

// Genre is an artist genre (artist_genres).
type Genre struct{ ID, Name string }

// Context is where plays started from.
type Context struct {
	URI, Type, ID string
	Name          string
	HasName       bool
	NameSource    string // api, catalog, manual, unresolved
	SpotifyOwned  bool
	ImageURL      string
	Album, Artist int32 // album/artist contexts, else -1
}

// Device is a playback device as the poller saw it.
type Device struct{ Name, Type string }

// Entities are the resolved entities of a snapshot, indexed densely.
// They are immutable once built.
type Entities struct {
	Tracks    []Track
	Artists   []Artist
	Albums    []Album
	Genres    []Genre
	Contexts  []Context
	Devices   []Device
	Platforms []string

	TrackByID   map[string]int32
	ArtistByID  map[string]int32 // includes provisional IDs merged into real artists
	AlbumByID   map[string]int32 // likewise
	GenreByID   map[string]int32
	ContextByID map[string]int32 // by URI
	deviceKey   map[Device]int16
	platformKey map[string]int16

	// provArtist and provAlbum map provisional keys to entities.
	provArtist map[string]int32
	provAlbum  map[string]int32
	// real artists by normalized name, and real albums by artist, for
	// unification.
	realByName   map[string][]int32
	artistAlbums map[int32][]int32
	// Stale: a real artist arrived (incremental) whose name matches an
	// existing provisional artist; the next full rebuild unifies them.
	Stale bool
}

// Provisional ID prefixes (D20). The rest is base64url of the
// normalized key, so IDs are stable across rebuilds.
const (
	ProvArtistPrefix = "ar_"
	ProvAlbumPrefix  = "al_"
)

// NormName normalizes an entity name for provisional keys: trimmed
// spaces and ASCII lower-case (what SQLite's trim() and lower() do).
func NormName(s string) string {
	s = strings.Trim(s, " ")
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// ProvArtistID is the provisional artist ID of a name.
func ProvArtistID(name string) string {
	return ProvArtistPrefix + base64.RawURLEncoding.EncodeToString([]byte(NormName(name)))
}

// ProvAlbumID is the provisional album ID of an album name by an artist
// ("Greatest Hits" is many albums).
func ProvAlbumID(artist, album string) string {
	return ProvAlbumPrefix + base64.RawURLEncoding.EncodeToString([]byte(NormName(artist)+"\x00"+NormName(album)))
}

// GenreID is the stable slug of a genre name (as the reference dataset
// makes them).
func GenreID(name string) string {
	s := strings.ToLower(name)
	return strings.NewReplacer("&", "n", " ", "-", "'", "", "/", "-", "?", "", "#", "").Replace(s)
}

// ---------- resolver input ----------

// CatTrack, CatAlbum, CatArtist and CatContext are catalog rows.
type CatTrack struct {
	Name       string
	DurationMs int64 // 0: unknown
	Explicit   Tri
	AlbumID    string
	Hydrated   bool
	Fallback   bool // tracks.source = 'fallback' (D27)
}

type CatAlbum struct {
	Name, Type, ReleaseDate, ImageURL string
	TotalTracks                       int
	Hydrated                          bool
}

type CatArtist struct {
	Name, ImageURL string
	Hydrated       bool
}

type CatContext struct {
	Type, Name, NameSource, ImageURL string
	HasName                          bool
}

// LiveTrack is the track object of a live raw row.
type LiveTrack struct {
	Name       string
	DurationMs int64
	Explicit   Tri
	Album      LiveAlbum
	Artists    []LiveArtist
}

type LiveAlbum struct {
	ID, Name, Type, ReleaseDate, ImageURL string
	TotalTracks                           int
	Artists                               []LiveArtist
}

type LiveArtist struct{ ID, Name string }

// ExportNames are the names on a track's newest export row.
type ExportNames struct{ Track, Artist, Album string }

// ResolverInput is what entity resolution reads, for some or all tracks.
type ResolverInput struct {
	Tracks       map[string]CatTrack
	TrackArtists map[string][]string
	Albums       map[string]CatAlbum
	AlbumArtists map[string][]string
	Artists      map[string]CatArtist
	ArtistGenres map[string][]string
	Contexts     map[string]CatContext
	Live         map[string]LiveTrack
	Export       map[string]ExportNames
	// Fallback metadata (D27) for provisional artists and albums, keyed by
	// their provisional IDs; their genres are in ArtistGenres under the
	// same IDs.
	FallbackArtists map[string]CatArtist
	FallbackAlbums  map[string]CatAlbum
}

func newResolverInput() *ResolverInput {
	return &ResolverInput{Tracks: map[string]CatTrack{}, TrackArtists: map[string][]string{}, Albums: map[string]CatAlbum{},
		AlbumArtists: map[string][]string{}, Artists: map[string]CatArtist{}, ArtistGenres: map[string][]string{},
		Contexts: map[string]CatContext{}, Live: map[string]LiveTrack{}, Export: map[string]ExportNames{},
		FallbackArtists: map[string]CatArtist{}, FallbackAlbums: map[string]CatAlbum{}}
}

// ---------- resolution ----------

func newEntities() *Entities {
	return &Entities{TrackByID: map[string]int32{}, ArtistByID: map[string]int32{}, AlbumByID: map[string]int32{},
		GenreByID: map[string]int32{}, ContextByID: map[string]int32{}, deviceKey: map[Device]int16{},
		platformKey: map[string]int16{}, provArtist: map[string]int32{}, provAlbum: map[string]int32{},
		realByName: map[string][]int32{}, artistAlbums: map[int32][]int32{}}
}

// clone copies e for copy-on-write extension (entities themselves are
// shared; slices of them are appended, never modified in place).
func (e *Entities) clone() *Entities {
	c := *e
	c.Tracks, c.Artists, c.Albums = slices.Clip(e.Tracks), slices.Clip(e.Artists), slices.Clip(e.Albums)
	c.Genres, c.Contexts = slices.Clip(e.Genres), slices.Clip(e.Contexts)
	c.Devices, c.Platforms = slices.Clip(e.Devices), slices.Clip(e.Platforms)
	c.TrackByID, c.ArtistByID, c.AlbumByID = maps.Clone(e.TrackByID), maps.Clone(e.ArtistByID), maps.Clone(e.AlbumByID)
	c.GenreByID, c.ContextByID = maps.Clone(e.GenreByID), maps.Clone(e.ContextByID)
	c.deviceKey, c.platformKey = maps.Clone(e.deviceKey), maps.Clone(e.platformKey)
	c.provArtist, c.provAlbum = maps.Clone(e.provArtist), maps.Clone(e.provAlbum)
	c.realByName, c.artistAlbums = maps.Clone(e.realByName), maps.Clone(e.artistAlbums)
	return &c
}

// Resolve builds entities for trackIDs from in (a full resolution).
func Resolve(in *ResolverInput, trackIDs []string) *Entities {
	e := newEntities()
	e.addTracks(in, trackIDs)
	// Within one batch real entities resolve first, so a full resolution
	// is never stale (only ID-less local-file artists can precede a real
	// namesake, and rebuilding would not change that).
	e.Stale = false
	e.compact()
	return e
}

// compact copies every string e keeps into one allocation. Resolution
// reads them from short-lived rows scattered across the heap, and each
// kept string would otherwise pin a mostly-empty page of garbage: on a
// lifetime export that roughly doubled the heap the server holds while
// idle (D25).
func (e *Entities) compact() {
	seen := map[string]string{}
	var buf []byte
	size := 0
	intern := func(s string) string {
		if s == "" {
			return s
		}
		if t, ok := seen[s]; ok {
			return t
		}
		start := len(buf)
		buf = append(buf, s...)
		t := unsafe.String(&buf[start], len(s))
		seen[s] = t
		return t
	}
	visit := func(f func(string) string) {
		for i := range e.Tracks {
			t := &e.Tracks[i]
			t.ID, t.Name = f(t.ID), f(t.Name)
		}
		for i := range e.Artists {
			a := &e.Artists[i]
			a.ID, a.Name, a.ImageURL = f(a.ID), f(a.Name), f(a.ImageURL)
		}
		for i := range e.Albums {
			a := &e.Albums[i]
			a.ID, a.Name, a.ImageURL, a.Type, a.ReleaseDate = f(a.ID), f(a.Name), f(a.ImageURL), f(a.Type), f(a.ReleaseDate)
		}
		for i := range e.Genres {
			g := &e.Genres[i]
			g.ID, g.Name = f(g.ID), f(g.Name)
		}
		for i := range e.Contexts {
			c := &e.Contexts[i]
			c.URI, c.Type, c.ID, c.Name, c.NameSource, c.ImageURL = f(c.URI), f(c.Type), f(c.ID), f(c.Name), f(c.NameSource), f(c.ImageURL)
		}
		for i := range e.Devices {
			d := &e.Devices[i]
			d.Name, d.Type = f(d.Name), f(d.Type)
		}
		for i := range e.Platforms {
			e.Platforms[i] = f(e.Platforms[i])
		}
	}
	// Size the buffer first (fields and map keys), so interned strings
	// never move.
	add := func(s string) string {
		if _, ok := seen[s]; !ok {
			seen[s] = s
			size += len(s)
		}
		return s
	}
	visit(add)
	for _, m := range []map[string]int32{e.TrackByID, e.ArtistByID, e.AlbumByID, e.GenreByID, e.ContextByID, e.provArtist, e.provAlbum} {
		for k := range m {
			add(k)
		}
	}
	for k := range e.realByName {
		add(k)
	}
	clear(seen)
	buf = make([]byte, 0, size)
	visit(intern)
	rekey := func(m map[string]int32) map[string]int32 {
		out := make(map[string]int32, len(m))
		for k, v := range m {
			out[intern(k)] = v
		}
		return out
	}
	e.TrackByID, e.ArtistByID, e.AlbumByID = rekey(e.TrackByID), rekey(e.ArtistByID), rekey(e.AlbumByID)
	e.GenreByID, e.ContextByID = rekey(e.GenreByID), rekey(e.ContextByID)
	e.provArtist, e.provAlbum = rekey(e.provArtist), rekey(e.provAlbum)
	byName := make(map[string][]int32, len(e.realByName))
	for k, v := range e.realByName {
		byName[intern(k)] = v
	}
	e.realByName = byName
	plat := make(map[string]int16, len(e.platformKey))
	for k, v := range e.platformKey {
		plat[intern(k)] = v
	}
	e.platformKey = plat
	dev := make(map[Device]int16, len(e.deviceKey))
	for k, v := range e.deviceKey {
		dev[Device{intern(k.Name), intern(k.Type)}] = v
	}
	e.deviceKey = dev

	// Likewise the small index slices.
	n := 0
	for i := range e.Tracks {
		n += len(e.Tracks[i].Artists) + len(e.Tracks[i].Genres)
	}
	for i := range e.Artists {
		n += len(e.Artists[i].Genres)
	}
	for i := range e.Albums {
		n += len(e.Albums[i].Artists)
	}
	ids := make([]int32, 0, n)
	pack := func(x []int32) []int32 {
		if len(x) == 0 {
			return x
		}
		k := len(ids)
		ids = append(ids, x...)
		return ids[k:len(ids):len(ids)] // full: an append copies
	}
	for i := range e.Tracks {
		t := &e.Tracks[i]
		t.Artists, t.Genres = pack(t.Artists), pack(t.Genres)
	}
	for i := range e.Artists {
		e.Artists[i].Genres = pack(e.Artists[i].Genres)
	}
	for i := range e.Albums {
		e.Albums[i].Artists = pack(e.Albums[i].Artists)
	}
}

// Extend returns a copy of e with trackIDs resolved too (incremental
// merges); tracks already known keep their resolution.
func (e *Entities) Extend(in *ResolverInput, trackIDs []string) *Entities {
	var todo []string
	for _, id := range trackIDs {
		if _, ok := e.TrackByID[id]; !ok {
			todo = append(todo, id)
		}
	}
	if len(todo) == 0 {
		return e
	}
	c := e.clone()
	c.addTracks(in, todo)
	return c
}

// addTracks resolves tracks with real entities first, so provisional
// names can unify with the real artists and albums of this batch.
func (e *Entities) addTracks(in *ResolverInput, ids []string) {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var later []string
	for _, id := range ids {
		if !e.addReal(in, id) {
			later = append(later, id)
		}
	}
	for _, id := range later {
		e.addProvisional(in, id)
	}
}

// addReal resolves a track known to the catalog or a live row.
func (e *Entities) addReal(in *ResolverInput, id string) bool {
	ct, inCat := in.Tracks[id]
	lt, inLive := in.Live[id]
	catArtists := in.TrackArtists[id]
	if !inLive && (!inCat || len(catArtists) == 0) {
		return false
	}
	t := Track{ID: id, Album: -1}
	t.Name = firstNonEmpty(ct.Name, lt.Name, in.Export[id].Track, id)
	t.DurationMs = ct.DurationMs
	if t.DurationMs == 0 {
		t.DurationMs = lt.DurationMs
	}
	t.Explicit = ct.Explicit
	if t.Explicit == Unknown {
		t.Explicit = lt.Explicit
	}
	t.Hydrated = inCat && ct.Hydrated
	switch {
	case t.Hydrated:
		t.Meta = MetaSpotify
	case inCat && ct.Fallback:
		t.Meta = MetaFallback
	case t.DurationMs > 0:
		// A length without a fallback match came from Spotify: a live
		// track object, or a catalog row the poller seeded from one (D24).
		t.Meta = MetaSpotify
	}
	if len(catArtists) > 0 {
		for _, a := range catArtists {
			t.Artists = appendUnique(t.Artists, e.realArtist(in, a, ""))
		}
	} else {
		for _, a := range lt.Artists {
			t.Artists = appendUnique(t.Artists, e.liveArtist(in, a))
		}
	}
	switch {
	case ct.AlbumID != "":
		t.Album = e.realAlbum(in, ct.AlbumID, lt.Album, t.Artists)
	case lt.Album.ID != "":
		t.Album = e.realAlbum(in, lt.Album.ID, lt.Album, t.Artists)
	case lt.Album.Name != "":
		var artist string
		if len(t.Artists) > 0 {
			artist = e.Artists[t.Artists[0]].Name
		}
		t.Album = e.provisionalAlbum(in, artist, lt.Album.Name, t.Artists)
	}
	e.addTrack(t)
	return true
}

// addProvisional resolves an export-only track by its names.
func (e *Entities) addProvisional(in *ResolverInput, id string) {
	n := in.Export[id]
	t := Track{ID: id, Album: -1}
	t.Name = firstNonEmpty(in.Tracks[id].Name, n.Track, id)
	t.DurationMs = in.Tracks[id].DurationMs
	if in.Tracks[id].Fallback {
		t.Meta = MetaFallback
	}
	if n.Artist != "" {
		t.Artists = []int32{e.provisionalArtist(in, n.Artist)}
	}
	if n.Album != "" {
		t.Album = e.provisionalAlbum(in, n.Artist, n.Album, t.Artists)
	}
	e.addTrack(t)
}

func (e *Entities) addTrack(t Track) {
	for _, a := range t.Artists {
		for _, g := range e.Artists[a].Genres {
			t.Genres = appendUnique(t.Genres, g)
		}
	}
	e.TrackByID[t.ID] = int32(len(e.Tracks))
	e.Tracks = append(e.Tracks, t)
}

// realArtist returns the entity of a Spotify artist ID.
func (e *Entities) realArtist(in *ResolverInput, id, liveName string) int32 {
	if i, ok := e.ArtistByID[id]; ok {
		return i
	}
	ca := in.Artists[id]
	a := Artist{ID: id, Name: firstNonEmpty(ca.Name, liveName, id), ImageURL: ca.ImageURL, Hydrated: ca.Hydrated}
	for _, g := range in.ArtistGenres[id] {
		a.Genres = appendUnique(a.Genres, e.genre(g))
	}
	switch {
	case a.Hydrated:
		a.Meta = MetaSpotify
	case a.ImageURL != "":
		// Only oEmbed writes an unhydrated Spotify artist's image (D27).
		a.Meta = MetaFallback
	}
	i := int32(len(e.Artists))
	e.Artists = append(e.Artists, a)
	e.ArtistByID[id] = i
	key := NormName(a.Name)
	e.realByName[key] = append(slices.Clip(e.realByName[key]), i)
	if _, ok := e.provArtist[key]; ok {
		e.Stale = true
	}
	return i
}

// liveArtist resolves an artist of a live track object (local files
// have artists without IDs).
func (e *Entities) liveArtist(in *ResolverInput, a LiveArtist) int32 {
	if a.ID == "" {
		return e.provisionalArtist(in, a.Name)
	}
	return e.realArtist(in, a.ID, a.Name)
}

// provisionalArtist returns the artist a name-only credit stands for: the
// one real artist of that name, else a provisional entity with what the
// fallback sources found for that name (D27).
func (e *Entities) provisionalArtist(in *ResolverInput, name string) int32 {
	key := NormName(name)
	if real := e.realByName[key]; len(real) == 1 {
		e.ArtistByID[ProvArtistID(name)] = real[0]
		return real[0]
	}
	if i, ok := e.provArtist[key]; ok {
		return i
	}
	i := int32(len(e.Artists))
	id := ProvArtistID(name)
	a := Artist{ID: id, Name: strings.Trim(name, " "), ImageURL: in.FallbackArtists[id].ImageURL, Provisional: true}
	for _, g := range in.ArtistGenres[id] {
		a.Genres = appendUnique(a.Genres, e.genre(g))
	}
	if a.ImageURL != "" || len(a.Genres) > 0 {
		a.Meta = MetaFallback
	}
	e.Artists = append(e.Artists, a)
	e.provArtist[key] = i
	e.ArtistByID[e.Artists[i].ID] = i
	return i
}

func (e *Entities) realAlbum(in *ResolverInput, id string, live LiveAlbum, trackArtists []int32) int32 {
	if i, ok := e.AlbumByID[id]; ok {
		return i
	}
	ca, inCat := in.Albums[id]
	if live.ID != id {
		live = LiveAlbum{}
	}
	a := Album{ID: id, Name: firstNonEmpty(ca.Name, live.Name, id), ImageURL: firstNonEmpty(ca.ImageURL, live.ImageURL),
		Type: firstNonEmpty(ca.Type, live.Type), ReleaseDate: firstNonEmpty(ca.ReleaseDate, live.ReleaseDate),
		TotalTracks: ca.TotalTracks, Hydrated: inCat && ca.Hydrated}
	if a.TotalTracks == 0 {
		a.TotalTracks = live.TotalTracks
	}
	a.ReleaseYear = releaseYear(a.ReleaseDate)
	if a.Hydrated || a.ImageURL != "" || a.ReleaseDate != "" {
		a.Meta = MetaSpotify
	}
	switch {
	case len(in.AlbumArtists[id]) > 0:
		for _, x := range in.AlbumArtists[id] {
			a.Artists = appendUnique(a.Artists, e.realArtist(in, x, ""))
		}
	case len(live.Artists) > 0:
		for _, x := range live.Artists {
			a.Artists = appendUnique(a.Artists, e.liveArtist(in, x))
		}
	case len(trackArtists) > 0:
		a.Artists = trackArtists[:1:1]
	}
	i := int32(len(e.Albums))
	e.Albums = append(e.Albums, a)
	e.AlbumByID[id] = i
	for _, x := range a.Artists {
		e.artistAlbums[x] = append(slices.Clip(e.artistAlbums[x]), i)
	}
	return i
}

// provisionalAlbum returns the album a name-only album stands for: the one
// real album of that name by an artist of that name, else a provisional
// entity with what the fallback sources found for it (D27).
func (e *Entities) provisionalAlbum(in *ResolverInput, artist, album string, artists []int32) int32 {
	key := NormName(artist) + "\x00" + NormName(album)
	if i, ok := e.provAlbum[key]; ok {
		return i
	}
	id := ProvAlbumID(artist, album)
	// Unify with the one real album of that name credited to the artist.
	name := NormName(album)
	var match []int32
	for _, a := range artists {
		for _, al := range e.artistAlbums[a] {
			if NormName(e.Albums[al].Name) == name {
				match = appendUnique(match, al)
			}
		}
	}
	if len(match) == 1 {
		e.provAlbum[key] = match[0]
		e.AlbumByID[id] = match[0]
		return match[0]
	}
	i := int32(len(e.Albums))
	fa := in.FallbackAlbums[id]
	a := Album{ID: id, Name: strings.Trim(album, " "), ImageURL: fa.ImageURL, Type: fa.Type, ReleaseDate: fa.ReleaseDate,
		ReleaseYear: releaseYear(fa.ReleaseDate), TotalTracks: fa.TotalTracks, Provisional: true}
	if a.ImageURL != "" || a.Type != "" || a.ReleaseDate != "" || a.TotalTracks > 0 {
		a.Meta = MetaFallback
	}
	if len(artists) > 0 {
		a.Artists = artists[:1:1]
	}
	e.Albums = append(e.Albums, a)
	e.provAlbum[key] = i
	e.AlbumByID[id] = i
	return i
}

func (e *Entities) genre(name string) int32 {
	id := GenreID(name)
	if i, ok := e.GenreByID[id]; ok {
		if e.Genres[i].Name == name {
			return i
		}
		// Two names with one slug: suffix the later one.
		for n := 2; ; n++ {
			alt := id + "-" + strconv.Itoa(n)
			if i, ok := e.GenreByID[alt]; !ok || e.Genres[i].Name == name {
				id = alt
				break
			}
		}
		if i, ok := e.GenreByID[id]; ok {
			return i
		}
	}
	i := int32(len(e.Genres))
	e.Genres = append(e.Genres, Genre{ID: id, Name: name})
	e.GenreByID[id] = i
	return i
}

// context returns the index of a context URI, adding it if new.
func (e *Entities) context(in *ResolverInput, uri string) int32 {
	if i, ok := e.ContextByID[uri]; ok {
		return i
	}
	i := int32(len(e.Contexts))
	e.Contexts = append(e.Contexts, e.makeContext(in, uri))
	e.ContextByID[uri] = i
	return i
}

// makeContext names a context: the catalog's contexts row wins (playlist
// names, manual names), else album and artist contexts take their
// entity's name, and Liked Songs is named.
func (e *Entities) makeContext(in *ResolverInput, uri string) Context {
	c := ParseContextURI(uri)
	switch c.Type {
	case "album":
		if i, ok := e.AlbumByID[c.ID]; ok {
			c.Album = i
			c.Name, c.HasName, c.NameSource = e.Albums[i].Name, true, "catalog"
			c.ImageURL = e.Albums[i].ImageURL
		}
	case "artist":
		if i, ok := e.ArtistByID[c.ID]; ok {
			c.Artist = i
			c.Name, c.HasName, c.NameSource = e.Artists[i].Name, true, "catalog"
			c.ImageURL = e.Artists[i].ImageURL
		}
	case "collection":
		c.Name, c.HasName, c.NameSource = "Liked Songs", true, "catalog"
	}
	if cc, ok := in.Contexts[uri]; ok {
		if cc.HasName || cc.NameSource != "unresolved" || !c.HasName {
			c.Name, c.HasName, c.NameSource = cc.Name, cc.HasName, cc.NameSource
		}
		if cc.ImageURL != "" {
			c.ImageURL = cc.ImageURL
		}
	}
	return c
}

// spotifyOwnedPrefix starts the IDs of Spotify-made playlists (Daily Mix,
// Discover Weekly, ...), which apps cannot read (D17).
const spotifyOwnedPrefix = "37i9dQZ"

// ParseContextURI splits a context URI (spotify:playlist:ID,
// spotify:user:NAME:collection, ...) into type and ID, unnamed.
func ParseContextURI(uri string) Context {
	parts := strings.Split(uri, ":")
	c := Context{URI: uri, Type: "other", ID: parts[len(parts)-1], NameSource: "unresolved", Album: -1, Artist: -1}
	switch {
	case len(parts) == 3 && (parts[1] == "playlist" || parts[1] == "album" || parts[1] == "artist"):
		c.Type = parts[1]
	case parts[len(parts)-1] == "collection":
		c.Type = "collection"
	}
	c.SpotifyOwned = c.Type == "playlist" && strings.HasPrefix(c.ID, spotifyOwnedPrefix)
	return c
}

func (e *Entities) device(name, typ string) int16 {
	if name == "" {
		return -1
	}
	d := Device{name, typ}
	if i, ok := e.deviceKey[d]; ok {
		return i
	}
	i := int16(len(e.Devices))
	e.Devices = append(e.Devices, d)
	e.deviceKey[d] = i
	return i
}

func (e *Entities) platform(name string) int16 {
	if name == "" {
		return -1
	}
	if i, ok := e.platformKey[name]; ok {
		return i
	}
	i := int16(len(e.Platforms))
	e.Platforms = append(e.Platforms, name)
	e.platformKey[name] = i
	return i
}

func releaseYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil || y <= 0 {
		return 0
	}
	return y
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}

func appendUnique[T comparable](s []T, v T) []T {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}
