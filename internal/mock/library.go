package mock

import (
	"context"
	"fmt"
	"slices"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/libview"
	"spotussi/internal/query"
	"spotussi/internal/refdata"
	"spotussi/internal/spotify"
)

// Library endpoints over the dataset's synthetic library (D36), by naive
// scans. A saved track matches only its own ID here: the dataset has no
// ISRCs or relinks.

// SyncTookMs is how long each synthetic library sync ran.
const SyncTookMs = 4_000

func (b *Backend) nTracks() int32 { return int32(len(b.ds.Tracks)) }

// libExcluded reports whether a library track is hidden by exclusions.
func (b *Backend) libExcluded(t int32) bool {
	if t < b.nTracks() && b.exclTracks[t] {
		return true
	}
	return slices.ContainsFunc(b.ds.LibTrack(t).Artists, func(a int32) bool { return b.exclArtists[a] })
}

func (b *Backend) libTrackRef(t int32) api.TrackRef {
	if t < b.nTracks() {
		return b.trackRef(t)
	}
	x := b.ds.LibTrack(t)
	al := b.ds.Albums[x.Album]
	return api.TrackRef{ID: x.ID, Name: x.Name, Artists: b.artistLites(x.Artists),
		Album: &api.AlbumLite{ID: al.ID, Name: al.Name}, ImageURL: b.albumImage(x.Album), DurationMs: ptr(int64(x.DurationMs))}
}

func (b *Backend) libMatchKey(t int32) string {
	x := b.ds.LibTrack(t)
	main := ""
	if len(x.Artists) > 0 {
		main = b.ds.Artists[x.Artists[0]].Name
	}
	return libview.MatchKey(x.Name, main)
}

// libPlays is the plays of every library track, by one scan.
type libPlays struct {
	starts []libview.Starts // by track index (history tracks only)
	played map[string]bool  // match keys of tracks with counted plays
}

func (b *Backend) libPlays() *libPlays {
	lp := &libPlays{starts: make([]libview.Starts, len(b.ds.Tracks)), played: map[string]bool{}}
	for _, i := range b.vis {
		p := b.ds.Plays[i]
		if b.counted[i] {
			lp.starts[p.Track].Add(p.StartMs, int64(p.MsPlayed))
		} else {
			lp.starts[p.Track].AddMs(int64(p.MsPlayed))
		}
	}
	for t, n := range b.trackPlays {
		if n > 0 {
			lp.played[b.libMatchKey(int32(t))] = true
		}
	}
	return lp
}

func (b *Backend) libTrack(lp *libPlays, t int32, addedAt int64) libview.Track {
	out := libview.Track{Ref: b.libTrackRef(t), AddedAt: addedAt}
	var s libview.Starts
	if t < b.nTracks() {
		s.Merge(&lp.starts[t])
	}
	s.Fill(&out)
	out.Status = libview.Status(out.Plays, lp.played[b.libMatchKey(t)])
	return out
}

func (b *Backend) savedTracks() []libview.Track {
	lp := b.libPlays()
	var out []libview.Track
	for _, s := range b.ds.Library.Tracks {
		if s.Present() && !b.libExcluded(s.Item) {
			out = append(out, b.libTrack(lp, s.Item, s.AddedAt))
		}
	}
	return out
}

func present(xs []refdata.Saved) int {
	n := 0
	for _, s := range xs {
		if s.Present() {
			n++
		}
	}
	return n
}

func (b *Backend) lastLibrarySync() int64 {
	sy := b.ds.Library.Syncs
	if len(sy) == 0 {
		return 0
	}
	return sy[len(sy)-1] + SyncTookMs
}

func (b *Backend) LibrarySummary(ctx context.Context) (*api.LibrarySummary, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return libview.Summary(b.savedTracks(), present(b.ds.Library.Albums), len(b.ds.Library.Playlists), b.lastLibrarySync()), nil
}

func (b *Backend) LibraryTracks(ctx context.Context, p api.LibraryTracksParams) (*api.LibraryTracks, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return libview.Tracks(b.savedTracks(), p), nil
}

func (b *Backend) LibraryAlbums(ctx context.Context, p api.LibraryAlbumsParams) (*api.LibraryAlbums, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []libview.Album
	for _, s := range b.ds.Library.Albums {
		if !s.Present() {
			continue
		}
		v := libview.Album{Ref: b.albumRef(s.Item), AddedAt: s.AddedAt}
		if b.albumMeta[s.Item] != derive.MetaPending {
			v.TracksTotal = b.ds.Albums[s.Item].TotalTracks
		}
		played := map[int32]bool{}
		for _, i := range b.vis {
			p := b.ds.Plays[i]
			if b.ds.Tracks[p.Track].Album != s.Item {
				continue
			}
			v.Ms += int64(p.MsPlayed)
			if b.counted[i] {
				v.Plays++
				played[p.Track] = true
			}
		}
		v.TracksPlayed = len(played)
		out = append(out, v)
	}
	return libview.Albums(out, p), nil
}

func (b *Backend) libPlaylist(lp *libPlays, pl *refdata.Playlist) libview.Playlist {
	v := libview.Playlist{ItemsTotal: pl.Total, ItemsAvailable: pl.ItemsAvailable,
		Ref: api.LibraryPlaylistRef{ID: pl.ID, URI: libview.PlaylistURI(pl.ID), Name: pl.Name, OwnerName: ptr(pl.OwnerName),
			Relation: pl.Relation, SpotifyOwned: libview.SpotifyOwned(pl.ID)}}
	if !v.Ref.SpotifyOwned {
		v.Ref.ImageURL = imageURL("playlist", pl.ID)
	}
	if pl.Context >= 0 {
		for _, i := range b.vis {
			p := b.ds.Plays[i]
			if p.Context != pl.Context {
				continue
			}
			v.MsHere += int64(p.MsPlayed)
			if b.counted[i] {
				v.PlaysHere++
				v.LastHere = max(v.LastHere, p.StartMs)
			}
		}
	}
	if pl.ItemsAvailable {
		seen := map[int32]bool{}
		for _, s := range pl.Items {
			if !s.Present() || seen[s.Item] || b.libExcluded(s.Item) {
				continue
			}
			seen[s.Item] = true
			v.Tracks++
			if s.Item < b.nTracks() && b.trackPlays[s.Item] > 0 {
				v.TracksPlayed++
			}
		}
	}
	return v
}

func (b *Backend) LibraryPlaylists(ctx context.Context, p api.LibraryPlaylistsParams) (*api.LibraryPlaylists, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	lp := b.libPlays()
	var out []libview.Playlist
	for i := range b.ds.Library.Playlists {
		out = append(out, b.libPlaylist(lp, &b.ds.Library.Playlists[i]))
	}
	return libview.Playlists(out, p), nil
}

func (b *Backend) LibraryPlaylist(ctx context.Context, id string, pg api.Page) (*api.LibraryPlaylistDetail, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	k := slices.IndexFunc(b.ds.Library.Playlists, func(pl refdata.Playlist) bool { return pl.ID == id })
	if k < 0 {
		return nil, fmt.Errorf("%w: playlist %q", api.ErrNotFound, id)
	}
	pl := &b.ds.Library.Playlists[k]
	lp := b.libPlays()
	v := b.libPlaylist(lp, pl)
	here := map[int32]int64{}
	if pl.Context >= 0 {
		for _, i := range b.vis {
			if p := b.ds.Plays[i]; p.Context == pl.Context && b.counted[i] {
				here[p.Track]++
			}
		}
	}
	var items []libview.Item
	pos := 0
	for _, s := range pl.Items {
		if !s.Present() {
			continue
		}
		if !b.libExcluded(s.Item) {
			items = append(items, libview.Item{Position: pos, Track: b.libTrack(lp, s.Item, s.AddedAt), AddedBy: s.AddedBy,
				AddedByOwner: s.AddedBy == refdata.Me, PlaysHere: here[s.Item]})
		}
		pos++
	}
	return libview.PlaylistDetail(&v, items, pg), nil
}

func (b *Backend) LibraryTimeline(ctx context.Context, spec query.Spec, bucket query.Bucket) (*api.LibraryTimeline, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cur, _, err := b.resolve(spec)
	if err != nil {
		return nil, err
	}
	lib := &b.ds.Library
	var events []libview.Event
	add := func(xs []refdata.Saved, saved, removed int) {
		for _, s := range xs {
			if s.AddedAt > 0 {
				events = append(events, libview.Event{At: s.AddedAt, Kind: saved})
			}
			if !s.Present() {
				events = append(events, libview.Event{At: lib.Syncs[s.Gone], Kind: removed})
			}
		}
	}
	add(lib.Tracks, libview.TrackSaved, libview.TrackRemoved)
	add(lib.Albums, libview.AlbumSaved, libview.AlbumRemoved)
	for _, pl := range lib.Playlists {
		add(pl.Items, libview.PlaylistAdd, libview.PlaylistRemoval)
	}
	return libview.Timeline(cur, bucket, b.weekStart(), events)
}

// SyncLibrary pretends to start a sync: the synthetic library never
// changes.
func (b *Backend) SyncLibrary(ctx context.Context) (*api.LibraryStatus, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	st := b.libraryStatus()
	st.State = "syncing"
	st.Progress = ptr(0.0)
	return &st, nil
}

func (b *Backend) libraryStatus() api.LibraryStatus {
	lib := &b.ds.Library
	ok := api.LibraryListStatus{State: "ok"}
	st := api.LibraryStatus{State: "idle", Lists: api.LibraryLists{Tracks: ok, Albums: ok, Playlists: ok},
		SavedTracks: present(lib.Tracks), SavedAlbums: present(lib.Albums), Playlists: len(lib.Playlists)}
	for _, pl := range lib.Playlists {
		if pl.ItemsAvailable {
			st.PlaylistsWithItems++
		}
	}
	if last := b.lastLibrarySync(); last > 0 {
		st.LastSyncAt, st.LastFullAt = ptr(last), ptr(last)
		st.NextSyncAt = ptr(max(last+3_600_000, b.clock().UnixMilli()+60_000))
	}
	return st
}

// savedAt is when a track was saved, if it is saved now.
func (b *Backend) savedAt(t int32) *int64 {
	for _, s := range b.ds.Library.Tracks {
		if s.Item == t && s.Present() && s.AddedAt > 0 {
			return ptr(s.AddedAt)
		}
	}
	return nil
}

var mockScopes = spotify.Scopes
