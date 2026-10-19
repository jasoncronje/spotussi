package derive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"spotussi/internal/db"
)

// Engine owns the derived layer: it builds snapshots, keeps the plays and
// sessions tables in step with them, and decides when to rebuild.
//
//   - Startup: if the inputs' fingerprint matches the last full rebuild,
//     the plays table is loaded as is; otherwise a full rebuild runs.
//   - Full rebuild (raw import, catalog change, settings or exclusion
//     change, merge logic change): merge all raw rows, resolve entities,
//     write plays and sessions into new tables and swap them in, in one
//     transaction; then publish the new snapshot.
//   - Incremental (live plays): re-merge the raw rows after the last
//     listening gap before the new rows, replace those plays in memory
//     and in the table.
//   - Settings and exclusion changes apply to the published snapshot
//     immediately (cheap), then a full rebuild refreshes the tables.
//
// Hooks (CatalogChanged, RawChanged, LiveChanged, ...) never block: they
// set a flag and wake the worker.
type Engine struct {
	db              *db.DB
	settings        func(context.Context) (Settings, error)
	playedAtIsStart func(context.Context) bool
	now             func() time.Time

	snap  atomic.Pointer[Snapshot]
	ready chan struct{} // closed once the first snapshot is published

	runMu sync.Mutex // serializes rebuilds and incremental merges

	mu         sync.Mutex
	cur        Settings
	verdict    bool // played_at-is-start verdict of the last rebuild
	excl       []Exclusion
	inited     bool      // cur and excl hold real inputs (start, or a first rebuild)
	fullAt     time.Time // pending full rebuild not before this (zero: none)
	reason     string    // why the pending full rebuild runs
	incPending bool
	lastFull   time.Time // end of the last full rebuild
	marks      watermarks
	status     Status
	wake       chan struct{}
}

// Status is the derive part of system status.
type Status struct {
	State         string  // idle, rebuilding
	Progress      float64 // 0-1 while rebuilding
	LastRebuildAt int64   // epoch ms, 0: never
	LastRebuildMs int64
	LastError     string
}

// Options configure an Engine.
type Options struct {
	DB *db.DB
	// Settings reads the current derivation settings.
	Settings func(context.Context) (Settings, error)
	// PlayedAtIsStart reports whether the capture calibration measured
	// recently-played's played_at to be the start (nil: never).
	PlayedAtIsStart func(context.Context) bool
	Now             func() time.Time
}

// Debounce intervals.
const (
	// A catalog change rebuilds after this quiet period, and at most once
	// per catalogMinInterval while hydration keeps reporting batches.
	catalogQuiet       = 5 * time.Second
	catalogMinInterval = time.Minute
	// Settings, exclusion and raw changes rebuild after a short quiet
	// period (they already apply in memory, or arrive as one import).
	shortQuiet = 2 * time.Second
	// pollInterval is how often raw tables are checked for rows nobody
	// announced (e.g. `spotussi import` into a running server's database).
	pollInterval = 30 * time.Second
)

// New builds an engine. Run starts it.
func New(o Options) *Engine {
	e := &Engine{db: o.DB, settings: o.Settings, playedAtIsStart: o.PlayedAtIsStart, now: o.Now,
		ready: make(chan struct{}), wake: make(chan struct{}, 1), status: Status{State: "rebuilding"}}
	if e.now == nil {
		e.now = time.Now
	}
	if e.playedAtIsStart == nil {
		e.playedAtIsStart = func(context.Context) bool { return false }
	}
	return e
}

// Snapshot returns the current snapshot, waiting for the first one (until
// ctx ends).
func (e *Engine) Snapshot(ctx context.Context) (*Snapshot, error) {
	select {
	case <-e.ready:
		return e.snap.Load(), nil
	case <-ctx.Done():
		return nil, fmt.Errorf("derived data is still loading: %w", ctx.Err())
	}
}

// Status reports progress for system status.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

func (e *Engine) poke() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// scheduleFull asks for a full rebuild at the latest at at.
func (e *Engine) scheduleFull(at time.Time, reason string) {
	e.mu.Lock()
	if e.fullAt.IsZero() || at.Before(e.fullAt) {
		e.fullAt, e.reason = at, reason
	}
	e.mu.Unlock()
	e.poke()
}

// CatalogChanged tells the engine that hydration (M4) committed catalog
// rows: tracks, albums, artists, track_artists, album_artists,
// artist_genres or contexts. It never blocks. A full rebuild follows once
// calls stop for catalogQuiet, and at most once per catalogMinInterval,
// so calling it after every hydration batch is fine.
func (e *Engine) CatalogChanged() {
	e.mu.Lock()
	at := e.now().Add(catalogQuiet)
	if next := e.lastFull.Add(catalogMinInterval); at.Before(next) {
		at = next
	}
	switch {
	case e.fullAt.IsZero():
		e.fullAt, e.reason = at, "catalog"
	case e.reason == "catalog":
		e.fullAt = at // debounce: wait for the batches to pause
	}
	e.mu.Unlock()
	e.poke()
}

// RawChanged tells the engine that export rows were imported.
func (e *Engine) RawChanged() { e.scheduleFull(e.now().Add(shortQuiet), "import") }

// LiveChanged tells the engine that capture stored live rows (poller
// plays or recently-played items); they are merged incrementally.
func (e *Engine) LiveChanged() {
	e.mu.Lock()
	e.incPending = true
	e.mu.Unlock()
	e.poke()
}

// SettingsChanged applies new derivation settings to the published
// snapshot now, and refreshes the tables in the background.
func (e *Engine) SettingsChanged(s Settings) {
	e.mu.Lock()
	changed := !s.Equal(e.cur)
	e.cur = s
	if changed {
		e.republishLocked()
	}
	e.mu.Unlock()
	if changed {
		e.scheduleFull(e.now().Add(shortQuiet), "settings")
	}
}

// SetExclusions applies a new exclusion list (as stored) like
// SettingsChanged.
func (e *Engine) SetExclusions(excl []Exclusion) {
	e.mu.Lock()
	e.excl = slices.Clone(excl)
	e.republishLocked()
	e.mu.Unlock()
	e.scheduleFull(e.now().Add(shortQuiet), "exclusions")
}

// ContextsChanged reloads context names (a manual playlist name) into the
// published snapshot. Contexts are not part of the derived tables.
func (e *Engine) ContextsChanged(ctx context.Context) error {
	in := newResolverInput()
	if err := loadContexts(ctx, e.db.Read, in); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.snap.Load()
	if s == nil {
		return nil
	}
	ents := s.Ent.clone()
	ents.Contexts = slices.Clone(ents.Contexts)
	for i, c := range ents.Contexts {
		ents.Contexts[i] = ents.makeContext(in, c.URI)
	}
	b := *s.Base
	b.Ent = ents
	e.publishLocked(&b, s.View)
	return nil
}

// republishLocked re-applies settings and exclusions to the current base.
func (e *Engine) republishLocked() {
	if s := e.snap.Load(); s != nil {
		e.publishLocked(s.Base, nil)
	}
}

// publishLocked publishes base with view, or a new view when view is nil
// or was built with other settings or exclusions.
func (e *Engine) publishLocked(b *Base, v *View) {
	if v == nil || !v.Settings.Equal(e.cur) || !slices.Equal(v.Exclusions, e.excl) {
		v = NewView(b, e.cur, e.excl)
	}
	e.snap.Store(&Snapshot{Base: b, View: v, BuiltAt: e.now()})
	select {
	case <-e.ready:
	default:
		close(e.ready)
	}
}

// Rebuild runs a full rebuild now and waits for it (tests, and the first
// start without Run).
func (e *Engine) Rebuild(ctx context.Context) error {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	return e.fullRebuild(ctx)
}

// Merge runs a pending incremental merge now (tests).
func (e *Engine) Merge(ctx context.Context) error {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	return e.incremental(ctx)
}

// Run loads or rebuilds the derived layer, then serves hooks until ctx
// ends.
func (e *Engine) Run(ctx context.Context) {
	e.runMu.Lock()
	err := e.start(ctx)
	e.runMu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Error("derive: startup failed; retrying with a full rebuild", "err", err)
		e.scheduleFull(e.now(), "startup")
	}
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		e.mu.Lock()
		fullAt, inc := e.fullAt, e.incPending
		e.mu.Unlock()
		now := e.now()
		switch {
		case !fullAt.IsZero() && !now.Before(fullAt):
			e.runMu.Lock()
			err := e.fullRebuild(ctx)
			e.runMu.Unlock()
			if err != nil && ctx.Err() == nil {
				slog.Error("derive: full rebuild failed", "err", err)
				e.scheduleFull(e.now().Add(time.Minute), "retry")
			}
			continue
		case inc:
			e.mu.Lock()
			e.incPending = false
			e.mu.Unlock()
			e.runMu.Lock()
			err := e.incremental(ctx)
			e.runMu.Unlock()
			if err != nil && ctx.Err() == nil {
				slog.Error("derive: incremental merge failed; scheduling a full rebuild", "err", err)
				e.scheduleFull(e.now().Add(shortQuiet), "incremental failed")
			}
			continue
		}
		var timer <-chan time.Time
		var t *time.Timer
		if !fullAt.IsZero() {
			t = time.NewTimer(fullAt.Sub(now))
			timer = t.C
		}
		select {
		case <-ctx.Done():
		case <-e.wake:
		case <-timer:
		case <-tick.C:
			e.checkRaw(ctx)
		}
		if t != nil {
			t.Stop()
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// watermarks are the newest raw rows merged so far.
type watermarks struct {
	exportID, pollerID, recentFetched int64
}

// String and parseMarks store watermarks in derive_state: the raw rows
// the plays table covers, so a restart merges only newer live rows.
func (w watermarks) String() string {
	return fmt.Sprintf("%d:%d:%d", w.exportID, w.pollerID, w.recentFetched)
}

func parseMarks(s string) (watermarks, bool) {
	var w watermarks
	n, _ := fmt.Sscanf(s, "%d:%d:%d", &w.exportID, &w.pollerID, &w.recentFetched)
	return w, n == 3
}

func readMarks(ctx context.Context, q *sql.DB) (watermarks, error) {
	var w watermarks
	err := q.QueryRowContext(ctx, `SELECT (SELECT COALESCE(MAX(id), 0) FROM raw_export_rows),
		(SELECT COALESCE(MAX(id), 0) FROM raw_poller_plays),
		(SELECT COALESCE(MAX(fetched_at), 0) FROM raw_recent_plays)`).Scan(&w.exportID, &w.pollerID, &w.recentFetched)
	return w, err
}

// checkRaw notices raw rows no hook announced, and a new played_at
// calibration verdict.
func (e *Engine) checkRaw(ctx context.Context) {
	w, err := readMarks(ctx, e.db.Read)
	if err != nil {
		return
	}
	pas := e.playedAtIsStart(ctx)
	e.mu.Lock()
	m, verdict := e.marks, e.verdict
	e.mu.Unlock()
	switch {
	case pas != verdict:
		e.scheduleFull(e.now(), "calibration")
	case w.exportID > m.exportID:
		e.RawChanged()
	case w.pollerID > m.pollerID || w.recentFetched > m.recentFetched:
		e.LiveChanged()
	}
}

// inputs reads settings, exclusions and the calibration verdict.
func (e *Engine) inputs(ctx context.Context) (Settings, []Exclusion, bool, error) {
	s, err := e.settings(ctx)
	if err != nil {
		return s, nil, false, err
	}
	excl, err := loadExclusions(ctx, e.db.Read)
	if err != nil {
		return s, nil, false, err
	}
	return s, excl, e.playedAtIsStart(ctx), nil
}

func (e *Engine) start(ctx context.Context) error {
	s, excl, pas, err := e.inputs(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.cur, e.excl, e.inited = s, excl, true
	e.mu.Unlock()
	fp, err := fingerprint(ctx, e.db.Read, s, excl, pas)
	if err != nil {
		return err
	}
	stored, err := getState(ctx, e.db.Read, "fingerprint")
	if err != nil {
		return err
	}
	if stored != fp {
		slog.Info("derive: inputs changed since the last rebuild; rebuilding")
		return e.fullRebuild(ctx)
	}
	start := time.Now()
	storedMarks, err := getState(ctx, e.db.Read, "marks")
	if err != nil {
		return err
	}
	marks, ok := parseMarks(storedMarks)
	if !ok {
		slog.Info("derive: no stored watermarks; rebuilding")
		return e.fullRebuild(ctx)
	}
	plays, err := loadPlays(ctx, e.db.Read)
	if err != nil {
		return err
	}
	span, err := exportSpan(ctx, e.db.Read)
	if err != nil {
		return err
	}
	in, err := loadResolverInput(ctx, e.db.Read, nil, nil)
	if err != nil {
		return err
	}
	ents := Resolve(in, trackIDs(plays))
	b := NewBase(plays, ents, in, true, span)
	at, _ := getState(ctx, e.db.Read, "last_rebuild_at")
	took, _ := getState(ctx, e.db.Read, "last_rebuild_ms")
	e.mu.Lock()
	e.marks, e.verdict = marks, pas
	e.status = Status{State: "idle"}
	fmt.Sscan(at, &e.status.LastRebuildAt)
	fmt.Sscan(took, &e.status.LastRebuildMs)
	e.publishLocked(b, nil)
	e.mu.Unlock()
	loadTook, n := time.Since(start).Round(time.Millisecond), len(plays)
	plays = nil
	slog.Info("derive: loaded derived plays", "plays", n, "took", loadTook, "heap_mb", releaseMemory())
	// Live rows stored after the last write are merged now.
	e.checkRaw(ctx)
	return nil
}

func trackIDs(plays []Play) []string {
	ids := make([]string, 0, len(plays)/4)
	seen := make(map[string]bool, len(plays)/4)
	for i := range plays {
		if id := plays[i].TrackID; !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func (e *Engine) setProgress(p float64) {
	e.mu.Lock()
	e.status.Progress = p
	e.mu.Unlock()
}

// fullRebuild re-derives everything from the raw layer.
func (e *Engine) fullRebuild(ctx context.Context) error {
	e.mu.Lock()
	reason := e.reason
	e.fullAt, e.reason = time.Time{}, ""
	e.incPending = false // a full rebuild merges every live row too
	e.status.State, e.status.Progress = "rebuilding", 0
	e.mu.Unlock()
	start := time.Now()
	err := e.rebuild(ctx)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastFull = e.now()
	e.status.State, e.status.Progress = "idle", 0
	if err != nil {
		e.status.LastError = err.Error()
		return err
	}
	e.status.LastError = ""
	e.status.LastRebuildAt, e.status.LastRebuildMs = e.now().UnixMilli(), time.Since(start).Milliseconds()
	took := time.Since(start).Round(time.Millisecond)
	slog.Info("derive: full rebuild done", "reason", reason, "plays", e.snap.Load().Len(), "took", took,
		"heap_mb", releaseMemory())
	return nil
}

func (e *Engine) rebuild(ctx context.Context) error {
	start := time.Now()
	s, excl, pas, err := e.inputs(ctx)
	if err != nil {
		return err
	}
	// The fingerprint and watermarks are taken before reading, so rows
	// arriving during the rebuild are picked up afterwards.
	fp, err := fingerprint(ctx, e.db.Read, s, excl, pas)
	if err != nil {
		return err
	}
	marks, err := readMarks(ctx, e.db.Read)
	if err != nil {
		return err
	}
	names := map[string]ExportNames{}
	in := MergeInput{PlayedAtIsStart: pas}
	if in.Export, err = readExport(ctx, e.db.Read, rawWindow{}, names); err != nil {
		return err
	}
	if in.Poller, err = readPoller(ctx, e.db.Read, rawWindow{}); err != nil {
		return err
	}
	if in.Recent, err = readRecent(ctx, e.db.Read, rawWindow{}); err != nil {
		return err
	}
	if in.ExportSpan, err = exportSpan(ctx, e.db.Read); err != nil {
		return err
	}
	e.setProgress(0.25)
	plays := Merge(in)
	in.Export, in.Poller, in.Recent = nil, nil, nil
	for i := range plays {
		plays[i].ID = int64(i + 1)
	}
	rin, err := loadResolverInput(ctx, e.db.Read, nil, names)
	if err != nil {
		return err
	}
	ents := Resolve(rin, trackIDs(plays))
	b := NewBase(plays, ents, rin, true, in.ExportSpan)
	v := NewView(b, s, excl)
	snap := &Snapshot{Base: b, View: v}
	e.setProgress(0.4)
	state := map[string]string{"fingerprint": fp, "marks": marks.String(),
		"last_rebuild_at": fmt.Sprint(e.now().UnixMilli()), "last_rebuild_ms": fmt.Sprint(time.Since(start).Milliseconds())}
	if err := writeFull(ctx, e.db, plays, snap, state); err != nil {
		return fmt.Errorf("write derived tables: %w", err)
	}
	e.mu.Lock()
	e.marks, e.verdict = marks, pas
	if !e.inited { // Rebuild called without Run (tests, tools)
		e.cur, e.excl, e.inited = s, excl, true
	}
	e.publishLocked(b, v)
	e.mu.Unlock()
	return nil
}

// incrementalMaxBack bounds how far back an incremental merge looks for a
// listening gap; gapMs is the gap that separates independent plays (well
// above every match window).
const (
	incrementalMaxBack = 24 * time.Hour
	gapMs              = 5 * 60_000
)

// incremental merges live rows newer than the watermarks.
func (e *Engine) incremental(ctx context.Context) error {
	cur := e.snap.Load()
	if cur == nil {
		return nil
	}
	e.mu.Lock()
	m := e.marks
	e.mu.Unlock()
	marks, err := readMarks(ctx, e.db.Read)
	if err != nil {
		return err
	}
	if marks.exportID > m.exportID {
		e.RawChanged()
		return nil
	}
	// The earliest time a new row can describe.
	var minT sql.NullInt64
	if err := e.db.Read.QueryRowContext(ctx, `SELECT MIN(t) FROM (
		SELECT MIN(started_ms) AS t FROM raw_poller_plays WHERE id > ?
		UNION ALL SELECT MIN(played_at_ms - COALESCE(duration_ms, 0)) FROM raw_recent_plays WHERE fetched_at > ?)`,
		m.pollerID, m.recentFetched).Scan(&minT); err != nil {
		return err
	}
	if !minT.Valid {
		e.mu.Lock()
		e.marks = marks
		e.mu.Unlock()
		return nil
	}
	start := time.Now()
	b := cur.Base
	// Re-merge from the last listening gap (≥ gapMs) before the new
	// rows: plays [k:] and everything new. The cut sits mid-gap, so every
	// raw row of play k-1 lies before it and every row of play k after it
	// (rows sit within a minute of their play), and no match crosses it.
	limit := minT.Int64 - incrementalMaxBack.Milliseconds()
	k := b.SearchStart(minT.Int64 - gapMs)
	next := func(k int) int64 { // when the listening after play k-1 starts
		if k < b.Len() {
			return min(b.Start[k], minT.Int64)
		}
		return minT.Int64
	}
	for k > 0 && next(k)-b.End(int32(k-1)) < gapMs {
		if b.Start[k-1] < limit {
			// No gap within a day: merge everything again.
			e.scheduleFull(e.now(), "no gap for an incremental merge")
			return nil
		}
		k--
	}
	cut := next(k) - gapMs/2
	w := rawWindow{fromMs: cut}
	e.mu.Lock()
	in := MergeInput{PlayedAtIsStart: e.verdict, ExportSpan: b.ExportSpan}
	e.mu.Unlock()
	if in.Export, err = readExport(ctx, e.db.Read, w, nil); err != nil {
		return err
	}
	if in.Poller, err = readPoller(ctx, e.db.Read, w); err != nil {
		return err
	}
	if in.Recent, err = readRecent(ctx, e.db.Read, w); err != nil {
		return err
	}
	window := Merge(in)
	window = slices.DeleteFunc(window, func(p Play) bool { return p.StartedMs < cut })

	// Keep the IDs of plays that survive (same track and start).
	type key struct {
		track string
		start int64
	}
	old := map[key]int64{}
	for i := k; i < b.Len(); i++ {
		old[key{b.Ent.Tracks[b.Track[i]].ID, b.Start[i]}] = b.ID[i]
	}
	maxID := b.MaxID
	for i := range window {
		if id, ok := old[key{window[i].TrackID, window[i].StartedMs}]; ok {
			window[i].ID = id
			delete(old, key{window[i].TrackID, window[i].StartedMs})
		} else {
			maxID++
			window[i].ID = maxID
		}
	}

	ids := trackIDs(window)
	var unknown idFilter
	for _, id := range ids {
		if _, ok := b.Ent.TrackByID[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	rin := newResolverInput()
	if len(unknown) > 0 {
		if rin, err = loadResolverInput(ctx, e.db.Read, unknown, nil); err != nil {
			return err
		}
	} else if err := loadContexts(ctx, e.db.Read, rin); err != nil {
		return err
	}
	ents := b.Ent.Extend(rin, ids)
	wb := NewBase(window, ents, rin, false, b.ExportSpan)
	nb := concatBase(b, k, wb)

	e.mu.Lock()
	v := NewView(nb, e.cur, e.excl)
	e.mu.Unlock()
	snap := &Snapshot{Base: nb, View: v}
	// Rewrite the session straddling the cut, and every later one.
	sessionFrom := len(v.Sessions)
	firstVis, _ := slices.BinarySearch(v.Vis, int32(k))
	if firstVis > 0 {
		firstVis--
	}
	for n, s := range v.Sessions {
		if s.To > firstVis {
			sessionFrom = n
			break
		}
	}
	if err := writeTail(ctx, e.db, cut, window, snap, k, sessionFrom, map[string]string{"marks": marks.String()}); err != nil {
		return fmt.Errorf("write derived tables: %w", err)
	}
	e.mu.Lock()
	e.marks = marks
	e.publishLocked(nb, v)
	e.mu.Unlock()
	slog.Debug("derive: incremental merge", "plays", len(window), "took", time.Since(start).Round(time.Microsecond))
	if ents.Stale {
		// A new real artist shares its name with a provisional one.
		e.scheduleFull(e.now().Add(shortQuiet), "unify")
	}
	return nil
}

// concatBase is b's plays before k followed by w's.
func concatBase(b *Base, k int, w *Base) *Base {
	n := k + w.Len()
	out := &Base{Ent: w.Ent, ExportSpan: b.ExportSpan, MaxID: max(b.MaxID, w.MaxID)}
	out.ID = append(append(make([]int64, 0, n), b.ID[:k]...), w.ID...)
	out.Start = append(append(make([]int64, 0, n), b.Start[:k]...), w.Start...)
	out.Wall = append(append(make([]int32, 0, n), b.Wall[:k]...), w.Wall...)
	out.Ms = append(append(make([]int32, 0, n), b.Ms[:k]...), w.Ms...)
	out.Track = append(append(make([]int32, 0, n), b.Track[:k]...), w.Track...)
	out.Ctx = append(append(make([]int32, 0, n), b.Ctx[:k]...), w.Ctx...)
	out.Device = append(append(make([]int16, 0, n), b.Device[:k]...), w.Device...)
	out.Platform = append(append(make([]int16, 0, n), b.Platform[:k]...), w.Platform...)
	out.Flags = append(append(make([]uint16, 0, n), b.Flags[:k]...), w.Flags...)
	return out
}

// releaseMemory returns a rebuild's scratch memory to the OS. A full
// rebuild peaks at several times the snapshot's size (raw rows, merge
// and resolver inputs, the table writes), and the runtime would keep
// that peak resident for a server that then idles (budget: < 60 MB).
// It returns the heap still in use, in MB, for the log.
func releaseMemory() int {
	debug.FreeOSMemory()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return int(ms.HeapInuse >> 20)
}

// ErrNotReady is returned before the first snapshot exists.
var ErrNotReady = errors.New("derived data is not ready")
