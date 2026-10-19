package derive

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"spotussi/internal/db"
)

// Reading the raw layer and catalog, and writing the derived tables.

// ---------- raw ----------

// rawWindow restricts raw reads to rows at or after a time (0: all).
type rawWindow struct{ fromMs int64 }

func nullTri(v sql.NullBool) Tri {
	if !v.Valid {
		return Unknown
	}
	return TriOf(v.Bool)
}

// relinks maps original track IDs to the IDs Spotify relinked them to
// (tracks.linked_from, filled by hydration): an export row can name the
// original while the live sources name the relinked track.
func relinks(ctx context.Context, q *sql.DB) (map[string]string, error) {
	rs, err := q.QueryContext(ctx, `SELECT linked_from, id FROM tracks WHERE linked_from IS NOT NULL AND linked_from != id`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	m := map[string]string{}
	for rs.Next() {
		var from, to string
		if err := rs.Scan(&from, &to); err != nil {
			return nil, err
		}
		m[from] = to
	}
	return m, rs.Err()
}

// exportTrackID is the track key of an export row's URI, after relinking.
func exportTrackID(uri string, relinked map[string]string) string {
	id := TrackIDFromURI(uri)
	if to, ok := relinked[id]; ok {
		return to
	}
	return id
}

// readExport reads music export rows (video files excluded, D11) and the
// newest names per track.
func readExport(ctx context.Context, q *sql.DB, w rawWindow, names map[string]ExportNames) ([]ExportRow, error) {
	relinked, err := relinks(ctx, q)
	if err != nil {
		return nil, err
	}
	rs, err := q.QueryContext(ctx, `SELECT id, ts_ms, ms_played, uri, track_name, artist_name, album_name,
			reason_start, reason_end, platform, conn_country, shuffle, skipped, offline
		FROM raw_export_rows WHERE content_type = 'track' AND video = 0 AND uri IS NOT NULL AND ts_ms >= ?
		ORDER BY id`, w.fromMs)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []ExportRow
	var uri, track, artist, album, rs1, re, platform, country sql.NullString
	var shuffle, skipped, offline sql.NullBool
	for rs.Next() {
		var r ExportRow
		if err := rs.Scan(&r.ID, &r.TsMs, &r.MsPlayed, &uri, &track, &artist, &album, &rs1, &re, &platform, &country,
			&shuffle, &skipped, &offline); err != nil {
			return nil, err
		}
		r.TrackID = exportTrackID(uri.String, relinked)
		r.ReasonStart, r.ReasonEnd, r.Platform, r.Country = rs1.String, re.String, platform.String, country.String
		r.Shuffle, r.Skipped, r.Offline = nullTri(shuffle), nullTri(skipped), nullTri(offline)
		out = append(out, r)
		if names != nil {
			names[r.TrackID] = ExportNames{Track: track.String, Artist: artist.String, Album: album.String}
		}
	}
	return out, rs.Err()
}

// exportSpan is [first start, last end] of the music export rows.
func exportSpan(ctx context.Context, q *sql.DB) ([2]int64, error) {
	var lo, hi sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT MIN(ts_ms - ms_played), MAX(ts_ms) FROM raw_export_rows
		WHERE content_type = 'track' AND video = 0`).Scan(&lo, &hi)
	return [2]int64{lo.Int64, hi.Int64}, err
}

func readPoller(ctx context.Context, q *sql.DB, w rawWindow) ([]PollerRow, error) {
	rs, err := q.QueryContext(ctx, `SELECT id, track_id, duration_ms, started_ms, ended_ms, ms_played, skipped, shuffle,
			device_name, device_type, context_uri
		FROM raw_poller_plays WHERE started_ms >= ? ORDER BY id`, w.fromMs)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []PollerRow
	for rs.Next() {
		var r PollerRow
		var shuffle sql.NullBool
		var dn, dt, cu sql.NullString
		if err := rs.Scan(&r.ID, &r.TrackID, &r.DurationMs, &r.StartedMs, &r.EndedMs, &r.MsPlayed, &r.Skipped, &shuffle,
			&dn, &dt, &cu); err != nil {
			return nil, err
		}
		r.Shuffle, r.DeviceName, r.DeviceType, r.ContextURI = nullTri(shuffle), dn.String, dt.String, cu.String
		out = append(out, r)
	}
	return out, rs.Err()
}

func readRecent(ctx context.Context, q *sql.DB, w rawWindow) ([]RecentRow, error) {
	rs, err := q.QueryContext(ctx, `SELECT played_at_ms, track_id, duration_ms, context_uri FROM raw_recent_plays
		WHERE played_at_ms >= ?`, w.fromMs)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []RecentRow
	for rs.Next() {
		var r RecentRow
		var dur sql.NullInt64
		var cu sql.NullString
		if err := rs.Scan(&r.PlayedAtMs, &r.TrackID, &dur, &cu); err != nil {
			return nil, err
		}
		r.DurationMs, r.ContextURI = dur.Int64, cu.String
		out = append(out, r)
	}
	return out, rs.Err()
}

// ---------- catalog and live track objects ----------

// liveJSON is the part of a Spotify track object resolution reads.
type liveJSON struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
	Explicit   *bool  `json:"explicit"`
	Artists    []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		AlbumType   string `json:"album_type"`
		ReleaseDate string `json:"release_date"`
		TotalTracks int    `json:"total_tracks"`
		Images      []struct {
			URL string `json:"url"`
		} `json:"images"`
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"album"`
}

func (j *liveJSON) track() LiveTrack {
	t := LiveTrack{Name: j.Name, DurationMs: j.DurationMs, Album: LiveAlbum{ID: j.Album.ID, Name: j.Album.Name,
		Type: j.Album.AlbumType, ReleaseDate: j.Album.ReleaseDate, TotalTracks: j.Album.TotalTracks}}
	if j.Explicit != nil {
		t.Explicit = TriOf(*j.Explicit)
	}
	for _, a := range j.Artists {
		t.Artists = append(t.Artists, LiveArtist{ID: a.ID, Name: a.Name})
	}
	for _, a := range j.Album.Artists {
		t.Album.Artists = append(t.Album.Artists, LiveArtist{ID: a.ID, Name: a.Name})
	}
	if len(j.Album.Images) > 0 { // Spotify lists the largest first
		t.Album.ImageURL = j.Album.Images[0].URL
	}
	return t
}

// idFilter limits catalog reads to some IDs (nil: all).
type idFilter []string

// in runs fn with an "IN (...)" clause and args per chunk of ids, or once
// with "1 = 1" when the filter is nil.
func (f idFilter) each(col string, fn func(cond string, args []any) error) error {
	if f == nil {
		return fn("1 = 1", nil)
	}
	const chunk = 500
	for i := 0; i < len(f); i += chunk {
		part := f[i:min(len(f), i+chunk)]
		args := make([]any, len(part))
		for k, id := range part {
			args[k] = id
		}
		if err := fn(col+" IN (?"+strings.Repeat(", ?", len(part)-1)+")", args); err != nil {
			return err
		}
	}
	return nil
}

func queryEach(ctx context.Context, q *sql.DB, f idFilter, col, sqlTmpl string, scan func(*sql.Rows) error) error {
	return f.each(col, func(cond string, args []any) error {
		rs, err := q.QueryContext(ctx, strings.ReplaceAll(sqlTmpl, "{cond}", cond), args...)
		if err != nil {
			return err
		}
		defer rs.Close()
		for rs.Next() {
			if err := scan(rs); err != nil {
				return err
			}
		}
		return rs.Err()
	})
}

// loadResolverInput reads what resolution needs for tracks (nil: all
// tracks). Export names come from readExport for full loads, or are
// read here when names is nil.
func loadResolverInput(ctx context.Context, q *sql.DB, tracks idFilter, names map[string]ExportNames) (*ResolverInput, error) {
	in := newResolverInput()
	err := queryEach(ctx, q, tracks, "id", `SELECT id, name, duration_ms, explicit, album_id, hydrated_at IS NOT NULL,
		COALESCE(source = 'fallback', 0) FROM tracks WHERE {cond}`, func(rs *sql.Rows) error {
		var id, name string
		var dur sql.NullInt64
		var explicit sql.NullBool
		var album sql.NullString
		var t CatTrack
		if err := rs.Scan(&id, &name, &dur, &explicit, &album, &t.Hydrated, &t.Fallback); err != nil {
			return err
		}
		t.Name, t.DurationMs, t.Explicit, t.AlbumID = name, dur.Int64, nullTri(explicit), album.String
		in.Tracks[id] = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = queryEach(ctx, q, tracks, "track_id", `SELECT track_id, artist_id FROM track_artists WHERE {cond}
		ORDER BY track_id, position`, func(rs *sql.Rows) error {
		var t, a string
		if err := rs.Scan(&t, &a); err != nil {
			return err
		}
		in.TrackArtists[t] = append(in.TrackArtists[t], a)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Live track objects: the newest poller row per track, else the newest
	// recently-played item.
	for _, src := range []struct{ sql, path string }{
		{`SELECT track_id, json, MAX(id) FROM raw_poller_plays WHERE {cond} GROUP BY track_id`, "item"},
		{`SELECT track_id, json, MAX(played_at_ms) FROM raw_recent_plays WHERE {cond} GROUP BY track_id`, "track"},
	} {
		err = queryEach(ctx, q, tracks, "track_id", src.sql, func(rs *sql.Rows) error {
			var id, raw string
			var ignored int64
			if err := rs.Scan(&id, &raw, &ignored); err != nil {
				return err
			}
			if _, ok := in.Live[id]; ok {
				return nil
			}
			var wrap map[string]json.RawMessage
			var j liveJSON
			if json.Unmarshal([]byte(raw), &wrap) == nil && json.Unmarshal(wrap[src.path], &j) == nil && (j.Name != "" || len(j.Artists) > 0) {
				in.Live[id] = j.track()
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if names != nil {
		in.Export = names
	} else {
		relinked, err := relinks(ctx, q)
		if err != nil {
			return nil, err
		}
		uris := idFilter(nil)
		if tracks != nil {
			uris = make(idFilter, len(tracks))
			for i, t := range tracks {
				uris[i] = "spotify:track:" + t
				if strings.Contains(t, ":") {
					uris[i] = t
				}
			}
			want := map[string]bool{}
			for _, t := range tracks {
				want[t] = true
			}
			for from, to := range relinked {
				if want[to] {
					uris = append(uris, "spotify:track:"+from)
				}
			}
		}
		err = queryEach(ctx, q, uris, "uri", `SELECT uri, track_name, artist_name, album_name, MAX(id) FROM raw_export_rows
			WHERE content_type = 'track' AND video = 0 AND {cond} GROUP BY uri`, func(rs *sql.Rows) error {
			var uri string
			var t, a, al sql.NullString
			var ignored int64
			if err := rs.Scan(&uri, &t, &a, &al, &ignored); err != nil {
				return err
			}
			in.Export[exportTrackID(uri, relinked)] = ExportNames{Track: t.String, Artist: a.String, Album: al.String}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Albums and artists: every one the tracks and live objects refer to.
	albums, artists := idFilter{}, idFilter{}
	seenAl, seenAr := map[string]bool{}, map[string]bool{}
	addAl := func(id string) {
		if id != "" && !seenAl[id] {
			seenAl[id] = true
			albums = append(albums, id)
		}
	}
	addAr := func(id string) {
		if id != "" && !seenAr[id] {
			seenAr[id] = true
			artists = append(artists, id)
		}
	}
	for _, t := range in.Tracks {
		addAl(t.AlbumID)
	}
	for _, as := range in.TrackArtists {
		for _, a := range as {
			addAr(a)
		}
	}
	for _, t := range in.Live {
		addAl(t.Album.ID)
		for _, a := range t.Artists {
			addAr(a.ID)
		}
		for _, a := range t.Album.Artists {
			addAr(a.ID)
		}
	}
	if tracks == nil {
		albums, artists = nil, nil // read whole tables
	}
	err = queryEach(ctx, q, albums, "id", `SELECT id, name, album_type, release_date, total_tracks, image_url,
			hydrated_at IS NOT NULL FROM albums WHERE {cond}`, func(rs *sql.Rows) error {
		var id string
		var a CatAlbum
		var typ, rel, img sql.NullString
		var total sql.NullInt64
		if err := rs.Scan(&id, &a.Name, &typ, &rel, &total, &img, &a.Hydrated); err != nil {
			return err
		}
		a.Type, a.ReleaseDate, a.ImageURL, a.TotalTracks = typ.String, rel.String, img.String, int(total.Int64)
		in.Albums[id] = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = queryEach(ctx, q, albums, "album_id", `SELECT album_id, artist_id FROM album_artists WHERE {cond}
		ORDER BY album_id, position`, func(rs *sql.Rows) error {
		var al, a string
		if err := rs.Scan(&al, &a); err != nil {
			return err
		}
		in.AlbumArtists[al] = append(in.AlbumArtists[al], a)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if albums != nil {
		for _, as := range in.AlbumArtists {
			for _, a := range as {
				addAr(a)
			}
		}
	}
	// Provisional artists and albums of the export names, for their
	// fallback metadata (D27).
	var provArtists, provAlbums idFilter
	if tracks != nil {
		provArtists, provAlbums = idFilter{}, idFilter{}
		for _, n := range in.Export {
			if n.Artist != "" {
				id := ProvArtistID(n.Artist)
				addAr(id)
				provArtists = append(provArtists, id)
			}
			if n.Album != "" {
				provAlbums = append(provAlbums, ProvAlbumID(n.Artist, n.Album))
			}
		}
	}
	err = queryEach(ctx, q, provArtists, "id", `SELECT id, image_url FROM fallback_artists WHERE {cond}`,
		func(rs *sql.Rows) error {
			var id string
			var img sql.NullString
			if err := rs.Scan(&id, &img); err != nil {
				return err
			}
			in.FallbackArtists[id] = CatArtist{ImageURL: img.String}
			return nil
		})
	if err != nil {
		return nil, err
	}
	err = queryEach(ctx, q, provAlbums, "id", `SELECT id, album_type, release_date, total_tracks, image_url
		FROM fallback_albums WHERE {cond}`, func(rs *sql.Rows) error {
		var id string
		var typ, rel, img sql.NullString
		var total sql.NullInt64
		if err := rs.Scan(&id, &typ, &rel, &total, &img); err != nil {
			return err
		}
		in.FallbackAlbums[id] = CatAlbum{Type: typ.String, ReleaseDate: rel.String, ImageURL: img.String, TotalTracks: int(total.Int64)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = queryEach(ctx, q, artists, "id", `SELECT id, name, image_url, hydrated_at IS NOT NULL FROM artists WHERE {cond}`,
		func(rs *sql.Rows) error {
			var id string
			var a CatArtist
			var img sql.NullString
			if err := rs.Scan(&id, &a.Name, &img, &a.Hydrated); err != nil {
				return err
			}
			a.ImageURL = img.String
			in.Artists[id] = a
			return nil
		})
	if err != nil {
		return nil, err
	}
	err = queryEach(ctx, q, artists, "artist_id", `SELECT artist_id, genre FROM artist_genres WHERE {cond}
		ORDER BY artist_id, genre`, func(rs *sql.Rows) error {
		var a, g string
		if err := rs.Scan(&a, &g); err != nil {
			return err
		}
		in.ArtistGenres[a] = append(in.ArtistGenres[a], g)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := loadContexts(ctx, q, in); err != nil {
		return nil, err
	}
	return in, nil
}

// loadContexts reads every catalog context (few: one per playlist).
func loadContexts(ctx context.Context, q *sql.DB, in *ResolverInput) error {
	rs, err := q.QueryContext(ctx, `SELECT uri, type, name, name_source, image_url FROM contexts`)
	if err != nil {
		return err
	}
	defer rs.Close()
	for rs.Next() {
		var uri string
		var c CatContext
		var name, img sql.NullString
		if err := rs.Scan(&uri, &c.Type, &name, &c.NameSource, &img); err != nil {
			return err
		}
		c.Name, c.HasName, c.ImageURL = name.String, name.Valid, img.String
		in.Contexts[uri] = c
	}
	return rs.Err()
}

// ---------- exclusions ----------

func loadExclusions(ctx context.Context, q *sql.DB) ([]Exclusion, error) {
	rs, err := q.QueryContext(ctx, `SELECT kind, id, name FROM exclusions ORDER BY created_at, kind, id`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []Exclusion
	for rs.Next() {
		var x Exclusion
		if err := rs.Scan(&x.Kind, &x.ID, &x.Name); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rs.Err()
}

// ---------- derived tables ----------

// The derived tables as in 0006_derive.sql; full rebuilds create them
// under a _new suffix, fill them, and swap them in.
const (
	playsDDL = `CREATE TABLE plays%s (
    id                  INTEGER PRIMARY KEY,
    track_id            TEXT    NOT NULL,
    album_id            TEXT,
    started_ms          INTEGER NOT NULL,
    ended_ms            INTEGER NOT NULL,
    local_day           TEXT    NOT NULL,
    local_hour          INTEGER NOT NULL,
    ms_played           INTEGER NOT NULL,
    ms_played_estimated INTEGER NOT NULL,
    counts_as_play      INTEGER NOT NULL,
    skipped             INTEGER NOT NULL,
    shuffle             INTEGER,
    platform            TEXT,
    device_name         TEXT,
    device_type         TEXT,
    context_uri         TEXT,
    country             TEXT,
    reason_start        TEXT,
    reason_end          TEXT,
    offline             INTEGER,
    sources             INTEGER NOT NULL,
    contradicted        INTEGER NOT NULL,
    excluded            INTEGER NOT NULL,
    session_id          INTEGER,
    export_row_id       INTEGER,
    poller_play_id      INTEGER,
    recent_played_at_ms INTEGER
) STRICT`
	playsIndexes = `CREATE INDEX plays_started ON plays (started_ms);
CREATE INDEX plays_track ON plays (track_id, started_ms)`
	sessionsDDL = `CREATE TABLE sessions%s (
    id         INTEGER PRIMARY KEY,
    started_ms INTEGER NOT NULL,
    ended_ms   INTEGER NOT NULL,
    rows       INTEGER NOT NULL,
    plays      INTEGER NOT NULL,
    ms_played  INTEGER NOT NULL,
    skips      INTEGER NOT NULL
) STRICT`
	sessionsIndexes = `CREATE INDEX sessions_started ON sessions (started_ms)`

	playsColumns = `id, track_id, album_id, started_ms, ended_ms, local_day, local_hour, ms_played, ms_played_estimated,
		counts_as_play, skipped, shuffle, platform, device_name, device_type, context_uri, country, reason_start,
		reason_end, offline, sources, contradicted, excluded, session_id, export_row_id, poller_play_id, recent_played_at_ms`
)

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func triVal(t Tri) any {
	switch t {
	case True:
		return 1
	case False:
		return 0
	}
	return nil
}

// playRows writes plays (a contiguous run of snap's plays, same order)
// into table with stmt.
func insertPlays(ctx context.Context, stmt *sql.Stmt, plays []Play, snap *Snapshot, first int) error {
	b, v, e := snap.Base, snap.View, snap.Base.Ent
	session := sessionIDs(snap)
	var zone localZone
	for k := range plays {
		p := &plays[k]
		i := int32(first + k)
		t := &e.Tracks[b.Track[i]]
		var album string
		if t.Album >= 0 {
			album = e.Albums[t.Album].ID
		}
		var sid any
		if s := session[i]; s != 0 {
			sid = s
		}
		_, err := stmt.ExecContext(ctx, p.ID, p.TrackID, nullStr(album), p.StartedMs, p.EndedMs,
			zone.localDay(v.Settings.Loc, p.StartedMs), v.Hour[i], p.MsPlayed, p.Estimated, v.Counted[i], p.Skipped,
			triVal(p.Shuffle), nullStr(p.Platform), nullStr(p.DeviceName), nullStr(p.DeviceType), nullStr(p.ContextURI),
			nullStr(p.Country), nullStr(p.ReasonStart), nullStr(p.ReasonEnd), triVal(p.Offline), p.Sources,
			p.Contradicted, sid == nil, sid, nullInt(p.ExportID), nullInt(p.PollerID), nullInt(p.RecentPlayedAt))
		if err != nil {
			return err
		}
	}
	return nil
}

// sessionIDs is the session ID of every play (0: excluded).
func sessionIDs(snap *Snapshot) []int64 {
	out := make([]int64, snap.Len())
	for _, s := range snap.Sessions {
		id := snap.ID[snap.Vis[s.From]]
		for _, i := range snap.Vis[s.From:s.To] {
			out[i] = id
		}
	}
	return out
}

func insertSessions(ctx context.Context, stmt *sql.Stmt, snap *Snapshot, from int) error {
	for _, s := range snap.Sessions[from:] {
		var plays, ms, skips int64
		for _, i := range snap.Vis[s.From:s.To] {
			if snap.Counted[i] {
				plays++
			}
			ms += int64(snap.Ms[i])
			if snap.Has(i, FSkipped) {
				skips++
			}
		}
		if _, err := stmt.ExecContext(ctx, snap.ID[snap.Vis[s.From]], s.Start, s.End, s.To-s.From, plays, ms, skips); err != nil {
			return err
		}
	}
	return nil
}

// writeFull replaces the derived tables with plays (all of snap's, in
// order) in one transaction: new tables are filled, then swapped in, so
// readers see either the old or the new derived layer.
func writeFull(ctx context.Context, d *db.DB, plays []Play, snap *Snapshot, state map[string]string) error {
	tx, err := d.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	for _, s := range []string{`DROP TABLE IF EXISTS plays_new`, `DROP TABLE IF EXISTS sessions_new`,
		fmt.Sprintf(playsDDL, "_new"), fmt.Sprintf(sessionsDDL, "_new")} {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO plays_new (`+playsColumns+`) VALUES (`+placeholders(27)+`)`)
	if err != nil {
		return err
	}
	if err := insertPlays(ctx, stmt, plays, snap, 0); err != nil {
		return err
	}
	stmt.Close()
	stmt, err = tx.PrepareContext(ctx, `INSERT INTO sessions_new (id, started_ms, ended_ms, rows, plays, ms_played, skips)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	if err := insertSessions(ctx, stmt, snap, 0); err != nil {
		return err
	}
	stmt.Close()
	for _, s := range []string{`DROP TABLE plays`, `DROP TABLE sessions`, `ALTER TABLE plays_new RENAME TO plays`,
		`ALTER TABLE sessions_new RENAME TO sessions`, playsIndexes, sessionsIndexes} {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	if err := putState(ctx, tx, state); err != nil {
		return err
	}
	return tx.Commit()
}

// writeTail replaces the plays starting at or after cutMs (and the
// sessions from the one containing the first replaced play) with snap's.
func writeTail(ctx context.Context, d *db.DB, cutMs int64, plays []Play, snap *Snapshot, first int, sessionFrom int, state map[string]string) error {
	tx, err := d.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	if _, err := tx.ExecContext(ctx, `DELETE FROM plays WHERE started_ms >= ?`, cutMs); err != nil {
		return err
	}
	var sessionStart int64 = 1 << 62
	if sessionFrom < len(snap.Sessions) {
		sessionStart = snap.Sessions[sessionFrom].Start
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE started_ms >= ?`, sessionStart); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO plays (`+playsColumns+`) VALUES (`+placeholders(27)+`)`)
	if err != nil {
		return err
	}
	if err := insertPlays(ctx, stmt, plays, snap, first); err != nil {
		return err
	}
	stmt.Close()
	// Plays before the cut that joined a re-derived session.
	if sessionFrom < len(snap.Sessions) {
		s := snap.Sessions[sessionFrom]
		id := snap.ID[snap.Vis[s.From]]
		if _, err := tx.ExecContext(ctx, `UPDATE plays SET session_id = ? WHERE started_ms >= ? AND started_ms < ? AND excluded = 0`,
			id, s.Start, cutMs); err != nil {
			return err
		}
	}
	stmt, err = tx.PrepareContext(ctx, `INSERT INTO sessions (id, started_ms, ended_ms, rows, plays, ms_played, skips)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	if err := insertSessions(ctx, stmt, snap, sessionFrom); err != nil {
		return err
	}
	stmt.Close()
	if err := putState(ctx, tx, state); err != nil {
		return err
	}
	return tx.Commit()
}

func placeholders(n int) string { return "?" + strings.Repeat(", ?", n-1) }

// loadPlays reads the plays table back into merged plays (the restart
// path when nothing changed since the last rebuild).
func loadPlays(ctx context.Context, q *sql.DB) ([]Play, error) {
	rs, err := q.QueryContext(ctx, `SELECT id, track_id, started_ms, ended_ms, ms_played, ms_played_estimated, skipped,
			shuffle, platform, device_name, device_type, context_uri, country, reason_start, reason_end, offline, sources,
			contradicted, export_row_id, poller_play_id, recent_played_at_ms
		FROM plays ORDER BY started_ms, id`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []Play
	for rs.Next() {
		var p Play
		var shuffle, offline sql.NullBool
		var platform, dn, dt, cu, country, rs1, re sql.NullString
		var exp, pol, rec sql.NullInt64
		if err := rs.Scan(&p.ID, &p.TrackID, &p.StartedMs, &p.EndedMs, &p.MsPlayed, &p.Estimated, &p.Skipped, &shuffle,
			&platform, &dn, &dt, &cu, &country, &rs1, &re, &offline, &p.Sources, &p.Contradicted, &exp, &pol, &rec); err != nil {
			return nil, err
		}
		p.Shuffle, p.Offline = nullTri(shuffle), nullTri(offline)
		p.Platform, p.DeviceName, p.DeviceType, p.ContextURI = platform.String, dn.String, dt.String, cu.String
		p.Country, p.ReasonStart, p.ReasonEnd = country.String, rs1.String, re.String
		p.ExportID, p.PollerID, p.RecentPlayedAt = exp.Int64, pol.Int64, rec.Int64
		out = append(out, p)
	}
	return out, rs.Err()
}

// ---------- state ----------

func putState(ctx context.Context, tx *sql.Tx, state map[string]string) error {
	for k, v := range state {
		if _, err := tx.ExecContext(ctx, `INSERT INTO derive_state (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return nil
}

func getState(ctx context.Context, q *sql.DB, key string) (string, error) {
	var v string
	err := q.QueryRowContext(ctx, `SELECT value FROM derive_state WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// derivationVersion changes whenever merge or derived-table semantics
// change, forcing a rebuild on the next start.
const derivationVersion = "1"

// fingerprint summarizes the inputs of the derived tables that only a
// full rebuild folds in: export rows, the played_at calibration, the
// catalog fields resolution reads, settings and exclusions. Live rows
// are not part of it: the stored watermarks say which ones the tables
// cover, and newer ones are merged incrementally after loading. Equal
// fingerprints mean the stored tables are current up to the watermarks.
func fingerprint(ctx context.Context, q *sql.DB, s Settings, excl []Exclusion, playedAtIsStart bool) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "v%s|%s|%s|%d|%d|%t|", derivationVersion, s.Rule.Kind, s.Loc, s.Rule.ThresholdMs, s.SessionGapMinutes, playedAtIsStart)
	for _, x := range excl {
		fmt.Fprintf(h, "%s:%s:%s|", x.Kind, x.ID, x.Name)
	}
	var parts []any
	row := q.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) || ':' || COALESCE(MAX(id), 0) FROM raw_export_rows),
		(SELECT COUNT(*) || ':' || COALESCE(MAX(hydrated_at), 0) || ':' || TOTAL(duration_ms) || ':' || COUNT(album_id)
			|| ':' || COUNT(hydrated_at) FROM tracks),
		(SELECT COUNT(*) || ':' || COALESCE(MAX(hydrated_at), 0) || ':' || COUNT(hydrated_at) FROM albums),
		(SELECT COUNT(*) || ':' || COALESCE(MAX(hydrated_at), 0) || ':' || COUNT(hydrated_at) FROM artists),
		(SELECT COUNT(*) FROM track_artists),
		(SELECT COUNT(*) FROM album_artists),
		(SELECT COUNT(*) FROM artist_genres),
		(SELECT COUNT(*) || ':' || COALESCE(MAX(updated_at), 0) FROM fallback_artists),
		(SELECT COUNT(*) || ':' || COALESCE(MAX(updated_at), 0) FROM fallback_albums),
		(SELECT COUNT(*) || ':' || COALESCE(MAX(updated_at), 0) FROM catalog_provenance)`)
	vals := make([]string, 10)
	for i := range vals {
		parts = append(parts, &vals[i])
	}
	if err := row.Scan(parts...); err != nil {
		return "", err
	}
	h.Write([]byte(strings.Join(vals, "|")))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func nowMs() int64 { return time.Now().UnixMilli() }
