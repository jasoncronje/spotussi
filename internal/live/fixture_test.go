package live_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"testing"

	"spotussi/internal/db"
	"spotussi/internal/mock"
	"spotussi/internal/refdata"
)

// loadRefdata writes a reference dataset into the raw layer and catalog
// the way capture, import and hydration would have, so derivation has
// to reconstruct the dataset's plays (D8) and entities from scratch:
//
//   - every export-confirmed row becomes a raw export row (ts = end);
//   - every poller-captured row a raw poller play;
//   - every row recently-played saw a raw recent item with played_at at
//     its end (the default calibration verdict);
//   - the catalog holds what hydration would have stored: unhydrated
//     tracks, albums and artists have no duration, artwork, release
//     data or genres. Artwork URLs are the mock's placeholder URLs.
//
// Live track JSON carries no durations or album details, so everything
// resolution reads comes from the catalog, as on a hydrated instance.
func loadRefdata(t testing.TB, d *db.DB, ds *refdata.Dataset) {
	t.Helper()
	if err := writeRefdata(context.Background(), d, ds, 0, len(ds.Plays)); err != nil {
		t.Fatal(err)
	}
}

// writeRefdata writes the raw rows of plays [from, to), and the catalog
// with the first batch.
func writeRefdata(ctx context.Context, d *db.DB, ds *refdata.Dataset, from, to int) error {
	tx, err := d.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if from == 0 {
		if err := writeCatalog(ctx, tx, ds); err != nil {
			return err
		}
	}
	if err := writeRaw(ctx, tx, ds, from, to); err != nil {
		return err
	}
	return tx.Commit()
}

func writeCatalog(ctx context.Context, tx *sql.Tx, ds *refdata.Dataset) error {
	img := func(kind, id string) any { return mock.ImagePath + kind + "/" + id + ".svg" }

	albumHydrated := make([]bool, len(ds.Albums))
	for _, t := range ds.Tracks {
		if t.Hydrated {
			albumHydrated[t.Album] = true
		}
	}
	exec := func(q string, args ...any) error { _, err := tx.ExecContext(ctx, q, args...); return err }
	for _, a := range ds.Artists {
		var image, at any
		if a.Hydrated {
			image, at = img("artist", a.ID), int64(1)
			for _, g := range a.Genres {
				if err := exec(`INSERT INTO artist_genres (artist_id, genre) VALUES (?, ?)`, a.ID, ds.Genres[g].Name); err != nil {
					return err
				}
			}
		}
		if err := exec(`INSERT INTO artists (id, name, image_url, hydrated_at) VALUES (?, ?, ?, ?)`, a.ID, a.Name, image, at); err != nil {
			return err
		}
	}
	for i, al := range ds.Albums {
		var typ, date, total, image, at any
		if albumHydrated[i] {
			typ, date, total, image, at = al.Type, al.ReleaseDate, al.TotalTracks, img("album", al.ID), int64(1)
		}
		if err := exec(`INSERT INTO albums (id, name, album_type, release_date, total_tracks, image_url, hydrated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, al.ID, al.Name, typ, date, total, image, at); err != nil {
			return err
		}
		for p, a := range al.Artists {
			if err := exec(`INSERT INTO album_artists (album_id, artist_id, position) VALUES (?, ?, ?)`, al.ID, ds.Artists[a].ID, p); err != nil {
				return err
			}
		}
	}
	for _, t := range ds.Tracks {
		var dur, explicit, at any
		if t.Hydrated {
			dur, explicit, at = int64(t.DurationMs), t.Explicit, int64(1)
		}
		if err := exec(`INSERT INTO tracks (id, name, duration_ms, explicit, album_id, source, hydrated_at)
			VALUES (?, ?, ?, ?, ?, 'api', ?)`, t.ID, t.Name, dur, explicit, ds.Albums[t.Album].ID, at); err != nil {
			return err
		}
		for p, a := range t.Artists {
			if err := exec(`INSERT INTO track_artists (track_id, artist_id, position) VALUES (?, ?, ?)`, t.ID, ds.Artists[a].ID, p); err != nil {
				return err
			}
		}
	}
	for _, c := range ds.Contexts {
		var err error
		switch {
		case c.Type == "collection":
			err = exec(`INSERT INTO contexts (uri, type, name, name_source) VALUES (?, 'collection', ?, 'api')`, c.URI, c.Name)
		case c.Type == "playlist" && c.NameSource == "api":
			err = exec(`INSERT INTO contexts (uri, type, name, name_source, image_url) VALUES (?, 'playlist', ?, 'api', ?)`,
				c.URI, c.Name, img("playlist", c.ID))
		case c.Type == "playlist" && c.NameSource == "manual":
			err = exec(`INSERT INTO contexts (uri, type, name, name_source) VALUES (?, 'playlist', ?, 'manual')`, c.URI, c.Name)
		case c.Type == "playlist":
			err = exec(`INSERT INTO contexts (uri, type, name_source) VALUES (?, 'playlist', 'unresolved')`, c.URI)
		}
		if err != nil {
			return err
		}
	}

	return exec(`INSERT INTO imports (id, filename, origin, kind, state, created_at) VALUES (1, 'refdata', 'cli', 'zip', 'done', 0)`)
}

func writeRaw(ctx context.Context, tx *sql.Tx, ds *refdata.Dataset, from, to int) error {
	exp, err := tx.PrepareContext(ctx, `INSERT INTO raw_export_rows (import_id, row_hash, ts_ms, ms_played, content_type, uri,
		track_name, artist_name, album_name, reason_start, reason_end, shuffle, skipped, offline, platform, conn_country, json)
		VALUES (1, ?, ?, ?, 'track', ?, ?, ?, ?, 'trackdone', ?, ?, ?, 0, ?, 'ZZ', '{}')`)
	if err != nil {
		return err
	}
	pol, err := tx.PrepareContext(ctx, `INSERT INTO raw_poller_plays (track_id, track_uri, duration_ms, started_ms, ended_ms,
		ms_played, skipped, end_reason, shuffle, device_name, device_type, context_uri, observations, first_seen_ms,
		last_seen_ms, json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'track_change', ?, ?, ?, ?, 3, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	rec, err := tx.PrepareContext(ctx, `INSERT INTO raw_recent_plays (played_at_ms, track_id, duration_ms, context_uri, json, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for n := from; n < to; n++ {
		p := ds.Plays[n]
		t := ds.Tracks[p.Track]
		end := p.EndMs()
		var shuffle, ctxURI, devName, devType, platform any
		if p.Has(refdata.FlagShuffleKnown) {
			shuffle = p.Has(refdata.FlagShuffle)
		}
		if p.Context >= 0 {
			ctxURI = ds.Contexts[p.Context].URI
		}
		if p.Device >= 0 {
			devName, devType = ds.Devices[p.Device].Name, ds.Devices[p.Device].Type
		}
		if p.Platform >= 0 {
			platform = ds.Platforms[p.Platform]
		}
		skipped := p.Has(refdata.FlagSkipped)
		if p.Has(refdata.FlagSrcExport) {
			reason := "trackdone"
			if skipped {
				reason = "fwdbtn"
			}
			var h [8]byte
			binary.BigEndian.PutUint64(h[:], uint64(n))
			sum := sha256.Sum256(h[:])
			if _, err := exp.ExecContext(ctx, sum[:], end, p.MsPlayed, "spotify:track:"+t.ID, t.Name,
				ds.Artists[t.Artists[0]].Name, ds.Albums[t.Album].Name, reason, shuffle, skipped, platform); err != nil {
				return err
			}
		}
		item, _ := json.Marshal(map[string]any{"id": t.ID, "uri": "spotify:track:" + t.ID, "name": t.Name})
		if p.Has(refdata.FlagSrcPoller) {
			var dur int64
			if t.Hydrated {
				dur = int64(t.DurationMs)
			}
			if _, err := pol.ExecContext(ctx, t.ID, "spotify:track:"+t.ID, dur, p.StartMs, end, p.MsPlayed, skipped,
				shuffle, devName, devType, ctxURI, p.StartMs, end, `{"item":`+string(item)+`}`, end); err != nil {
				return err
			}
		}
		if p.Has(refdata.FlagSrcRecent) {
			if _, err := rec.ExecContext(ctx, end, t.ID, int64(t.DurationMs), ctxURI, `{"track":`+string(item)+`}`, end); err != nil {
				return err
			}
		}
	}
	return nil
}
