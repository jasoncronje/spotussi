package hydrate

import (
	"context"
	"database/sql"
	"strings"

	"spotussi/internal/spotify"
)

// Catalog writes. Every function runs inside the caller's transaction, so
// an entity, its credits, its queue entry and the follow-up work it
// enqueues commit together.

// Track sources (tracks.source): who wrote the row last.
const (
	SourceAPI    = "api"
	SourcePoller = "poller"
	SourceRecent = "recent"
	// SourceFallback marks a track row only fallback sources wrote
	// (hydrated_at null; per-field sources in catalog_provenance, D27).
	SourceFallback = "fallback"
)

// storeTrack upserts a full track object under key (the ID the raw data
// references: the requested ID for lookups, the item's ID for live
// data), with its album and artist credits, removes its queue entry and
// enqueues its unhydrated artists at priority prio.
func storeTrack(ctx context.Context, tx *sql.Tx, key string, t *spotify.Track, source string, now, prio int64) error {
	if err := clearFallback(ctx, tx, "track", key); err != nil { // Spotify replaces fallback fields (D27)
		return err
	}
	var linked, albumID, isrc any
	if t.LinkedFrom != nil && t.LinkedFrom.ID != "" && t.LinkedFrom.ID != key {
		linked = t.LinkedFrom.ID
	}
	if t.ExternalIDs.ISRC != "" {
		isrc = t.ExternalIDs.ISRC
	}
	if !t.IsLocal && t.Album.ID != "" {
		albumID = t.Album.ID
		if err := storeAlbum(ctx, tx, &t.Album, now, prio); err != nil {
			return err
		}
	}
	name := t.Name
	if name == "" {
		name = key
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO tracks (id, name, duration_ms, explicit, isrc, album_id, disc_number,
			track_number, is_local, linked_from, source, hydrated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name,
			duration_ms = COALESCE(excluded.duration_ms, duration_ms), explicit = excluded.explicit,
			isrc = COALESCE(excluded.isrc, isrc), album_id = COALESCE(excluded.album_id, album_id),
			disc_number = COALESCE(excluded.disc_number, disc_number),
			track_number = COALESCE(excluded.track_number, track_number), is_local = excluded.is_local,
			linked_from = COALESCE(excluded.linked_from, linked_from), source = excluded.source,
			hydrated_at = excluded.hydrated_at`,
		key, name, positive(t.DurationMs), t.Explicit, isrc, albumID, positive(int64(t.DiscNumber)),
		positive(int64(t.TrackNumber)), t.IsLocal, linked, source, now)
	if err != nil {
		return err
	}
	if !t.IsLocal { // local files' artists have no IDs
		if err := storeCredits(ctx, tx, "track_artists", "track_id", key, t.Artists, prio); err != nil {
			return err
		}
	}
	return dequeue(ctx, tx, "track", key)
}

// storeAlbum upserts an album from the (simplified) album object of a
// track or an album lookup. A simplified album carries every column the
// catalog keeps, so it counts as hydrated (D9: no separate album calls).
func storeAlbum(ctx context.Context, tx *sql.Tx, a *spotify.SimpleAlbum, now, prio int64) error {
	large, small := pickImages(a.Images)
	name := a.Name
	if name == "" {
		name = a.ID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO albums (id, name, album_type, release_date, release_date_precision,
			total_tracks, image_url, image_url_small, hydrated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name,
			album_type = COALESCE(excluded.album_type, album_type),
			release_date = COALESCE(excluded.release_date, release_date),
			release_date_precision = COALESCE(excluded.release_date_precision, release_date_precision),
			total_tracks = COALESCE(excluded.total_tracks, total_tracks),
			image_url = COALESCE(excluded.image_url, image_url),
			image_url_small = COALESCE(excluded.image_url_small, image_url_small),
			hydrated_at = excluded.hydrated_at`,
		a.ID, name, nullStr(a.AlbumType), nullStr(a.ReleaseDate), nullStr(a.ReleaseDatePrecision),
		positive(int64(a.TotalTracks)), large, small, now)
	if err != nil {
		return err
	}
	if len(a.Artists) > 0 {
		if err := storeCredits(ctx, tx, "album_artists", "album_id", a.ID, a.Artists, prio); err != nil {
			return err
		}
	}
	return dequeue(ctx, tx, "album", a.ID)
}

// storeCredits replaces the artist credits of a track or album, adds the
// artists as known-by-name and enqueues the unhydrated ones.
func storeCredits(ctx context.Context, tx *sql.Tx, table, col, id string, artists []spotify.SimpleArtist, prio int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+col+` = ?`, id); err != nil {
		return err
	}
	pos := 0
	for _, a := range artists {
		if a.ID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO `+table+` (`+col+`, artist_id, position) VALUES (?, ?, ?)
			ON CONFLICT DO NOTHING`, id, a.ID, pos); err != nil {
			return err
		}
		pos++
		name := a.Name
		if name == "" {
			name = a.ID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO artists (id, name) VALUES (?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name WHERE hydrated_at IS NULL`, a.ID, name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hydration_queue (entity_type, id, priority)
			SELECT 'artist', ?1, ?2 WHERE NOT EXISTS (SELECT 1 FROM artists WHERE id = ?1 AND hydrated_at IS NOT NULL)
			ON CONFLICT (entity_type, id) DO UPDATE SET priority = max(priority, excluded.priority)`, a.ID, prio); err != nil {
			return err
		}
	}
	return nil
}

// storeArtist upserts a full artist with its genres.
func storeArtist(ctx context.Context, tx *sql.Tx, key string, a *spotify.Artist, now int64) error {
	if err := clearFallback(ctx, tx, "artist", key); err != nil { // Spotify replaces fallback fields (D27)
		return err
	}
	large, small := pickImages(a.Images)
	name := a.Name
	if name == "" {
		name = key
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO artists (id, name, image_url, image_url_small, hydrated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, image_url = COALESCE(excluded.image_url, image_url),
			image_url_small = COALESCE(excluded.image_url_small, image_url_small), hydrated_at = excluded.hydrated_at`,
		key, name, large, small, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM artist_genres WHERE artist_id = ?`, key); err != nil {
		return err
	}
	for _, g := range a.Genres {
		if g = strings.TrimSpace(g); g == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO artist_genres (artist_id, genre) VALUES (?, ?) ON CONFLICT DO NOTHING`,
			key, g); err != nil {
			return err
		}
	}
	return dequeue(ctx, tx, "artist", key)
}

func dequeue(ctx context.Context, tx *sql.Tx, typ, id string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM hydration_queue WHERE entity_type = ? AND id = ?`, typ, id)
	return err
}

// pickImages returns the largest image and the smallest one of at least
// 64 px (or the smallest at all). Spotify lists images largest first but
// that is not guaranteed; images without a width sort as 0.
func pickImages(imgs []spotify.Image) (large, small any) {
	var l, s *spotify.Image
	width := func(i *spotify.Image) int {
		if i.Width == nil {
			return 0
		}
		return *i.Width
	}
	for i := range imgs {
		im := &imgs[i]
		if im.URL == "" {
			continue
		}
		if l == nil || width(im) > width(l) {
			l = im
		}
		w := width(im)
		switch {
		case s == nil:
			s = im
		case w >= 64 && (width(s) < 64 || w < width(s)):
			s = im
		case width(s) < 64 && w > width(s):
			s = im
		}
	}
	if l == nil {
		return nil, nil
	}
	return l.URL, s.URL
}

func positive(n int64) any {
	if n <= 0 {
		return nil
	}
	return n
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
