package hydrate

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"spotussi/internal/spotify"
)

// Play contexts (D17): every context URI seen in live data gets a
// contexts row. Playlists are looked up once; albums and artists take
// their names from the catalog (looked up only when the catalog lacks
// them); Liked Songs, shows and anything else resolve without a request
// and keep a null name (the type says what they are).

// SpotifyOwnedPrefix starts the ID of every Spotify-made playlist
// (Discover Weekly, Daily Mix, Release Radar, editorial). Apps created
// after Nov 2024 cannot read them (D17), so they are not requested.
const SpotifyOwnedPrefix = "37i9dQZ"

// ParseContext returns a context URI's type (playlist, album, artist,
// show, collection, other) and the ID to look up ("" when there is
// nothing to look up).
func ParseContext(uri string) (typ, id string) {
	parts := strings.Split(uri, ":")
	if len(parts) < 3 || parts[0] != "spotify" {
		return "other", ""
	}
	last := parts[len(parts)-1]
	switch {
	case len(parts) == 3 && (parts[1] == "playlist" || parts[1] == "album" || parts[1] == "artist" || parts[1] == "show"):
		return parts[1], last
	case len(parts) == 5 && parts[1] == "user" && parts[3] == "playlist": // legacy spotify:user:NAME:playlist:ID
		return "playlist", last
	case last == "collection" || parts[1] == "collection":
		return "collection", ""
	}
	return "other", ""
}

// ensureContext records a context seen in live data and, while it is
// unresolved, enqueues it (priority: plays seen from it).
func ensureContext(ctx context.Context, tx *sql.Tx, uri string, now int64) error {
	typ, id := ParseContext(uri)
	if _, err := tx.ExecContext(ctx, `INSERT INTO contexts (uri, type, name_source) VALUES (?, ?, 'unresolved')
		ON CONFLICT (uri) DO NOTHING`, uri, typ); err != nil {
		return err
	}
	if id == "" || typ == "show" || (typ == "playlist" && strings.HasPrefix(id, SpotifyOwnedPrefix)) {
		// Nothing Spotify will tell us (D11: shows are out of scope).
		_, err := tx.ExecContext(ctx, `UPDATE contexts SET resolved_at = ? WHERE uri = ? AND resolved_at IS NULL`, now, uri)
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO hydration_queue (entity_type, id, priority)
		SELECT 'context', ?1, 1 WHERE EXISTS (SELECT 1 FROM contexts WHERE uri = ?1 AND resolved_at IS NULL)
		ON CONFLICT (entity_type, id) DO UPDATE SET priority = priority + 1`, uri)
	return err
}

// setContext records a resolved context. A manual name is never
// overwritten; name "" leaves the context unresolved (but done).
func setContext(ctx context.Context, tx *sql.Tx, uri, name, owner string, image any, now int64) error {
	src := "api"
	if name == "" {
		src = "unresolved"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE contexts SET name = ?, name_source = ?, owner_id = COALESCE(?, owner_id),
			image_url = COALESCE(?, image_url), resolved_at = ?
		WHERE uri = ? AND name_source <> 'manual'`, nullStr(name), src, nullStr(owner), image, now, uri); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE contexts SET resolved_at = ? WHERE uri = ? AND resolved_at IS NULL`, now, uri); err != nil {
		return err
	}
	return dequeue(ctx, tx, "context", uri)
}

// resolveContext resolves one queued context, through the catalog when
// it can. It reports whether a request was made.
func (h *Hydrator) resolveContext(ctx context.Context, uri string, prio int64) (requested bool, err error) {
	typ, id := ParseContext(uri)
	now := h.now().UnixMilli()
	var name string
	var image sql.NullString
	switch typ {
	case "album":
		err = h.db.Read.QueryRowContext(ctx, `SELECT name, image_url FROM albums WHERE id = ? AND hydrated_at IS NOT NULL`, id).
			Scan(&name, &image)
	case "artist":
		err = h.db.Read.QueryRowContext(ctx, `SELECT name, image_url FROM artists WHERE id = ? AND hydrated_at IS NOT NULL`, id).
			Scan(&name, &image)
	default:
		err = sql.ErrNoRows
	}
	if err == nil {
		return false, h.tx(ctx, func(tx *sql.Tx) error { return setContext(ctx, tx, uri, name, "", nullStr(image.String), now) })
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	switch typ {
	case "playlist":
		p, err := h.sp.Playlist(ctx, id)
		if code := spotify.StatusOf(err); code == http.StatusNotFound || code == http.StatusForbidden {
			// Deleted, someone else's private playlist, or Spotify-made
			// (D17): stays unresolved; the owner can name it by hand.
			return true, h.tx(ctx, func(tx *sql.Tx) error { return setContext(ctx, tx, uri, "", "", nil, now) })
		}
		if err != nil {
			return true, err
		}
		large, _ := pickImages(p.Images)
		return true, h.tx(ctx, func(tx *sql.Tx) error { return setContext(ctx, tx, uri, p.Name, p.Owner.ID, large, now) })
	case "album":
		a, err := h.sp.Album(ctx, id)
		if err != nil {
			return true, err
		}
		large, _ := pickImages(a.Images)
		return true, h.tx(ctx, func(tx *sql.Tx) error {
			if err := storeAlbum(ctx, tx, &a.SimpleAlbum, now, prio); err != nil {
				return err
			}
			return setContext(ctx, tx, uri, a.Name, "", large, now)
		})
	case "artist":
		a, err := h.sp.Artist(ctx, id)
		if err != nil {
			return true, err
		}
		large, _ := pickImages(a.Images)
		return true, h.tx(ctx, func(tx *sql.Tx) error {
			if err := storeArtist(ctx, tx, id, a, now); err != nil {
				return err
			}
			return setContext(ctx, tx, uri, a.Name, "", large, now)
		})
	}
	return false, h.tx(ctx, func(tx *sql.Tx) error { return setContext(ctx, tx, uri, "", "", nil, now) })
}
