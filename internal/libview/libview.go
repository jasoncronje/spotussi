// Package libview shapes library data into the /library responses (D36):
// play status, heavy rotation, sorting, paging and timeline buckets. The
// live backend and the mock oracle each gather the facts their own way
// (the derive snapshot and the library tables, or a naive scan of the
// reference dataset) and hand them here, so the differential test
// compares what they gathered and both answer with the same rules.
package libview

import (
	"cmp"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/query"
)

// Track is a saved track (or a playlist item) with its plays: the
// counted, visible plays of every track it matches (same ID, relinked or
// same ISRC). Times are epoch ms, 0 when unknown.
type Track struct {
	Ref     api.TrackRef
	AddedAt int64
	// Status is api.StatusPlayed, StatusMaybePlayed or StatusNeverPlayed.
	Status   string
	Plays    int64
	Ms       int64
	First    int64
	Last     int64
	Rotation int64 // start of the RotationPlays-th counted play
	// FirstAfter is the first counted play at or after the save.
	FirstAfter int64
}

// Wire is the API item.
func (t *Track) Wire() api.LibraryTrack {
	return api.LibraryTrack{Track: t.Ref, AddedAt: ptr(t.AddedAt), Status: t.Status, Plays: t.Plays, MsPlayed: t.Ms,
		FirstPlayedAt: ptr(t.First), LastPlayedAt: ptr(t.Last), RotationAt: ptr(t.Rotation)}
}

func ptr(ms int64) *int64 {
	if ms == 0 {
		return nil
	}
	return &ms
}

// Starts collects counted play starts of matched tracks; Stats turns
// them into a Track's play fields.
type Starts struct {
	starts []int64
	ms     int64
}

// Add records a counted play.
func (s *Starts) Add(start, ms int64) {
	s.starts = append(s.starts, start)
	s.ms += ms
}

// Merge adds o's plays to s.
func (s *Starts) Merge(o *Starts) {
	s.starts = append(s.starts, o.starts...)
	s.ms += o.ms
}

// AddMs records time of a visible play that did not count.
func (s *Starts) AddMs(ms int64) { s.ms += ms }

// Fill sets t's plays, time and play dates (t.AddedAt must be set).
func (s *Starts) Fill(t *Track) {
	slices.Sort(s.starts)
	t.Plays, t.Ms = int64(len(s.starts)), s.ms
	if n := len(s.starts); n > 0 {
		t.First, t.Last = s.starts[0], s.starts[n-1]
		if n >= api.RotationPlays {
			t.Rotation = s.starts[api.RotationPlays-1]
		}
		if t.AddedAt > 0 {
			if k, _ := slices.BinarySearch(s.starts, t.AddedAt); k < n {
				t.FirstAfter = s.starts[k]
			}
		}
	}
}

// MatchKey is the name key of "maybe played": normalized title and main
// artist.
func MatchKey(title, mainArtist string) string {
	return derive.NormName(title) + "\x00" + derive.NormName(mainArtist)
}

// Status decides a saved track's play status.
func Status(plays int64, sameName bool) string {
	switch {
	case plays > 0:
		return api.StatusPlayed
	case sameName:
		return api.StatusMaybePlayed
	}
	return api.StatusNeverPlayed
}

const dayMs = 24 * 60 * 60 * 1000

// Summary builds /library/summary from the saved tracks.
func Summary(tracks []Track, albums, playlists int, lastSync int64) *api.LibrarySummary {
	out := &api.LibrarySummary{LastSyncAt: ptr(lastSync), SavedTracks: len(tracks), SavedAlbums: albums,
		Playlists: playlists, RotationPlays: api.RotationPlays}
	var first, rot []float64
	for i := range tracks {
		t := &tracks[i]
		switch t.Status {
		case api.StatusPlayed:
			out.Tracks.Played++
		case api.StatusMaybePlayed:
			out.Tracks.MaybePlayed++
		default:
			out.Tracks.NeverPlayed++
		}
		if t.FirstAfter > 0 {
			first = append(first, float64(t.FirstAfter-t.AddedAt)/dayMs)
		}
		m := &out.RotationMix
		switch {
		case t.Rotation == 0:
			m.NotYet++
		case t.AddedAt == 0 || t.Rotation < t.AddedAt:
			m.BeforeSaving++
		default:
			d := t.Rotation - t.AddedAt
			rot = append(rot, float64(d)/dayMs)
			switch {
			case d <= dayMs:
				m.WithinDay++
			case d <= 7*dayMs:
				m.WithinWeek++
			case d <= 30*dayMs:
				m.WithinMonth++
			default:
				m.Later++
			}
		}
	}
	out.DaysToFirstPlay = days(first)
	out.DaysToRotation = days(rot)
	return out
}

func days(xs []float64) api.LibraryDays {
	if len(xs) == 0 {
		return api.LibraryDays{}
	}
	slices.Sort(xs)
	n := len(xs)
	m := xs[n/2]
	if n%2 == 0 {
		m = (xs[n/2-1] + xs[n/2]) / 2
	}
	m = math.Round(m*10) / 10
	return api.LibraryDays{Median: &m, Count: n}
}

// Tracks filters, sorts and pages saved tracks (tracks is reordered).
func Tracks(tracks []Track, p api.LibraryTracksParams) *api.LibraryTracks {
	out := &api.LibraryTracks{Offset: p.Page.Offset, Limit: p.Page.Limit, Items: []api.LibraryTrack{}}
	out.Counts.All = len(tracks)
	keep := tracks[:0:0]
	for i := range tracks {
		switch tracks[i].Status {
		case api.StatusPlayed:
			out.Counts.Played++
		case api.StatusMaybePlayed:
			out.Counts.MaybePlayed++
		default:
			out.Counts.NeverPlayed++
		}
		if p.Played == "" || p.Played == "all" || tracks[i].Status == p.Played {
			keep = append(keep, tracks[i])
		}
	}
	SortTracks(keep, p.Sort)
	out.Total = len(keep)
	for _, t := range page(keep, p.Page) {
		out.Items = append(out.Items, t.Wire())
	}
	return out
}

// SortTracks orders tracks: added (newest save first, unknown last),
// plays or last_played (most recent first, never last); ties by track ID.
func SortTracks(ts []Track, by string) {
	slices.SortStableFunc(ts, func(a, b Track) int {
		var c int
		switch by {
		case "plays":
			c = cmp.Compare(b.Plays, a.Plays)
		case "last_played":
			c = cmp.Compare(b.Last, a.Last)
		default:
			c = cmp.Compare(b.AddedAt, a.AddedAt)
		}
		if c != 0 {
			return c
		}
		return strings.Compare(a.Ref.ID, b.Ref.ID)
	})
}

func page[T any](xs []T, p api.Page) []T {
	lo := min(p.Offset, len(xs))
	hi := min(lo+p.Limit, len(xs))
	return xs[lo:hi]
}

// Album is a saved album with its plays.
type Album struct {
	Ref          api.AlbumRef
	AddedAt      int64
	TracksTotal  int // 0: unknown
	TracksPlayed int
	Plays, Ms    int64
}

// Albums sorts and pages saved albums (albums is reordered).
func Albums(albums []Album, p api.LibraryAlbumsParams) *api.LibraryAlbums {
	share := func(a Album) float64 {
		if a.TracksTotal <= 0 {
			return -1
		}
		return float64(a.TracksPlayed) / float64(a.TracksTotal)
	}
	slices.SortStableFunc(albums, func(a, b Album) int {
		var c int
		switch p.Sort {
		case "plays":
			c = cmp.Compare(b.Plays, a.Plays)
		case "played_share":
			c = cmp.Compare(share(b), share(a))
		default:
			c = cmp.Compare(b.AddedAt, a.AddedAt)
		}
		if c != 0 {
			return c
		}
		return strings.Compare(a.Ref.ID, b.Ref.ID)
	})
	out := &api.LibraryAlbums{Total: len(albums), Offset: p.Page.Offset, Limit: p.Page.Limit, Items: []api.LibraryAlbum{}}
	for _, a := range page(albums, p.Page) {
		w := api.LibraryAlbum{Album: a.Ref, AddedAt: ptr(a.AddedAt), TracksPlayed: a.TracksPlayed, Plays: a.Plays, MsPlayed: a.Ms}
		if a.TracksTotal > 0 {
			n := a.TracksTotal
			w.TracksTotal = &n
		}
		out.Items = append(out.Items, w)
	}
	return out
}

// Playlist is a playlist with its plays. Items is nil when the items are
// not synced.
type Playlist struct {
	Ref            api.LibraryPlaylistRef
	ItemsTotal     int // -1: unknown
	ItemsAvailable bool
	// Tracks: different tracks present, and those ever played.
	Tracks, TracksPlayed int
	PlaysHere, MsHere    int64
	LastHere             int64
}

// Wire is the API item.
func (p *Playlist) Wire() api.LibraryPlaylist {
	w := api.LibraryPlaylist{Playlist: p.Ref, ItemsAvailable: p.ItemsAvailable, PlaysHere: p.PlaysHere, MsHere: p.MsHere,
		LastPlayedHereAt: ptr(p.LastHere)}
	if p.ItemsTotal >= 0 {
		n := p.ItemsTotal
		w.ItemsTotal = &n
	}
	if p.ItemsAvailable {
		t, tp := p.Tracks, p.TracksPlayed
		w.Tracks, w.TracksPlayed = &t, &tp
	}
	return w
}

// Playlists filters, sorts and pages playlists (ps is reordered).
func Playlists(ps []Playlist, p api.LibraryPlaylistsParams) *api.LibraryPlaylists {
	keep := ps[:0:0]
	for _, pl := range ps {
		if p.Relation == "" || p.Relation == "all" || pl.Ref.Relation == p.Relation {
			keep = append(keep, pl)
		}
	}
	share := func(pl Playlist) float64 {
		if !pl.ItemsAvailable || pl.Tracks == 0 {
			return -1
		}
		return float64(pl.TracksPlayed) / float64(pl.Tracks)
	}
	slices.SortStableFunc(keep, func(a, b Playlist) int {
		var c int
		switch p.Sort {
		case "name":
			c = strings.Compare(derive.NormName(a.Ref.Name), derive.NormName(b.Ref.Name))
		case "played_share":
			c = cmp.Compare(share(b), share(a))
		default:
			c = cmp.Compare(b.PlaysHere, a.PlaysHere)
		}
		if c != 0 {
			return c
		}
		return strings.Compare(a.Ref.ID, b.Ref.ID)
	})
	out := &api.LibraryPlaylists{Total: len(keep), Offset: p.Page.Offset, Limit: p.Page.Limit, Items: []api.LibraryPlaylist{}}
	for _, pl := range page(keep, p.Page) {
		out.Items = append(out.Items, pl.Wire())
	}
	return out
}

// Item is a playlist item.
type Item struct {
	Position     int
	Track        Track
	AddedBy      string
	AddedByOwner bool
	PlaysHere    int64
}

// PlaylistDetail pages a playlist's items (in position order).
func PlaylistDetail(pl *Playlist, items []Item, pg api.Page) *api.LibraryPlaylistDetail {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Position < items[j].Position })
	out := &api.LibraryPlaylistDetail{Playlist: pl.Wire(), Total: len(items), Offset: pg.Offset, Limit: pg.Limit,
		Items: []api.LibraryPlaylistItem{}}
	for _, it := range page(items, pg) {
		w := api.LibraryPlaylistItem{Position: it.Position, Track: it.Track.Ref, AddedAt: ptr(it.Track.AddedAt),
			AddedByOwner: it.AddedByOwner, Status: it.Track.Status, Plays: it.Track.Plays, PlaysHere: it.PlaysHere}
		if it.AddedBy != "" {
			by := it.AddedBy
			w.AddedBy = &by
		}
		out.Items = append(out.Items, w)
	}
	return out
}

// Event kinds of the library timeline.
const (
	TrackSaved = iota
	TrackRemoved
	AlbumSaved
	AlbumRemoved
	PlaylistAdd
	PlaylistRemoval
)

// Event is something that happened to the library at At (epoch ms).
type Event struct {
	At   int64
	Kind int
}

// Timeline buckets events over a resolved range.
func Timeline(r query.Range, b query.Bucket, weekStart time.Weekday, events []Event) (*api.LibraryTimeline, error) {
	bk, spans, err := query.Buckets(r, b, weekStart)
	if err != nil {
		return nil, err
	}
	out := &api.LibraryTimeline{Range: WireRange(r), Bucket: string(bk), Points: make([]api.LibraryTimelinePoint, len(spans))}
	for i, s := range spans {
		out.Points[i] = api.LibraryTimelinePoint{Key: s.Key, StartAt: s.Start.UnixMilli()}
	}
	start, end := r.Start.UnixMilli(), r.End.UnixMilli()
	for _, e := range events {
		if e.At < start || e.At >= end {
			continue
		}
		k := sort.Search(len(spans), func(i int) bool { return spans[i].End.UnixMilli() > e.At })
		if k == len(spans) {
			continue
		}
		p := &out.Points[k]
		switch e.Kind {
		case TrackSaved:
			p.TracksSaved++
		case TrackRemoved:
			p.TracksRemoved++
		case AlbumSaved:
			p.AlbumsSaved++
		case AlbumRemoved:
			p.AlbumsRemoved++
		case PlaylistAdd:
			p.PlaylistAdds++
		case PlaylistRemoval:
			p.PlaylistRemovals++
		}
	}
	return out, nil
}

// WireRange is a resolved range in API form.
func WireRange(r query.Range) api.ResolvedRange {
	w := api.ResolvedRange{Preset: string(r.Preset), From: r.From, To: r.To,
		StartAt: r.Start.UnixMilli(), EndAt: r.End.UnixMilli(), Timezone: r.Loc.String()}
	if d := r.FirstPlayDate(); d != "" {
		w.FirstPlayDate = &d
	}
	return w
}

// SpotifyOwned reports whether a playlist ID is Spotify-made (D17).
func SpotifyOwned(id string) bool { return strings.HasPrefix(id, "37i9dQZ") }

// PlaylistURI is a playlist's context URI.
func PlaylistURI(id string) string { return "spotify:playlist:" + id }
