package live

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"spotussi/internal/api"
	"spotussi/internal/derive"
	"spotussi/internal/library"
	"spotussi/internal/libview"
	"spotussi/internal/query"
)

// Library endpoints (D36): the library tables (migration 0010) give what
// is saved and when; plays come from the snapshot. A saved track matches
// every track with its ID, a track relinked from or to it, and every
// track with its ISRC (catalog), so a song saved from one release and
// played from another counts as played.

// libTrack is a saved track or playlist item as stored.
type libTrack struct {
	key     string // catalog key: the Spotify ID, or the URI for local files
	addedAt int64
	addedBy string
	pos     int
}

func trackKey(uri string) string {
	if id, ok := strings.CutPrefix(uri, "spotify:track:"); ok {
		return id
	}
	return uri
}

// catTrack is what the catalog knows about a track not in the snapshot.
type catTrack struct {
	name       string
	ref        api.TrackRef
	artistIDs  []string
	mainArtist string
}

// libIndex is the catalog side of the library view for one snapshot:
// ISRCs and relinks of every catalog track, the name keys of played
// tracks, and the catalog rows of library tracks the snapshot lacks.
// Hydration and library sync announce catalog changes, which rebuild the
// snapshot, so a new snapshot gets a fresh index.
type libIndex struct {
	snap    *derive.Snapshot
	isrcOf  map[string]string   // catalog key -> ISRC
	byISRC  map[string][]string // ISRC -> catalog keys
	relinks map[string][]string // relinks, both ways
	played  map[string]bool     // MatchKey of tracks with counted plays

	mu  sync.Mutex
	cat map[string]*catTrack // tracks not in the snapshot, as loaded
}

// libQuery is one request's library view: the snapshot, its library
// index and catalog rows for library tracks the snapshot doesn't have.
type libQuery struct {
	*q
	*libIndex
	db       *sql.DB
	cat      map[string]*catTrack
	groups   map[string][]int32
	starts   []*libview.Starts // by snapshot track, for tracks in groups
	scanned  bool
	ctxPlays map[int32]*ctxTally
}

type ctxTally struct {
	plays, ms, last int64
	perTrack        map[int32]int64
}

func (b *Backend) libQuery(ctx context.Context) (*libQuery, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	idx, err := b.libIndex(ctx, x)
	if err != nil {
		return nil, err
	}
	return &libQuery{q: x, libIndex: idx, db: b.db.Read, cat: map[string]*catTrack{}, groups: map[string][]int32{}}, nil
}

// libIndex returns the index for x's snapshot, building it once.
func (b *Backend) libIndex(ctx context.Context, x *q) (*libIndex, error) {
	b.libMu.Lock()
	defer b.libMu.Unlock()
	if b.libIdx != nil && b.libIdx.snap == x.Snapshot {
		return b.libIdx, nil
	}
	idx := &libIndex{snap: x.Snapshot, isrcOf: map[string]string{}, byISRC: map[string][]string{},
		relinks: map[string][]string{}, played: map[string]bool{}, cat: map[string]*catTrack{}}
	rows, err := b.db.Read.QueryContext(ctx, `SELECT id, COALESCE(isrc, ''), COALESCE(linked_from, '') FROM tracks
		WHERE isrc IS NOT NULL OR linked_from IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, isrc, linked string
		if err := rows.Scan(&id, &isrc, &linked); err != nil {
			rows.Close()
			return nil, err
		}
		if isrc != "" {
			idx.isrcOf[id] = isrc
			idx.byISRC[isrc] = append(idx.byISRC[isrc], id)
		}
		if linked != "" {
			idx.relinks[id] = append(idx.relinks[id], linked)
			idx.relinks[linked] = append(idx.relinks[linked], id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for t := range x.e.Tracks {
		if x.TrackPlays[t] > 0 {
			idx.played[nameKey(x.e, int32(t))] = true
		}
	}
	b.libIdx = idx
	return idx, nil
}

// load reads the catalog rows of keys the snapshot lacks (names, refs),
// once per snapshot.
func (l *libQuery) load(ctx context.Context, keys []string) error {
	var missing []string
	l.mu.Lock()
	for _, k := range keys {
		if _, ok := l.e.TrackByID[k]; ok {
			continue
		}
		if _, ok := l.cat[k]; ok {
			continue
		}
		if c, ok := l.libIndex.cat[k]; ok {
			l.cat[k] = c
			continue
		}
		l.cat[k] = nil
		missing = append(missing, k)
	}
	l.mu.Unlock()
	found := map[string]*catTrack{}
	for len(missing) > 0 {
		n := min(len(missing), 500)
		chunk := missing[:n]
		missing = missing[n:]
		args := make([]any, len(chunk))
		for i, k := range chunk {
			args[i] = k
		}
		rows, err := l.db.QueryContext(ctx, `SELECT t.id, t.name, t.duration_ms, al.id, al.name, al.image_url
			FROM tracks t LEFT JOIN albums al ON al.id = t.album_id WHERE t.id IN (`+placeholders(n)+`)`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var c catTrack
			var id string
			var dur sql.NullInt64
			var alID, alName, img sql.NullString
			if err := rows.Scan(&id, &c.name, &dur, &alID, &alName, &img); err != nil {
				rows.Close()
				return err
			}
			c.ref = api.TrackRef{ID: id, Name: c.name, Artists: []api.ArtistLite{}}
			if dur.Valid && dur.Int64 > 0 {
				c.ref.DurationMs = ptr(dur.Int64)
			}
			if alID.Valid {
				c.ref.Album = &api.AlbumLite{ID: alID.String, Name: alName.String}
				c.ref.ImageURL = strPtr(img.String)
			}
			found[id] = &c
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = l.db.QueryContext(ctx, `SELECT ta.track_id, ta.artist_id, COALESCE(a.name, ta.artist_id)
			FROM track_artists ta LEFT JOIN artists a ON a.id = ta.artist_id
			WHERE ta.track_id IN (`+placeholders(n)+`) ORDER BY ta.track_id, ta.position`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var tid, aid, name string
			if err := rows.Scan(&tid, &aid, &name); err != nil {
				rows.Close()
				return err
			}
			if c := found[tid]; c != nil {
				if len(c.artistIDs) == 0 {
					c.mainArtist = name
				}
				c.artistIDs = append(c.artistIDs, aid)
				c.ref.Artists = append(c.ref.Artists, api.ArtistLite{ID: aid, Name: name})
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	if len(found) > 0 {
		// Only rows found are kept: a track seeded since the snapshot
		// is read again until the next one.
		l.mu.Lock()
		for k, c := range found {
			l.cat[k] = c
			l.libIndex.cat[k] = c
		}
		l.mu.Unlock()
	}
	return nil
}

func placeholders(n int) string { return "?" + strings.Repeat(", ?", n-1) }

// group returns the snapshot tracks a saved key matches.
func (l *libQuery) group(key string) []int32 {
	if g, ok := l.groups[key]; ok {
		return g
	}
	var g []int32
	seen := map[int32]bool{}
	addKey := func(k string) {
		if t, ok := l.e.TrackByID[k]; ok && !seen[t] {
			seen[t] = true
			g = append(g, t)
		}
	}
	addKey(key)
	for _, k := range l.relinks[key] {
		addKey(k)
	}
	if isrc := l.isrcOf[key]; isrc != "" {
		for _, k := range l.byISRC[isrc] {
			addKey(k)
		}
	}
	l.groups[key] = g
	return g
}

// excluded reports whether a saved key is hidden by exclusions.
func (l *libQuery) excluded(key string) bool {
	if t, ok := l.e.TrackByID[key]; ok {
		if l.TrackExcluded(t) {
			return true
		}
		for _, a := range l.e.Tracks[t].Artists {
			if l.ArtistExcluded(a) {
				return true
			}
		}
		return false
	}
	if c := l.cat[key]; c != nil {
		for _, id := range c.artistIDs {
			if a, ok := l.e.ArtistByID[id]; ok && l.ArtistExcluded(a) {
				return true
			}
		}
	}
	return false
}

// scan tallies the visible plays of every track in groups and, for
// contexts, the plays started from them.
func (l *libQuery) scan(contexts map[int32]bool) {
	if l.scanned {
		return
	}
	l.scanned = true
	l.starts = make([]*libview.Starts, len(l.e.Tracks))
	for _, g := range l.groups {
		for _, t := range g {
			if l.starts[t] == nil {
				l.starts[t] = &libview.Starts{}
			}
		}
	}
	l.ctxPlays = map[int32]*ctxTally{}
	for c := range contexts {
		l.ctxPlays[c] = &ctxTally{perTrack: map[int32]int64{}}
	}
	byCtx := len(l.ctxPlays) > 0
	for _, i := range l.Vis {
		t := l.Track[i]
		if s := l.starts[t]; s != nil {
			if l.Counted[i] {
				s.Add(l.Start[i], int64(l.Ms[i]))
			} else {
				s.AddMs(int64(l.Ms[i]))
			}
		}
		if c := l.Ctx[i]; byCtx && c >= 0 {
			if ct := l.ctxPlays[c]; ct != nil {
				ct.ms += int64(l.Ms[i])
				if l.Counted[i] {
					ct.plays++
					ct.last = max(ct.last, l.Start[i])
					ct.perTrack[t]++
				}
			}
		}
	}
}

func nameKey(e *derive.Entities, t int32) string {
	r := &e.Tracks[t]
	main := ""
	if len(r.Artists) > 0 {
		main = e.Artists[r.Artists[0]].Name
	}
	return libview.MatchKey(r.Name, main)
}

// track builds a saved track's view (after load, group and scan).
func (l *libQuery) track(key string, addedAt int64) libview.Track {
	out := libview.Track{AddedAt: addedAt}
	var mk string
	if t, ok := l.e.TrackByID[key]; ok {
		out.Ref = l.trackRef(t)
		mk = nameKey(l.e, t)
	} else if c := l.cat[key]; c != nil {
		out.Ref = c.ref
		mk = libview.MatchKey(c.name, c.mainArtist)
	} else {
		out.Ref = api.TrackRef{ID: key, Name: key, Artists: []api.ArtistLite{}}
	}
	var s libview.Starts
	for _, t := range l.groups[key] {
		if ts := l.starts[t]; ts != nil {
			s.Merge(ts)
		}
	}
	s.Fill(&out)
	out.Status = libview.Status(out.Plays, mk != "" && l.played[mk])
	return out
}

// spanCache is the present spans of each list read since the newest
// library sync row last changed (a sync starting or finishing), so a
// sync's writes show once it finishes.
type spanCache struct {
	ver   string
	lists map[string][]libTrack
}

// readSpans returns a list's present spans in order; callers must not
// modify the slice.
func (b *Backend) readSpans(ctx context.Context, list string) ([]libTrack, error) {
	var id int64
	var fin sql.NullInt64
	err := b.db.Read.QueryRowContext(ctx, `SELECT id, finished_at FROM library_syncs ORDER BY id DESC LIMIT 1`).Scan(&id, &fin)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	ver := fmt.Sprint(id, fin.Valid, fin.Int64)
	b.libMu.Lock()
	if c := b.spans; c != nil && c.ver == ver {
		if v, ok := c.lists[list]; ok {
			b.libMu.Unlock()
			return v, nil
		}
	}
	b.libMu.Unlock()
	rows, err := b.db.Read.QueryContext(ctx, `SELECT item_uri, COALESCE(added_at, 0), COALESCE(added_by, ''), COALESCE(position, 0)
		FROM library_spans WHERE list = ? AND gone_sync IS NULL ORDER BY position, id`, list)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []libTrack
	for rows.Next() {
		var t libTrack
		var uri string
		if err := rows.Scan(&uri, &t.addedAt, &t.addedBy, &t.pos); err != nil {
			return nil, err
		}
		t.key = trackKey(uri)
		if strings.HasPrefix(uri, "spotify:album:") {
			t.key = strings.TrimPrefix(uri, "spotify:album:")
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	b.libMu.Lock()
	if b.spans == nil || b.spans.ver != ver {
		b.spans = &spanCache{ver: ver, lists: map[string][]libTrack{}}
	}
	b.spans.lists[list] = out
	b.libMu.Unlock()
	return out, nil
}

// savedTracks is every saved track not hidden by exclusions.
func (b *Backend) savedTracks(ctx context.Context, l *libQuery) ([]libview.Track, error) {
	spans, err := b.readSpans(ctx, library.ListTracks)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(spans))
	for i, s := range spans {
		keys[i] = s.key
	}
	if err := l.load(ctx, keys); err != nil {
		return nil, err
	}
	var kept []libTrack
	for _, s := range spans {
		if !l.excluded(s.key) {
			l.group(s.key)
			kept = append(kept, s)
		}
	}
	l.scan(nil)
	out := make([]libview.Track, 0, len(kept))
	for _, s := range kept {
		out = append(out, l.track(s.key, s.addedAt))
	}
	return out, nil
}

func (b *Backend) libraryStatus(ctx context.Context) (library.Status, error) {
	if b.library != nil {
		return b.library.Status(ctx)
	}
	return library.ReadStatus(ctx, b.db.Read, nil)
}

func (b *Backend) LibrarySummary(ctx context.Context) (*api.LibrarySummary, error) {
	l, err := b.libQuery(ctx)
	if err != nil {
		return nil, err
	}
	tracks, err := b.savedTracks(ctx, l)
	if err != nil {
		return nil, err
	}
	st, err := library.ReadStatus(ctx, b.db.Read, nil)
	if err != nil {
		return nil, err
	}
	return libview.Summary(tracks, st.SavedAlbums, st.Playlists, st.LastSyncAt), nil
}

func (b *Backend) LibraryTracks(ctx context.Context, p api.LibraryTracksParams) (*api.LibraryTracks, error) {
	l, err := b.libQuery(ctx)
	if err != nil {
		return nil, err
	}
	tracks, err := b.savedTracks(ctx, l)
	if err != nil {
		return nil, err
	}
	return libview.Tracks(tracks, p), nil
}

func (b *Backend) LibraryAlbums(ctx context.Context, p api.LibraryAlbumsParams) (*api.LibraryAlbums, error) {
	l, err := b.libQuery(ctx)
	if err != nil {
		return nil, err
	}
	spans, err := b.readSpans(ctx, library.ListAlbums)
	if err != nil {
		return nil, err
	}
	// Per album: tracks with counted plays, plays and time.
	type agg struct {
		played    map[int32]bool
		plays, ms int64
	}
	byAlbum := map[int32]*agg{}
	for _, s := range spans {
		if a, ok := l.e.AlbumByID[s.key]; ok {
			byAlbum[a] = &agg{played: map[int32]bool{}}
		}
	}
	for _, i := range l.Vis {
		t := l.Track[i]
		al := l.e.Tracks[t].Album
		if al < 0 {
			continue
		}
		g := byAlbum[al]
		if g == nil {
			continue
		}
		g.ms += int64(l.Ms[i])
		if l.Counted[i] {
			g.plays++
			g.played[t] = true
		}
	}
	out := make([]libview.Album, 0, len(spans))
	for _, s := range spans {
		v := libview.Album{AddedAt: s.addedAt}
		if a, ok := l.e.AlbumByID[s.key]; ok {
			v.Ref = l.albumRef(a)
			v.TracksTotal = l.e.Albums[a].TotalTracks
			g := byAlbum[a]
			v.TracksPlayed, v.Plays, v.Ms = len(g.played), g.plays, g.ms
		} else {
			ref, total, err := b.catalogAlbum(ctx, s.key)
			if err != nil {
				return nil, err
			}
			v.Ref, v.TracksTotal = ref, total
		}
		out = append(out, v)
	}
	return libview.Albums(out, p), nil
}

// catalogAlbum is an album the snapshot doesn't have (saved, never
// played), from the catalog.
func (b *Backend) catalogAlbum(ctx context.Context, id string) (api.AlbumRef, int, error) {
	ref := api.AlbumRef{ID: id, Name: id, Artists: []api.ArtistLite{}}
	var name string
	var img, date sql.NullString
	var total sql.NullInt64
	err := b.db.Read.QueryRowContext(ctx, `SELECT name, image_url, release_date, total_tracks FROM albums WHERE id = ?`, id).
		Scan(&name, &img, &date, &total)
	if err == sql.ErrNoRows {
		return ref, 0, nil
	}
	if err != nil {
		return ref, 0, err
	}
	ref.Name, ref.ImageURL = name, strPtr(img.String)
	if y := releaseYearOf(date.String); y > 0 {
		ref.ReleaseYear = ptr(y)
	}
	rows, err := b.db.Read.QueryContext(ctx, `SELECT aa.artist_id, COALESCE(a.name, aa.artist_id) FROM album_artists aa
		LEFT JOIN artists a ON a.id = aa.artist_id WHERE aa.album_id = ? ORDER BY aa.position`, id)
	if err != nil {
		return ref, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var a api.ArtistLite
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			return ref, 0, err
		}
		ref.Artists = append(ref.Artists, a)
	}
	return ref, int(total.Int64), rows.Err()
}

func releaseYearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y := 0
	for _, c := range date[:4] {
		if c < '0' || c > '9' {
			return 0
		}
		y = y*10 + int(c-'0')
	}
	return y
}

type libPlaylist struct {
	id, name, relation string
	owner, image       sql.NullString
	total              sql.NullInt64
	itemsAvailable     bool
}

func (b *Backend) readPlaylists(ctx context.Context, id string) ([]libPlaylist, error) {
	q := `SELECT id, name, relation, owner_name, image_url, items_total, items_available FROM library_playlists
		WHERE gone_sync IS NULL`
	var args []any
	if id != "" {
		q += ` AND id = ?`
		args = append(args, id)
	}
	rows, err := b.db.Read.QueryContext(ctx, q+` ORDER BY position, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []libPlaylist
	for rows.Next() {
		var p libPlaylist
		if err := rows.Scan(&p.id, &p.name, &p.relation, &p.owner, &p.image, &p.total, &p.itemsAvailable); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// playlists builds playlist views; items maps each playlist with synced
// items to its track items (when withItems).
func (b *Backend) playlists(ctx context.Context, l *libQuery, ps []libPlaylist) ([]libview.Playlist, map[string][]libTrack, error) {
	items := map[string][]libTrack{}
	var keys []string
	for _, p := range ps {
		if !p.itemsAvailable {
			continue
		}
		its, err := b.readSpans(ctx, p.id)
		if err != nil {
			return nil, nil, err
		}
		items[p.id] = its
		for _, it := range its {
			keys = append(keys, it.key)
		}
	}
	if err := l.load(ctx, keys); err != nil {
		return nil, nil, err
	}
	for _, k := range keys {
		l.group(k)
	}
	ctxs := map[int32]bool{}
	for _, p := range ps {
		if c, ok := l.e.ContextByID[libview.PlaylistURI(p.id)]; ok {
			ctxs[c] = true
		}
	}
	l.scan(ctxs)
	out := make([]libview.Playlist, 0, len(ps))
	for _, p := range ps {
		v := libview.Playlist{ItemsTotal: -1, ItemsAvailable: p.itemsAvailable,
			Ref: api.LibraryPlaylistRef{ID: p.id, URI: libview.PlaylistURI(p.id), Name: p.name, OwnerName: strPtr(p.owner.String),
				Relation: p.relation, ImageURL: strPtr(p.image.String), SpotifyOwned: libview.SpotifyOwned(p.id)}}
		if p.total.Valid {
			v.ItemsTotal = int(p.total.Int64)
		}
		if c, ok := l.e.ContextByID[v.Ref.URI]; ok {
			if ct := l.ctxPlays[c]; ct != nil {
				v.PlaysHere, v.MsHere, v.LastHere = ct.plays, ct.ms, ct.last
			}
		}
		if p.itemsAvailable {
			seen := map[string]bool{}
			for _, it := range items[p.id] {
				if seen[it.key] || l.excluded(it.key) {
					continue
				}
				seen[it.key] = true
				v.Tracks++
				if l.track(it.key, it.addedAt).Plays > 0 {
					v.TracksPlayed++
				}
			}
		}
		out = append(out, v)
	}
	return out, items, nil
}

func (b *Backend) LibraryPlaylists(ctx context.Context, p api.LibraryPlaylistsParams) (*api.LibraryPlaylists, error) {
	l, err := b.libQuery(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := b.readPlaylists(ctx, "")
	if err != nil {
		return nil, err
	}
	views, _, err := b.playlists(ctx, l, ps)
	if err != nil {
		return nil, err
	}
	return libview.Playlists(views, p), nil
}

func (b *Backend) LibraryPlaylist(ctx context.Context, id string, pg api.Page) (*api.LibraryPlaylistDetail, error) {
	l, err := b.libQuery(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := b.readPlaylists(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, fmt.Errorf("%w: playlist %q", api.ErrNotFound, id)
	}
	views, items, err := b.playlists(ctx, l, ps)
	if err != nil {
		return nil, err
	}
	var account string
	_ = b.db.Read.QueryRowContext(ctx, `SELECT account_id FROM spotify_connection WHERE id = 1`).Scan(&account)
	var ct *ctxTally
	if c, ok := l.e.ContextByID[libview.PlaylistURI(id)]; ok {
		ct = l.ctxPlays[c]
	}
	var out []libview.Item
	for _, it := range items[id] {
		if l.excluded(it.key) {
			continue
		}
		v := libview.Item{Position: it.pos, Track: l.track(it.key, it.addedAt), AddedBy: it.addedBy,
			AddedByOwner: it.addedBy != "" && it.addedBy == account}
		if ct != nil {
			for _, t := range l.groups[it.key] {
				v.PlaysHere += ct.perTrack[t]
			}
		}
		out = append(out, v)
	}
	return libview.PlaylistDetail(&views[0], out, pg), nil
}

func (b *Backend) LibraryTimeline(ctx context.Context, spec query.Spec, bucket query.Bucket) (*api.LibraryTimeline, error) {
	x, err := b.q(ctx)
	if err != nil {
		return nil, err
	}
	cur, _, err := x.resolve(spec)
	if err != nil {
		return nil, err
	}
	rows, err := b.db.Read.QueryContext(ctx, `SELECT s.list, COALESCE(s.added_at, 0), COALESCE(g.started_at, 0)
		FROM library_spans s LEFT JOIN library_syncs g ON g.id = s.gone_sync`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []libview.Event
	for rows.Next() {
		var list string
		var added, gone int64
		if err := rows.Scan(&list, &added, &gone); err != nil {
			return nil, err
		}
		saved, removed := libview.PlaylistAdd, libview.PlaylistRemoval
		switch list {
		case library.ListTracks:
			saved, removed = libview.TrackSaved, libview.TrackRemoved
		case library.ListAlbums:
			saved, removed = libview.AlbumSaved, libview.AlbumRemoved
		}
		if added > 0 {
			events = append(events, libview.Event{At: added, Kind: saved})
		}
		if gone > 0 {
			events = append(events, libview.Event{At: gone, Kind: removed})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return libview.Timeline(cur, bucket, x.env.WeekStart, events)
}

func (b *Backend) SyncLibrary(ctx context.Context) (*api.LibraryStatus, error) {
	if b.library == nil {
		return nil, api.ErrSyncDisabled
	}
	if !b.library.SyncNow() {
		return nil, api.ErrSyncRunning
	}
	st, err := b.library.Status(ctx)
	if err != nil {
		return nil, err
	}
	w := WireLibraryStatus(st)
	w.State = library.StateSyncing
	return &w, nil
}

// savedAt is when a track was saved, if it is saved now.
func (b *Backend) savedAt(ctx context.Context, id string) (*int64, error) {
	var at sql.NullInt64
	err := b.db.Read.QueryRowContext(ctx, `SELECT MAX(COALESCE(added_at, 0)) FROM library_spans
		WHERE list = 'tracks' AND gone_sync IS NULL AND item_uri = ?`, "spotify:track:"+id).Scan(&at)
	if err != nil || !at.Valid || at.Int64 == 0 {
		return nil, err
	}
	return &at.Int64, nil
}

// WireLibraryStatus converts library sync status.
func WireLibraryStatus(st library.Status) api.LibraryStatus {
	ms := func(v int64) *int64 {
		if v == 0 {
			return nil
		}
		return &v
	}
	list := func(name string) api.LibraryListStatus {
		ls, ok := st.Lists[name]
		if !ok {
			return api.LibraryListStatus{State: library.ListPending}
		}
		return api.LibraryListStatus{State: ls.State, LastError: strPtr(ls.LastError)}
	}
	out := api.LibraryStatus{State: st.State, LastSyncAt: ms(st.LastSyncAt), LastFullAt: ms(st.LastFullAt),
		NextSyncAt: ms(st.NextSyncAt), ResumesAt: ms(st.ResumesAt), SavedTracks: st.SavedTracks, SavedAlbums: st.SavedAlbums,
		Playlists: st.Playlists, PlaylistsWithItems: st.PlaylistsWithItems, LastError: strPtr(st.LastError),
		LastErrorAt: ms(st.LastErrAt),
		Lists:       api.LibraryLists{Tracks: list(library.ListTracks), Albums: list(library.ListAlbums), Playlists: list(library.ListPlaylists)}}
	if st.State == library.StateSyncing {
		p := st.Progress
		out.Progress = &p
	}
	return out
}
