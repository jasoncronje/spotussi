package hydrate

import (
	"context"
	"database/sql"
)

// Planning: export rows (D10) name tracks by URI only, so every track URI
// in raw_export_rows that is not hydrated is queued, most-played first
// (D9.1). A play here is a stream of at least 30 s, the minimum of every
// play rule (D7); shorter streams still queue the track, at priority 0.
// Each pass handles the export rows added since the last one (watermark
// in settings) and recomputes the priority of every track they mention,
// so a later, overlapping export reorders the queue rather than
// duplicating it.

const trackURIPrefix = "spotify:track:"

// Plan queues the tracks of export rows imported since the last pass and
// reports how many queue entries it added or reprioritised.
func (h *Hydrator) Plan(ctx context.Context) (int64, error) {
	h.planMu.Lock()
	defer h.planMu.Unlock()
	mark, err := readMark(ctx, h.db.Read, planExportKey)
	if err != nil {
		return 0, err
	}
	var top int64
	if err := h.db.Read.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM raw_export_rows`).Scan(&top); err != nil {
		return 0, err
	}
	if top <= mark {
		return 0, nil
	}
	var n int64
	err = h.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `WITH new AS (
				SELECT DISTINCT uri FROM raw_export_rows
				WHERE id > ?1 AND id <= ?2 AND content_type = 'track' AND uri LIKE 'spotify:track:%'
			), counted AS (
				SELECT n.uri, (SELECT COALESCE(SUM(r.ms_played >= 30000), 0) FROM raw_export_rows r WHERE r.uri = n.uri) AS plays
				FROM new n
			)
			INSERT INTO hydration_queue (entity_type, id, priority)
			SELECT 'track', substr(uri, ?3), plays FROM counted c
			WHERE NOT EXISTS (SELECT 1 FROM tracks t WHERE t.id = substr(c.uri, ?3) AND t.hydrated_at IS NOT NULL)
			ON CONFLICT (entity_type, id) DO UPDATE SET priority = excluded.priority`,
			mark, top, len(trackURIPrefix)+1)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		return writeMark(ctx, tx, planExportKey, top, h.now().UnixMilli())
	})
	if err != nil {
		return 0, err
	}
	if n > 0 {
		h.wakeWorker()
		h.wakeFallbacks()
	}
	return n, nil
}

// repair queues catalog entries that are incomplete but not queued (only
// possible if an earlier version or a manual edit left them so): artists
// known by name only and unresolved contexts.
func (h *Hydrator) repair(ctx context.Context) error {
	return h.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO hydration_queue (entity_type, id, priority)
			SELECT 'artist', id, 0 FROM artists WHERE hydrated_at IS NULL
			ON CONFLICT (entity_type, id) DO NOTHING`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO hydration_queue (entity_type, id, priority)
			SELECT 'context', uri, 0 FROM contexts WHERE resolved_at IS NULL
			ON CONFLICT (entity_type, id) DO NOTHING`)
		return err
	})
}
