package hydrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"spotussi/internal/spotify"
)

// Seeding (D9.6): live plays arrive hydrated. Poller rows keep the last
// /me/player response and recently-played rows the verbatim item, both
// with a full track object (album and simplified artists included), so
// the catalog is filled from them for free. Progress is a watermark per
// table, stored in settings in the same transaction as the rows it
// covers: the first run backfills every existing row, later runs only
// what was captured since.

const (
	seedPollerKey = "hydrate_seed_poller_id"     // last raw_poller_plays.id seeded
	seedRecentKey = "hydrate_seed_recent_fetch"  // last raw_recent_plays.fetched_at seeded
	planExportKey = "hydrate_plan_export_row_id" // last raw_export_rows.id enqueued
	seedChunk     = 500
)

func readMark(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (int64, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(raw, 10, 64)
}

func writeMark(ctx context.Context, tx *sql.Tx, key string, v, now int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, strconv.FormatInt(v, 10), now)
	return err
}

// livePriority is the queue priority for the artists of a track seen
// live: its plays in the export (if any) plus one.
func livePriority(ctx context.Context, tx *sql.Tx, trackID string) (int64, error) {
	var n int64
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_export_rows WHERE uri = ?`, "spotify:track:"+trackID).Scan(&n)
	return n + 1, err
}

type seedRow struct {
	mark       int64
	raw        string
	contextURI sql.NullString
}

// Seed upserts the catalog from live rows captured since the last run
// and reports how many tracks it wrote. It is safe to call concurrently
// with Run (seeding is serialised).
func (h *Hydrator) Seed(ctx context.Context) (int, error) {
	h.seedMu.Lock()
	defer h.seedMu.Unlock()
	n1, err := h.seedTable(ctx, seedPollerKey, `SELECT id, json, context_uri FROM raw_poller_plays
		WHERE id > ? ORDER BY id LIMIT ?`, func(raw string) *spotify.Track {
		var st spotify.PlayerState
		if json.Unmarshal([]byte(raw), &st) != nil || st.Item == nil || (st.Item.Type != "" && st.Item.Type != "track") {
			return nil
		}
		return st.Item
	}, SourcePoller)
	if err != nil {
		return n1, err
	}
	// fetched_at is shared by every item of one backstop run, so a chunk
	// never ends inside a run: the LIMIT counts runs (up to 500 items
	// each), not rows.
	n2, err := h.seedTable(ctx, seedRecentKey, `SELECT fetched_at, json, context_uri FROM raw_recent_plays
		WHERE fetched_at IN (SELECT DISTINCT fetched_at FROM raw_recent_plays WHERE fetched_at > ?1 ORDER BY fetched_at LIMIT ?2 / 100)
		ORDER BY fetched_at`, func(raw string) *spotify.Track {
		var it spotify.RecentItem
		if json.Unmarshal([]byte(raw), &it) != nil || (it.Track.ID == "" && !it.Track.IsLocal) {
			return nil
		}
		return &it.Track
	}, SourceRecent)
	return n1 + n2, err
}

func (h *Hydrator) seedTable(ctx context.Context, key, query string, parse func(string) *spotify.Track, source string) (int, error) {
	written := 0
	for {
		mark, err := readMark(ctx, h.db.Read, key)
		if err != nil {
			return written, err
		}
		rs, err := h.db.Read.QueryContext(ctx, query, mark, seedChunk)
		if err != nil {
			return written, err
		}
		var rows []seedRow
		for rs.Next() {
			var r seedRow
			if err := rs.Scan(&r.mark, &r.raw, &r.contextURI); err != nil {
				rs.Close()
				return written, err
			}
			rows = append(rows, r)
		}
		rs.Close()
		if err := rs.Err(); err != nil {
			return written, err
		}
		if len(rows) == 0 {
			return written, nil
		}
		n := 0
		now := h.now().UnixMilli()
		err = h.tx(ctx, func(tx *sql.Tx) error {
			for _, r := range rows {
				if t := parse(r.raw); t != nil {
					key := t.Key()
					prio, err := livePriority(ctx, tx, key)
					if err != nil {
						return err
					}
					if err := storeTrack(ctx, tx, key, t, source, now, prio); err != nil {
						return err
					}
					n++
				}
				if r.contextURI.Valid && r.contextURI.String != "" {
					if err := ensureContext(ctx, tx, r.contextURI.String, now); err != nil {
						return err
					}
				}
			}
			return writeMark(ctx, tx, key, rows[len(rows)-1].mark, now)
		})
		if err != nil {
			return written, err
		}
		written += n
		if n > 0 {
			h.changed(n)
		}
	}
}
