package refdata

import (
	"math/rand/v2"
	"slices"
	"strconv"
)

// Me is the synthetic listener's Spotify account ID.
const Me = "listener"

// Library is the synthetic Spotify library (D36): saved tracks and
// albums and the listener's playlists, as library sync would have stored
// them. It is generated after the history with its own random stream,
// so the history is the same with or without it.
type Library struct {
	// Extra are never-played tracks (on played albums, by played artists)
	// that are saved or on playlists. Track index len(Tracks)+i; see
	// Dataset.LibTrack.
	Extra []Track
	// Syncs are sync start times, oldest first. A removal is seen at a
	// sync.
	Syncs     []int64
	Tracks    []Saved // Item: track index
	Albums    []Saved // Item: album index
	Playlists []Playlist
}

// Saved is one stretch of membership of an item in a list.
type Saved struct {
	Item    int32
	AddedAt int64  // 0: unknown
	AddedBy string // playlist items only
	Gone    int    // index into Syncs of the sync that saw it gone; -1 while present
}

// Playlist is a playlist in the listener's /me/playlists.
type Playlist struct {
	ID, Name, OwnerID, OwnerName string
	Relation                     string // own, collaborative, followed
	// Context is the playlist's context index, -1 if never played from.
	Context int32
	// ItemsAvailable: Spotify lets us read the items (own and
	// collaborative playlists).
	ItemsAvailable bool
	Total          int // items as Spotify reports
	Items          []Saved
}

// LibTrack is a track by library index (history tracks, then Extra).
func (ds *Dataset) LibTrack(i int32) *Track {
	if int(i) < len(ds.Tracks) {
		return &ds.Tracks[i]
	}
	return &ds.Library.Extra[int(i)-len(ds.Tracks)]
}

// Present reports whether s is in the list now.
func (s Saved) Present() bool { return s.Gone < 0 }

const libDay = int64(86_400_000)

func (g *gen) library(seed uint64) {
	ds := g.ds
	r := rand.New(rand.NewPCG(seed^0x6c6962726172, 0x5eed0f11b))
	lib := &ds.Library
	nTracks := int32(len(ds.Tracks))

	// Daily syncs over the last 90 days.
	for k := range 90 {
		lib.Syncs = append(lib.Syncs, g.endMs-int64(89-k)*libDay-3_600_000)
	}
	startMs := ds.Start.UnixMilli()
	clamp := func(t int64) int64 { return min(max(t, startMs), lib.Syncs[0]-libDay) }
	goneAt := func() int { return 1 + r.IntN(len(lib.Syncs)-1) }

	first := make([]int64, nTracks)
	rows := make([]int, nTracks)
	for i := range first {
		first[i] = -1
	}
	for _, p := range ds.Plays {
		if first[p.Track] < 0 {
			first[p.Track] = p.StartMs
		}
		rows[p.Track]++
	}

	// Never-played tracks: most new names, some the same song as a
	// played track on another release (maybe played).
	nExtra := max(len(ds.Tracks)/20, 8)
	for i := range nExtra {
		var t Track
		if i%5 == 0 {
			src := ds.Tracks[r.IntN(len(ds.Tracks))]
			t = Track{Name: src.Name, Album: int32(r.IntN(len(ds.Albums))), Artists: slices.Clone(src.Artists)}
		} else {
			al := int32(r.IntN(len(ds.Albums)))
			t = Track{Name: trackName(r), Album: al, Artists: slices.Clone(ds.Albums[al].Artists)}
		}
		t.ID, t.DurationMs, t.Hydrated = spotifyID(r), int32(150_000+r.IntN(150_000)), true
		lib.Extra = append(lib.Extra, t)
	}

	// Saved tracks: often the songs played most, saved around their first
	// play; some saved before ever playing; some removed, a few re-saved.
	for t := range nTracks {
		chance := 0.04
		switch {
		case rows[t] >= 10:
			chance = 0.5
		case rows[t] >= 3:
			chance = 0.2
		}
		if r.Float64() >= chance {
			continue
		}
		at := first[t] + r.Int64N(30*libDay)
		if r.IntN(6) == 0 {
			at = first[t] - r.Int64N(60*libDay) - 1
		}
		s := Saved{Item: t, AddedAt: clamp(at), Gone: -1}
		if r.IntN(12) == 0 {
			s.Gone = goneAt()
			lib.Tracks = append(lib.Tracks, s)
			if r.IntN(3) == 0 && s.Gone < len(lib.Syncs)-1 {
				lib.Tracks = append(lib.Tracks, Saved{Item: t, AddedAt: lib.Syncs[s.Gone] + r.Int64N(libDay/2), Gone: -1})
			}
			continue
		}
		lib.Tracks = append(lib.Tracks, s)
	}
	for i := range lib.Extra {
		if i%3 == 2 {
			continue // only on playlists
		}
		lib.Tracks = append(lib.Tracks, Saved{Item: nTracks + int32(i), AddedAt: clamp(startMs + r.Int64N(lib.Syncs[0]-startMs)), Gone: -1})
	}
	// One saved track whose save date Spotify doesn't report.
	if len(lib.Tracks) > 0 {
		lib.Tracks[len(lib.Tracks)/2].AddedAt = 0
	}

	// Saved albums.
	for al := range ds.Albums {
		if ds.Albums[al].TotalTracks < 3 || r.IntN(8) != 0 {
			continue
		}
		s := Saved{Item: int32(al), AddedAt: clamp(startMs + r.Int64N(lib.Syncs[0]-startMs)), Gone: -1}
		if r.IntN(10) == 0 {
			s.Gone = goneAt()
		}
		lib.Albums = append(lib.Albums, s)
	}

	// Playlists: every user playlist in the history (own, a few
	// collaborative or followed), the Spotify-made ones (followed), and
	// one own playlist never played from.
	fromCtx := map[int32][]int32{}
	seen := map[[2]int32]bool{}
	firstIn := map[[2]int32]int64{}
	for _, p := range ds.Plays {
		if p.Context < 0 {
			continue
		}
		k := [2]int32{p.Context, p.Track}
		if !seen[k] {
			seen[k] = true
			fromCtx[p.Context] = append(fromCtx[p.Context], p.Track)
			firstIn[k] = p.StartMs
		}
	}
	friends := []string{"sam", "alex", "jo"}
	n, made := 0, 0
	for c, x := range ds.Contexts {
		if x.Type != "playlist" {
			continue
		}
		pl := Playlist{ID: x.ID, Name: x.Name, Context: int32(c), OwnerID: Me, OwnerName: "Listener", Relation: "own"}
		switch {
		case x.SpotifyOwned:
			pl.OwnerID, pl.OwnerName, pl.Relation = "spotify", "Spotify", "followed"
			if pl.Name == "" {
				made++
				pl.Name = "Daily Mix " + strconv.Itoa(made+4)
			}
		case n%5 == 3:
			f := friends[n%len(friends)]
			pl.OwnerID, pl.OwnerName, pl.Relation = f, title(f), "collaborative"
		case n%5 == 4:
			f := friends[n%len(friends)]
			pl.OwnerID, pl.OwnerName, pl.Relation = f, title(f), "followed"
		}
		if !x.SpotifyOwned {
			n++
		}
		if pl.Relation == "followed" {
			pl.Total = 20 + r.IntN(60)
			lib.Playlists = append(lib.Playlists, pl)
			continue
		}
		pl.ItemsAvailable = true
		for _, t := range fromCtx[int32(c)] {
			s := Saved{Item: t, AddedAt: clamp(firstIn[[2]int32{int32(c), t}] - r.Int64N(3*libDay) - 1), AddedBy: Me, Gone: -1}
			if pl.Relation == "collaborative" && r.IntN(2) == 0 {
				s.AddedBy = pl.OwnerID
			}
			if r.IntN(15) == 0 {
				s.Gone = goneAt()
			}
			pl.Items = append(pl.Items, s)
		}
		for range r.IntN(4) {
			e := nTracks + int32(r.IntN(len(lib.Extra)))
			if slices.ContainsFunc(pl.Items, func(s Saved) bool { return s.Item == e }) {
				continue
			}
			pl.Items = append(pl.Items, Saved{Item: e, AddedAt: lib.Syncs[0] + r.Int64N(g.endMs-lib.Syncs[0]-libDay), AddedBy: Me, Gone: -1})
		}
		lib.Playlists = append(lib.Playlists, finish(pl))
	}
	quiet := Playlist{ID: spotifyID(r), Name: "Someday " + playlistName(r), Context: -1, OwnerID: Me, OwnerName: "Listener",
		Relation: "own", ItemsAvailable: true}
	for i := range min(6, len(lib.Extra)) {
		quiet.Items = append(quiet.Items, Saved{Item: nTracks + int32(i), AddedAt: lib.Syncs[10] + int64(i)*libDay, AddedBy: Me, Gone: -1})
	}
	quiet.Items = append(quiet.Items, Saved{Item: 0, AddedAt: lib.Syncs[12], AddedBy: Me, Gone: -1})
	lib.Playlists = append(lib.Playlists, finish(quiet))
}

func finish(pl Playlist) Playlist {
	for _, s := range pl.Items {
		if s.Present() {
			pl.Total++
		}
	}
	return pl
}
