package hydrate

import (
	"context"
	"database/sql"
	"strings"

	"spotussi/internal/spotify"
)

// Library seeding (D36): saved tracks and playlist items carry full track
// objects and saved albums full album objects, so library sync fills the
// catalog the way live capture does, without lookups. Library tracks are
// not queued for lookups of their own, and their unhydrated artists are
// queued only when the export has plays of the track (at the export
// plan's priority, its plays): the daily quota goes to what is listened
// to. A track that gets played later is queued by the usual rules.

// StoreLibraryTrack upserts a library track object inside the caller's
// transaction.
func StoreLibraryTrack(ctx context.Context, tx *sql.Tx, t *spotify.Track, now int64) error {
	key := t.Key()
	var n int64
	if !t.IsLocal {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_export_rows WHERE uri = ?`, "spotify:track:"+key).Scan(&n); err != nil {
			return err
		}
	}
	prio := int64(-1)
	if n > 0 {
		prio = n
	}
	return storeTrack(ctx, tx, key, t, SourceLibrary, now, prio)
}

// StoreLibraryAlbum upserts a saved album inside the caller's transaction;
// its unhydrated artists are not queued.
func StoreLibraryAlbum(ctx context.Context, tx *sql.Tx, a *spotify.SimpleAlbum, now int64) error {
	if a.ID == "" {
		return nil
	}
	return storeAlbum(ctx, tx, a, now, -1)
}

// NamePlaylistContext records a library playlist's name as its play
// context (D17), so playlists in the library are named without a lookup
// (manual names are kept). Spotify-made playlists are left alone.
func NamePlaylistContext(ctx context.Context, tx *sql.Tx, p *spotify.PlaylistSummary, now int64) error {
	if p.ID == "" || p.Name == "" || strings.HasPrefix(p.ID, SpotifyOwnedPrefix) {
		return nil
	}
	uri := "spotify:playlist:" + p.ID
	if _, err := tx.ExecContext(ctx, `INSERT INTO contexts (uri, type, name_source) VALUES (?, 'playlist', 'unresolved')
		ON CONFLICT (uri) DO NOTHING`, uri); err != nil {
		return err
	}
	large, _ := pickImages(p.Images)
	return setContext(ctx, tx, uri, p.Name, p.Owner.ID, large, now)
}
