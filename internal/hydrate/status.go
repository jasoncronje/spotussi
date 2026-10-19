package hydrate

import (
	"context"
	"time"

	"spotussi/internal/db"
)

// Status is catalog progress for system status.
type Status struct {
	State       string
	LastError   string
	LastErrorAt time.Time
	LastDoneAt  time.Time // last entry hydrated

	TracksTotal, TracksHydrated     int64
	AlbumsTotal, AlbumsHydrated     int64
	ArtistsTotal, ArtistsHydrated   int64
	ContextsTotal, ContextsResolved int64
	// Queue counts entries still to do per entity type (failures being
	// retried included); Failed those parked after a permanent failure.
	Queue  map[string]int64
	Failed int64
	// RatePerSec is the measured throughput (entries per second over the
	// last few minutes; 0 until measured).
	RatePerSec float64
	// ETA is when the queue should be empty (zero when idle or unknown),
	// counting whole quota windows once the quota is known.
	ETA time.Time
	// ResumesAt is when a spent lookup quota resets (zero: not spent).
	ResumesAt time.Time
	// QuotaLookups is how many lookups one quota window allowed, as last
	// observed (0: unknown).
	QuotaLookups int64
	// Fallbacks is each fallback source's progress, in stage order (D27).
	Fallbacks []FallbackStatus
}

// FallbackStatus is one fallback source's progress over the queue.
type FallbackStatus struct {
	Source, State string
	// Matched, Unmatched and Pending count queue entries the source found,
	// looked up without a confident match, and has still to look up;
	// Failed those being retried after a failure.
	Matched, Unmatched, Pending, Failed int64
	// ArtistsMatched and AlbumsMatched count the shared entities found.
	ArtistsMatched, AlbumsMatched int64
	Requests                      int64 // since start
	RatePerSec                    float64
	PausedUntil                   time.Time
	LastError                     string
	LastErrorAt                   time.Time
}

// QueueLength is the number of entries still to do.
func (s Status) QueueLength() int64 {
	var n int64
	for _, v := range s.Queue {
		n += v
	}
	return n
}

// ReadStatus counts catalog progress from the database alone (the worker
// is not running: state is disabled).
func ReadStatus(ctx context.Context, d *db.DB) (Status, error) {
	st := Status{State: StateDisabled, Queue: map[string]int64{"track": 0, "album": 0, "artist": 0, "context": 0}}
	err := d.Read.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM tracks),
			(SELECT COUNT(*) FROM tracks WHERE hydrated_at IS NOT NULL),
			(SELECT COUNT(*) FROM albums),
			(SELECT COUNT(*) FROM albums WHERE hydrated_at IS NOT NULL),
			(SELECT COUNT(*) FROM artists),
			(SELECT COUNT(*) FROM artists WHERE hydrated_at IS NOT NULL),
			(SELECT COUNT(*) FROM contexts),
			(SELECT COUNT(*) FROM contexts WHERE resolved_at IS NOT NULL)`).
		Scan(&st.TracksTotal, &st.TracksHydrated, &st.AlbumsTotal, &st.AlbumsHydrated,
			&st.ArtistsTotal, &st.ArtistsHydrated, &st.ContextsTotal, &st.ContextsResolved)
	if err != nil {
		return st, err
	}
	// Queued tracks and albums not in the catalog yet count towards the
	// totals (export tracks are known only by URI until hydrated).
	var tracksQueued, albumsQueued int64
	if err := d.Read.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM hydration_queue q WHERE q.entity_type = 'track' AND NOT EXISTS (SELECT 1 FROM tracks t WHERE t.id = q.id)),
			(SELECT COUNT(*) FROM hydration_queue q WHERE q.entity_type = 'album' AND NOT EXISTS (SELECT 1 FROM albums a WHERE a.id = q.id))`).
		Scan(&tracksQueued, &albumsQueued); err != nil {
		return st, err
	}
	st.TracksTotal += tracksQueued
	st.AlbumsTotal += albumsQueued
	rs, err := d.Read.QueryContext(ctx, `SELECT entity_type, COALESCE(last_error, '') LIKE ? AS failed, COUNT(*)
		FROM hydration_queue GROUP BY 1, 2`, permanentPrefix+"%")
	if err != nil {
		return st, err
	}
	defer rs.Close()
	for rs.Next() {
		var typ string
		var failed bool
		var n int64
		if err := rs.Scan(&typ, &failed, &n); err != nil {
			return st, err
		}
		if failed {
			st.Failed += n
		} else {
			st.Queue[typ] += n
		}
	}
	if err := rs.Err(); err != nil {
		return st, err
	}
	st.Fallbacks, err = readFallbackStatus(ctx, d)
	return st, err
}

// readFallbackStatus counts each source's outcomes over the queue
// (disabled: the worker fills in the runtime state).
func readFallbackStatus(ctx context.Context, d *db.DB) ([]FallbackStatus, error) {
	out := make([]FallbackStatus, 0, len(FallbackSources))
	for _, src := range FallbackSources {
		fs := FallbackStatus{Source: src, State: FallbackDisabled}
		err := d.Read.QueryRowContext(ctx, `SELECT
				COALESCE(SUM(f.status = 'matched'), 0), COALESCE(SUM(f.status = 'no_match'), 0),
				COALESCE(SUM(f.status = 'failed'), 0), COALESCE(SUM(f.id IS NULL OR f.status = 'failed'), 0)
			FROM hydration_queue q
			LEFT JOIN fallback_state f ON f.source = ?1 AND f.entity_type = q.entity_type AND f.id = q.id
			WHERE q.entity_type IN (`+fbTypes[src]+`)`, src).Scan(&fs.Matched, &fs.Unmatched, &fs.Failed, &fs.Pending)
		if err != nil {
			return nil, err
		}
		if err := d.Read.QueryRowContext(ctx, `SELECT
				(SELECT COUNT(*) FROM fallback_state WHERE source = ?1 AND entity_type = 'artist' AND status = 'matched'),
				(SELECT COUNT(*) FROM fallback_state WHERE source = ?1 AND entity_type = 'album' AND status = 'matched')`, src).
			Scan(&fs.ArtistsMatched, &fs.AlbumsMatched); err != nil {
			return nil, err
		}
		out = append(out, fs)
	}
	return out, nil
}

// Status reports catalog progress and the worker's state.
func (h *Hydrator) Status(ctx context.Context) (Status, error) {
	st, err := ReadStatus(ctx, h.db)
	if err != nil {
		return st, err
	}
	now := h.now()
	h.mu.Lock()
	st.State, st.LastError, st.LastErrorAt, st.LastDoneAt = h.state, h.lastError, h.lastErrorAt, h.lastDoneAt
	st.QuotaLookups, st.ResumesAt = h.quotaPerDay, h.resumesAt
	used := h.windowCount
	if !st.ResumesAt.After(now) {
		st.ResumesAt = time.Time{}
	}
	for i := range st.Fallbacks {
		fs := &st.Fallbacks[i]
		s := h.fbStat(fs.Source)
		fs.State, fs.LastError, fs.LastErrorAt = s.state, s.lastError, s.lastErrorAt
		fs.RatePerSec = rateOf(s.done, now)
		if c := h.fbClient(fs.Source); c != nil {
			fs.Requests, fs.PausedUntil = c.Requests(), c.Limiter().PausedUntil()
		}
	}
	st.RatePerSec = rateOf(h.done, now)
	h.mu.Unlock()

	remaining := st.QueueLength()
	if remaining == 0 || st.State == StateWaiting || st.State == StateDisabled {
		return st, nil
	}
	rate := st.RatePerSec
	var blocked time.Time
	if h.o.LimiterStats != nil {
		ls := h.o.LimiterStats()
		if rate == 0 {
			rate = ls.EffectiveRatePerSec
		}
		blocked = ls.BlockedUntil
	}
	if rate <= 0 {
		return st, nil
	}
	start := now
	if blocked.After(start) {
		start = blocked
	}
	if q := st.QuotaLookups; q > 0 {
		// Whole quota windows (a day each) for what this one cannot
		// take, then the rest at the rate.
		left := remaining
		if st.ResumesAt.IsZero() {
			left -= max(q-used, 0) // still allowed in this window
			if left <= 0 {
				st.ETA = start.Add(rateTime(remaining, rate))
				return st, nil
			}
			start = nextReset(now)
		} else {
			start = st.ResumesAt
		}
		days := (left - 1) / q
		st.ETA = start.Add(time.Duration(days) * 24 * time.Hour).Add(rateTime(left-days*q, rate))
		return st, nil
	}
	if !st.ResumesAt.IsZero() {
		start = st.ResumesAt
	}
	st.ETA = start.Add(rateTime(remaining, rate))
	return st, nil
}

// rateOf is the throughput of completions (oldest first) over the last
// throughputWindow; 0 until at least two.
func rateOf(done []time.Time, now time.Time) float64 {
	n := 0
	for _, t := range done {
		if now.Sub(t) <= throughputWindow {
			n++
		}
	}
	if n < 2 {
		return 0
	}
	span := min(now.Sub(done[len(done)-n]), throughputWindow)
	if span <= 0 {
		return 0
	}
	return float64(n) / span.Seconds()
}

func rateTime(n int64, rate float64) time.Duration {
	return time.Duration(float64(n) / rate * float64(time.Second))
}

// nextReset estimates the next quota reset: observed around 00:08 UTC
// (Oct 2026), so the next UTC midnight plus 10 minutes.
func nextReset(now time.Time) time.Time {
	u := now.UTC()
	r := time.Date(u.Year(), u.Month(), u.Day(), 0, 10, 0, 0, time.UTC)
	if !r.After(u) {
		r = r.Add(24 * time.Hour)
	}
	return r
}
