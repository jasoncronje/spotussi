package capture

import (
	"context"
	"log/slog"
	"math"
	"slices"
	"time"

	"spotussi/internal/db"
)

// played_at calibration (M2 spike). Spotify documents recently-played's
// played_at only as "the date and time the track was played". Whether it
// marks the start or the end of a play decides how imported and
// backstop-only plays are placed on the timeline, so it is measured: each
// recently-played item is matched to the poller play of the same track
// that overlaps it, and played_at - started_ms and played_at - ended_ms
// are recorded in capture_calibration. Whichever offset clusters around
// zero is the anchor.

const (
	// calibrationWindowMs widens a poller play's [start, end] when
	// matching a recently-played item to it.
	calibrationWindowMs = 60_000
	// calibrationHitMs is how close an offset must be to zero to count as
	// a hit for the verdict.
	calibrationHitMs = 5_000
	// calibrationMinPlayedMs: plays shorter than this cannot tell start
	// from end (both offsets are near zero), so they are left out of the
	// verdict.
	calibrationMinPlayedMs = 30_000
	// calibrationMinSamples discriminating samples are needed for a verdict.
	calibrationMinSamples = 20
	// calibrationRecent bounds the summary to the newest samples.
	calibrationRecent = 2000
)

// Calibration verdicts.
const (
	AnchorStart        = "start"             // played_at marks when the play started
	AnchorEnd          = "end"               // played_at marks when the play ended
	AnchorUndetermined = "undetermined"      // enough samples, no clear pattern
	AnchorInsufficient = "insufficient_data" // not enough matched plays yet
)

// OffsetStats summarizes played_at minus a poller timestamp, in ms.
type OffsetStats struct {
	MedianMs int64
	P10Ms    int64
	P90Ms    int64
	// HitRate is the share of discriminating samples within ±5 s.
	HitRate float64
}

// Calibration summarizes the played_at offsets measured so far.
type Calibration struct {
	Samples        int // matched pairs (newest calibrationRecent at most)
	Discriminating int // of which played for at least 30 s
	Start          OffsetStats
	End            OffsetStats
	Verdict        string
	UpdatedAt      time.Time // newest sample's computation time
}

// Calibration summarizes capture_calibration.
func (s *Service) Calibration(ctx context.Context) (*Calibration, error) {
	return ReadCalibration(ctx, s.db)
}

// ReadCalibration summarises the stored calibration samples; it works
// without a running service (the derivation engine reads the verdict
// when capture is disabled).
func ReadCalibration(ctx context.Context, d *db.DB) (*Calibration, error) {
	rows, err := d.Read.QueryContext(ctx, `SELECT start_offset_ms, end_offset_ms, ms_played, computed_at
		FROM capture_calibration ORDER BY played_at_ms DESC LIMIT ?`, calibrationRecent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var starts, ends []int64
	c := &Calibration{}
	var updated int64
	for rows.Next() {
		var so, eo, played, at int64
		if err := rows.Scan(&so, &eo, &played, &at); err != nil {
			return nil, err
		}
		c.Samples++
		updated = max(updated, at)
		if played >= calibrationMinPlayedMs {
			starts, ends = append(starts, so), append(ends, eo)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if updated > 0 {
		c.UpdatedAt = time.UnixMilli(updated)
	}
	c.Discriminating = len(starts)
	c.Start, c.End = offsetStats(starts), offsetStats(ends)
	c.Verdict = verdict(c)
	return c, nil
}

func offsetStats(v []int64) OffsetStats {
	if len(v) == 0 {
		return OffsetStats{}
	}
	v = slices.Clone(v)
	slices.Sort(v)
	pct := func(p float64) int64 { return v[int(math.Round(p*float64(len(v)-1)))] }
	hits := 0
	for _, x := range v {
		if x >= -calibrationHitMs && x <= calibrationHitMs {
			hits++
		}
	}
	return OffsetStats{MedianMs: pct(0.5), P10Ms: pct(0.1), P90Ms: pct(0.9), HitRate: float64(hits) / float64(len(v))}
}

// verdict names the anchor when one offset is within ±5 s for at least
// 80% of discriminating samples and the other for at most half as many.
func verdict(c *Calibration) string {
	if c.Discriminating < calibrationMinSamples {
		return AnchorInsufficient
	}
	switch {
	case c.End.HitRate >= 0.8 && c.Start.HitRate <= c.End.HitRate/2:
		return AnchorEnd
	case c.Start.HitRate >= 0.8 && c.End.HitRate <= c.Start.HitRate/2:
		return AnchorStart
	}
	return AnchorUndetermined
}

type calCandidate struct {
	playedAt           int64
	trackID            string
	pollerID           int64
	started, ended     int64
	msPlayed, duration int64
}

// distance from played_at to the poller play's [start, end] interval
// (0 inside it), then to its nearer edge.
func (c calCandidate) rank() (int64, int64) {
	outside := max(c.started-c.playedAt, c.playedAt-c.ended, 0)
	edge := min(abs(c.playedAt-c.started), abs(c.playedAt-c.ended))
	return outside, edge
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// calibrate matches new recently-played items to poller plays, stores the
// offsets, and returns the updated summary. Private-session plays are
// skipped (Spotify leaves them out of recently-played), and each side is
// matched at most once, nearest first.
func (s *Service) calibrate(ctx context.Context) (*Calibration, error) {
	rows, err := s.db.Read.QueryContext(ctx, `SELECT r.played_at_ms, r.track_id, p.id, p.started_ms, p.ended_ms, p.ms_played, p.duration_ms
		FROM raw_recent_plays r
		JOIN raw_poller_plays p ON p.track_id = r.track_id AND p.private_session = 0
			AND r.played_at_ms BETWEEN p.started_ms - ? AND p.ended_ms + ?
		WHERE NOT EXISTS (SELECT 1 FROM capture_calibration c WHERE c.played_at_ms = r.played_at_ms AND c.track_id = r.track_id)
			AND NOT EXISTS (SELECT 1 FROM capture_calibration c WHERE c.poller_play_id = p.id)`,
		calibrationWindowMs, calibrationWindowMs)
	if err != nil {
		return nil, err
	}
	var cands []calCandidate
	for rows.Next() {
		var c calCandidate
		if err := rows.Scan(&c.playedAt, &c.trackID, &c.pollerID, &c.started, &c.ended, &c.msPlayed, &c.duration); err != nil {
			rows.Close()
			return nil, err
		}
		cands = append(cands, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(cands, func(a, b calCandidate) int {
		ao, ae := a.rank()
		bo, be := b.rank()
		if ao != bo {
			return cmpInt(ao, bo)
		}
		if ae != be {
			return cmpInt(ae, be)
		}
		return cmpInt(a.playedAt, b.playedAt)
	})
	type recentKey struct {
		at int64
		id string
	}
	usedRecent := map[recentKey]bool{}
	usedPoller := map[int64]bool{}
	var picked []calCandidate
	for _, c := range cands {
		k := recentKey{c.playedAt, c.trackID}
		if usedRecent[k] || usedPoller[c.pollerID] {
			continue
		}
		usedRecent[k], usedPoller[c.pollerID] = true, true
		picked = append(picked, c)
	}
	if len(picked) > 0 {
		tx, err := s.db.Write.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback() //nolint:errcheck // no-op after commit
		now := s.now().UnixMilli()
		for _, c := range picked {
			if _, err := tx.ExecContext(ctx, `INSERT INTO capture_calibration
				(played_at_ms, track_id, poller_play_id, start_offset_ms, end_offset_ms, ms_played, duration_ms, computed_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
				c.playedAt, c.trackID, c.pollerID, c.playedAt-c.started, c.playedAt-c.ended, c.msPlayed, c.duration, now); err != nil {
				return nil, err
			}
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	cal, err := s.Calibration(ctx)
	if err != nil {
		return nil, err
	}
	if len(picked) > 0 {
		slog.Info("capture: played_at calibration", "new_samples", len(picked), "samples", cal.Samples,
			"discriminating", cal.Discriminating, "verdict", cal.Verdict,
			"start_offset_median_ms", cal.Start.MedianMs, "start_hit_rate", round2(cal.Start.HitRate),
			"end_offset_median_ms", cal.End.MedianMs, "end_hit_rate", round2(cal.End.HitRate))
	}
	return cal, nil
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
