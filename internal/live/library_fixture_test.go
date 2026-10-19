package live_test

import (
	"context"
	"database/sql"
	"fmt"

	"spotussi/internal/libview"
	"spotussi/internal/mock"
	"spotussi/internal/refdata"
)

// writeLibrary writes the dataset's synthetic library the way library
// sync would have stored it (D36): never-played tracks into the catalog,
// one sync per Library.Syncs, spans and playlists.
func writeLibrary(ctx context.Context, tx *sql.Tx, ds *refdata.Dataset) error {
	exec := func(q string, args ...any) error { _, err := tx.ExecContext(ctx, q, args...); return err }
	lib := &ds.Library
	for _, t := range lib.Extra {
		if err := exec(`INSERT INTO tracks (id, name, duration_ms, explicit, album_id, source, hydrated_at)
			VALUES (?, ?, ?, 0, ?, 'library', 1)`, t.ID, t.Name, int64(t.DurationMs), ds.Albums[t.Album].ID); err != nil {
			return err
		}
		for p, a := range t.Artists {
			if err := exec(`INSERT INTO track_artists (track_id, artist_id, position) VALUES (?, ?, ?)`, t.ID, ds.Artists[a].ID, p); err != nil {
				return err
			}
		}
	}
	for k, at := range lib.Syncs {
		if err := exec(`INSERT INTO library_syncs (id, kind, started_at, finished_at, requests) VALUES (?, 'full', ?, ?, 3)`,
			k+1, at, at+mock.SyncTookMs); err != nil {
			return err
		}
	}
	last := len(lib.Syncs)
	firstSeen := func(at int64) int {
		for k, s := range lib.Syncs {
			if s > at {
				return k + 1
			}
		}
		return last
	}
	spans := func(list string, xs []refdata.Saved, uri func(int32) string) error {
		pos := 0
		for _, s := range xs {
			var added, by, gone any
			if s.AddedAt > 0 {
				added = s.AddedAt
			}
			if s.AddedBy != "" {
				by = s.AddedBy
			}
			lastSeen, p := last, pos
			if s.Present() {
				pos++
			} else {
				gone, lastSeen = s.Gone+1, s.Gone
			}
			if err := exec(`INSERT INTO library_spans (list, item_uri, added_at, added_by, position, first_seen_sync,
				last_seen_sync, gone_sync, json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}')`,
				list, uri(s.Item), added, by, p, firstSeen(s.AddedAt), lastSeen, gone); err != nil {
				return err
			}
		}
		return nil
	}
	track := func(i int32) string { return "spotify:track:" + ds.LibTrack(i).ID }
	if err := spans("tracks", lib.Tracks, track); err != nil {
		return err
	}
	if err := spans("albums", lib.Albums, func(i int32) string { return "spotify:album:" + ds.Albums[i].ID }); err != nil {
		return err
	}
	for i, pl := range lib.Playlists {
		var img any
		if !libview.SpotifyOwned(pl.ID) {
			img = mock.ImagePath + "playlist/" + pl.ID + ".svg"
		}
		snap := fmt.Sprintf("snap-%d", i)
		var itemsSnap any
		if pl.ItemsAvailable {
			itemsSnap = snap
		}
		if err := exec(`INSERT INTO library_playlists (id, name, owner_id, owner_name, relation, collaborative, image_url,
			snapshot_id, items_total, items_available, items_snapshot_id, position, first_seen_sync, last_seen_sync, json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, '{}')`, pl.ID, pl.Name, pl.OwnerID, pl.OwnerName, pl.Relation,
			pl.Relation == "collaborative", img, snap, pl.Total, pl.ItemsAvailable, itemsSnap, i, last); err != nil {
			return err
		}
		if err := spans(pl.ID, pl.Items, track); err != nil {
			return err
		}
	}
	for _, l := range []string{"tracks", "albums", "playlists"} {
		if err := exec(`INSERT INTO library_walks (sync_id, list, mode, complete, total) VALUES (?, ?, 'walk', 1, 0)`, last, l); err != nil {
			return err
		}
	}
	return exec(`INSERT INTO spotify_connection (id, client_id, account_id, account_name, scopes, refresh_token, connected_at)
		VALUES (1, 'test', ?, 'Listener', 'user-library-read', x'00', 1)`, refdata.Me)
}
