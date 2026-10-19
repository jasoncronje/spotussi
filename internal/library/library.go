// Package library syncs the connected account's library (D36, DF9):
// saved tracks, saved albums, the user's playlists and the items of the
// playlists the user owns or collaborates on.
//
// Spotify exposes only the current state, so every sync records what it
// saw (migration 0009): a library_syncs row per run, a library_walks row
// per list checked or walked, and library_spans, one per stretch of
// membership of an item in a list. Only a complete walk closes a span.
// Adds and removals over time are read from the spans.
//
// Budget: every request goes through the shared limiter's background
// lane, so live capture keeps its reserve, and none is a catalog lookup
// (/me/… is the user quota bucket, /playlists/… its own, D24). An hourly
// quick check reads the first page of saved tracks and albums and the
// playlist list; a list whose total and first page match what is stored
// is unchanged, otherwise it is walked. Playlists are walked only when
// their snapshot_id changed. Once a day every list is walked in full.
//
// Track and album objects seed the catalog (hydrate.StoreLibraryTrack),
// so library tracks have names and artwork without lookups.
package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/hydrate"
	"spotussi/internal/spotify"
)

// Client is the part of the Spotify client sync uses.
type Client interface {
	Status(ctx context.Context) (*spotify.Status, error)
	SavedTracks(ctx context.Context, offset int) (*spotify.Page[spotify.SavedTrack], error)
	SavedAlbums(ctx context.Context, offset int) (*spotify.Page[spotify.SavedAlbum], error)
	MyPlaylists(ctx context.Context, offset int) (*spotify.Page[spotify.PlaylistSummary], error)
	PlaylistItems(ctx context.Context, id string, offset int) (*spotify.Page[spotify.PlaylistItem], error)
}

// Options configure a Service. Zero durations take the defaults.
type Options struct {
	DB     *db.DB
	Client Client
	// Interval is the time between quick checks (default 1 h).
	Interval time.Duration
	// FullEvery is the time between full walks (default 24 h).
	FullEvery time.Duration
	// FirstDelay is the wait before the first sync after start or after
	// (re)connecting (default 2 min).
	FirstDelay time.Duration
	// RetryDelay is the wait after a failed sync (default 10 min, at most
	// Interval).
	RetryDelay time.Duration
	Now        func() time.Time
	// OnCatalogChanged is called after a sync that seeded the catalog or
	// changed the library (the derivation rebuild cue, D25).
	OnCatalogChanged func()
}

// Lists.
const (
	ListTracks    = "tracks"
	ListAlbums    = "albums"
	ListPlaylists = "playlists"
)

// Service states.
const (
	StateDisabled      = "disabled"
	StateNotConnected  = "not_connected"
	StateIdle          = "idle"
	StateSyncing       = "syncing"
	StateBackingOff    = "backing_off"
	StateQuotaExceeded = "quota_exceeded"
)

// List states.
const (
	ListOK         = "ok"
	ListPending    = "pending" // not synced yet
	ListNeedsScope = "needs_scope"
	ListError      = "error"
)

// Sync kinds.
const (
	KindQuick  = "quick"
	KindFull   = "full"
	KindManual = "manual"
)

// Service runs library sync.
type Service struct {
	db  *db.DB
	sp  Client
	now func() time.Time

	interval, fullEvery, firstDelay, retryDelay time.Duration
	onChanged                                   func()

	trigger chan string // a sync kind to run now
	reset   chan struct{}

	runMu sync.Mutex // one sync at a time

	mu        sync.Mutex
	state     string
	nextAt    time.Time
	resumesAt time.Time
	progress  float64
	lastErr   string
	lastErrAt time.Time
	lists     map[string]listState
	// scopeDenied is set by a 403 on a library endpoint (scope revoked
	// in Spotify) until the next connect.
	scopeDenied bool
}

type listState struct {
	state string
	err   string
}

// New builds a Service (started by Run).
func New(o Options) *Service {
	s := &Service{db: o.DB, sp: o.Client, now: o.Now, interval: o.Interval, fullEvery: o.FullEvery,
		firstDelay: o.FirstDelay, retryDelay: o.RetryDelay, onChanged: o.OnCatalogChanged,
		trigger: make(chan string, 1), reset: make(chan struct{}, 1), state: StateIdle, lists: map[string]listState{}}
	if s.now == nil {
		s.now = time.Now
	}
	if s.interval <= 0 {
		s.interval = time.Hour
	}
	if s.fullEvery <= 0 {
		s.fullEvery = 24 * time.Hour
	}
	if s.firstDelay <= 0 {
		s.firstDelay = 2 * time.Minute
	}
	if s.retryDelay <= 0 {
		s.retryDelay = 10 * time.Minute
	}
	s.retryDelay = min(s.retryDelay, s.interval)
	return s
}

// SyncNow asks for a quick check now (owner-triggered). It reports false
// while a sync is already running or queued.
func (s *Service) SyncNow() bool {
	s.mu.Lock()
	busy := s.state == StateSyncing
	s.mu.Unlock()
	if busy {
		return false
	}
	select {
	case s.trigger <- KindManual:
		return true
	default:
		return false
	}
}

// Connected is called after Spotify was (re)connected: the scope check
// is reset and a sync is scheduled after FirstDelay.
func (s *Service) Connected() {
	s.mu.Lock()
	s.scopeDenied = false
	s.mu.Unlock()
	select {
	case s.reset <- struct{}{}:
	default:
	}
}

// Run syncs on schedule until ctx ends.
func (s *Service) Run(ctx context.Context) {
	wait := s.firstDelay
	for {
		s.setNext(s.now().Add(wait))
		t := time.NewTimer(wait)
		kind := ""
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-s.reset:
			t.Stop()
			wait = s.firstDelay
			continue
		case kind = <-s.trigger:
			t.Stop()
		case <-t.C:
		}
		if kind == "" {
			kind = s.dueKind(ctx)
		}
		wait = s.runOnce(ctx, kind)
	}
}

func (s *Service) setNext(t time.Time) {
	s.mu.Lock()
	s.nextAt = t
	s.mu.Unlock()
}

// dueKind is full when the last complete full walk is older than
// FullEvery.
func (s *Service) dueKind(ctx context.Context) string {
	var last sql.NullInt64
	_ = s.db.Read.QueryRowContext(ctx, `SELECT MAX(started_at) FROM library_syncs WHERE kind = 'full'
		AND finished_at IS NOT NULL AND error IS NULL`).Scan(&last)
	if !last.Valid || s.now().Sub(time.UnixMilli(last.Int64)) >= s.fullEvery-time.Minute {
		return KindFull
	}
	return KindQuick
}

// runOnce runs a sync and returns the wait until the next one.
func (s *Service) runOnce(ctx context.Context, kind string) time.Duration {
	err := s.Sync(ctx, kind)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case err == nil:
		return s.interval
	case s.state == StateNotConnected:
		return s.interval
	case s.state == StateQuotaExceeded && !s.resumesAt.IsZero():
		return max(time.Minute, s.resumesAt.Sub(s.now()))
	default:
		return s.retryDelay
	}
}

// errSkipped marks a list that was not synced (no scope).
var errSkipped = errors.New("skipped")

// Sync runs one sync of every list now (KindQuick, KindFull or
// KindManual) and returns the first error that stopped a list.
func (s *Service) Sync(ctx context.Context, kind string) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	prio := spotify.PriorityBackground
	if kind == KindManual {
		prio = spotify.PriorityInteractive
	}
	ctx = spotify.WithPriority(ctx, prio)

	st, err := s.sp.Status(ctx)
	if err != nil {
		return s.fail(err)
	}
	if st.State != spotify.StateConnected {
		s.mu.Lock()
		s.state, s.progress = StateNotConnected, 0
		s.mu.Unlock()
		return spotify.ErrNotConnected
	}
	s.mu.Lock()
	s.state, s.progress = StateSyncing, 0
	scope := slices.Contains(st.Scopes, spotify.ScopeLibrary) && !s.scopeDenied
	s.mu.Unlock()

	r := &run{s: s, kind: kind, account: st.AccountID, full: kind == KindFull}
	r.estimate = s.estimate(ctx)
	if r.sync, err = s.begin(ctx, kind); err != nil {
		return s.fail(err)
	}
	var first error
	note := func(list string, err error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case errors.Is(err, errSkipped):
			s.lists[list] = listState{state: ListNeedsScope}
		case err != nil:
			s.lists[list] = listState{state: ListError, err: err.Error()}
		default:
			s.lists[list] = listState{state: ListOK}
		}
		if err != nil && !errors.Is(err, errSkipped) && first == nil {
			first = err
		}
	}
	for _, l := range []string{ListTracks, ListAlbums} {
		if !scope {
			note(l, errSkipped)
			continue
		}
		err := r.syncList(ctx, l)
		if spotify.StatusOf(err) == http.StatusForbidden { // scope revoked in Spotify
			s.mu.Lock()
			s.scopeDenied = true
			s.mu.Unlock()
			err = errSkipped
		}
		note(l, err)
		if stop(err) {
			break
		}
	}
	if !stop(first) {
		note(ListPlaylists, r.syncPlaylists(ctx))
	}
	s.finish(ctx, r, first)
	if r.changed > 0 && s.onChanged != nil {
		s.onChanged()
	}
	if first != nil {
		return s.fail(first)
	}
	s.mu.Lock()
	s.state, s.progress = StateIdle, 0
	s.mu.Unlock()
	return nil
}

// stop reports errors that end the whole sync (not connected, revoked).
func stop(err error) bool {
	return errors.Is(err, spotify.ErrNotConnected) || errors.Is(err, spotify.ErrReconnectRequired) ||
		errors.Is(err, context.Canceled)
}

func (s *Service) fail(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress = 0
	var rl *spotify.RateLimitedError
	switch {
	case errors.Is(err, spotify.ErrNotConnected) || errors.Is(err, spotify.ErrReconnectRequired):
		s.state = StateNotConnected
	case errors.As(err, &rl) && rl.QuotaExceeded:
		s.state = StateQuotaExceeded
		s.resumesAt = s.now().Add(rl.Backoff)
	default:
		s.state = StateBackingOff
	}
	if !errors.Is(err, context.Canceled) {
		s.lastErr, s.lastErrAt = err.Error(), s.now()
		slog.Warn("library: sync failed", "err", err)
	}
	return err
}

func (s *Service) begin(ctx context.Context, kind string) (int64, error) {
	res, err := s.db.Write.ExecContext(ctx, `INSERT INTO library_syncs (kind, started_at) VALUES (?, ?)`, kind, s.now().UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Service) finish(ctx context.Context, r *run, err error) {
	var msg any
	if err != nil {
		msg = err.Error()
	}
	// The sync's own context may be cancelled; record it anyway.
	ctx = context.WithoutCancel(ctx)
	if _, e := s.db.Write.ExecContext(ctx, `UPDATE library_syncs SET finished_at = ?, requests = ?, error = ? WHERE id = ?`,
		s.now().UnixMilli(), r.requests, msg, r.sync); e != nil {
		slog.Warn("library: record sync", "err", e)
	}
}

// estimate is the number of requests a full walk would make, from the
// stored list sizes (for progress).
func (s *Service) estimate(ctx context.Context) int {
	pages := func(n int) int { return max(1, (n+spotify.LibraryPageSize-1)/spotify.LibraryPageSize) }
	var tracks, albums, playlists, items int
	_ = s.db.Read.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM library_spans WHERE list = 'tracks' AND gone_sync IS NULL),
		(SELECT COUNT(*) FROM library_spans WHERE list = 'albums' AND gone_sync IS NULL),
		(SELECT COUNT(*) FROM library_playlists WHERE gone_sync IS NULL),
		(SELECT COALESCE(SUM((COALESCE(items_total, 0) + 49) / 50), 0) FROM library_playlists
			WHERE gone_sync IS NULL AND relation <> 'followed')`).Scan(&tracks, &albums, &playlists, &items)
	return pages(tracks) + pages(albums) + pages(playlists) + items
}

// run is one sync in progress.
type run struct {
	s        *Service
	kind     string
	account  string
	full     bool
	sync     int64
	requests int
	estimate int
	changed  int
}

func (r *run) request() {
	r.requests++
	r.s.mu.Lock()
	r.s.progress = min(0.99, float64(r.requests)/float64(max(1, r.estimate)))
	r.s.mu.Unlock()
}

// obs is an item seen in a list.
type obs struct {
	uri     string
	addedAt int64
	addedBy string
	pos     int
	raw     json.RawMessage
	track   *spotify.Track
	album   *spotify.SimpleAlbum
}

func (o *obs) key() string { return o.uri + "\x00" + itoa(o.addedAt) }

// A fetcher reads the page at offset: Spotify's total, how many items
// the page held (episodes and empty items included) and the items kept.
type fetcher func(ctx context.Context, offset int) (total, n int, items []obs, err error)

func (r *run) fetcher(list string) fetcher {
	sp := r.s.sp
	switch list {
	case ListTracks:
		return func(ctx context.Context, off int) (int, int, []obs, error) {
			p, err := sp.SavedTracks(ctx, off)
			if err != nil {
				return 0, 0, nil, err
			}
			out := make([]obs, 0, len(p.Items))
			for i := range p.Items {
				it := &p.Items[i]
				if it.Track.ID == "" && it.Track.URI == "" {
					continue
				}
				out = append(out, obs{uri: trackURI(&it.Track), addedAt: spotify.ParseAddedAt(it.AddedAt),
					pos: off + i, raw: p.Raw[i], track: &it.Track})
			}
			return p.Total, len(p.Raw), out, nil
		}
	case ListAlbums:
		return func(ctx context.Context, off int) (int, int, []obs, error) {
			p, err := sp.SavedAlbums(ctx, off)
			if err != nil {
				return 0, 0, nil, err
			}
			out := make([]obs, 0, len(p.Items))
			for i := range p.Items {
				it := &p.Items[i]
				if it.Album.ID == "" {
					continue
				}
				out = append(out, obs{uri: "spotify:album:" + it.Album.ID, addedAt: spotify.ParseAddedAt(it.AddedAt),
					pos: off + i, raw: p.Raw[i], album: &it.Album})
			}
			return p.Total, len(p.Raw), out, nil
		}
	}
	id := list
	return func(ctx context.Context, off int) (int, int, []obs, error) {
		p, err := sp.PlaylistItems(ctx, id, off)
		if err != nil {
			return 0, 0, nil, err
		}
		out := make([]obs, 0, len(p.Items))
		for i := range p.Items {
			it := &p.Items[i]
			t := it.Object()
			if t == nil { // episodes (D11) and unavailable items
				continue
			}
			out = append(out, obs{uri: trackURI(t), addedAt: spotify.ParseAddedAt(it.AddedAt), addedBy: it.AddedByID(),
				pos: off + i, raw: p.Raw[i], track: t})
		}
		return p.Total, len(p.Raw), out, nil
	}
}

func trackURI(t *spotify.Track) string {
	if t.ID != "" && !t.IsLocal {
		return "spotify:track:" + t.ID
	}
	return t.URI
}

// syncList checks (quick) or walks (full, or when the check found a
// change) saved tracks or albums.
func (r *run) syncList(ctx context.Context, list string) error {
	fetch := r.fetcher(list)
	r.request()
	total, n, first, err := fetch(ctx, 0)
	if err != nil {
		return err
	}
	if !r.full {
		same, err := r.s.unchanged(ctx, list, total, first)
		if err != nil {
			return err
		}
		if same {
			return r.s.recordWalk(ctx, r.sync, list, "check", true, total, 0)
		}
	}
	return r.walk(ctx, list, fetch, total, n, first)
}

// walk reads every page of a list (the first already read) and applies
// it. A list whose total moved during the walk is recorded incomplete
// and closes nothing.
func (r *run) walk(ctx context.Context, list string, fetch fetcher, total, read int, items []obs) error {
	complete := read > 0 || total == 0
	for off := read; complete && off < total; {
		r.request()
		t, n, page, err := fetch(ctx, off)
		if err != nil {
			return err
		}
		if t != total || n == 0 {
			complete = false
			break
		}
		items = append(items, page...)
		off += n
	}
	n, err := r.s.apply(ctx, r.sync, list, items, complete)
	if err != nil {
		return err
	}
	r.changed += n
	return r.s.recordWalk(ctx, r.sync, list, "walk", complete, total, n)
}

// unchanged reports whether a list's total and first page match the
// stored present spans (the newest len(first) by added_at, compared as
// multisets).
func (s *Service) unchanged(ctx context.Context, list string, total int, first []obs) (bool, error) {
	var n int
	if err := s.db.Read.QueryRowContext(ctx, `SELECT COUNT(*) FROM library_spans WHERE list = ? AND gone_sync IS NULL`, list).
		Scan(&n); err != nil {
		return false, err
	}
	if n != total {
		return false, nil
	}
	if len(first) == 0 {
		return total == 0, nil
	}
	rows, err := s.db.Read.QueryContext(ctx, `SELECT item_uri, COALESCE(added_at, 0) FROM library_spans
		WHERE list = ? AND gone_sync IS NULL ORDER BY added_at DESC LIMIT ?`, list, len(first))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	want := map[string]int{}
	for i := range first {
		want[first[i].key()]++
	}
	for rows.Next() {
		var o obs
		if err := rows.Scan(&o.uri, &o.addedAt); err != nil {
			return false, err
		}
		k := o.key()
		if want[k] == 0 {
			return false, nil
		}
		want[k]--
	}
	return true, rows.Err()
}

func (s *Service) recordWalk(ctx context.Context, sync int64, list, mode string, complete bool, total, changed int) error {
	_, err := s.db.Write.ExecContext(ctx, `INSERT INTO library_walks (sync_id, list, mode, complete, total, changed)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (sync_id, list) DO UPDATE SET mode = excluded.mode,
		complete = excluded.complete, total = excluded.total, changed = excluded.changed`,
		sync, list, mode, complete, total, changed)
	return err
}

func (s *Service) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// apply diffs a list's items against its present spans: matching spans
// are seen again, new items open spans (and seed the catalog), and, when
// the walk is complete, spans without an item are closed. It returns the
// number of spans opened or closed.
func (s *Service) apply(ctx context.Context, sync int64, list string, items []obs, complete bool) (int, error) {
	changed := 0
	err := s.tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id, item_uri, COALESCE(added_at, 0) FROM library_spans
			WHERE list = ? AND gone_sync IS NULL ORDER BY id`, list)
		if err != nil {
			return err
		}
		present := map[string][]int64{}
		for rows.Next() {
			var id int64
			var o obs
			if err := rows.Scan(&id, &o.uri, &o.addedAt); err != nil {
				rows.Close()
				return err
			}
			present[o.key()] = append(present[o.key()], id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		seen, err := tx.PrepareContext(ctx, `UPDATE library_spans SET last_seen_sync = ?, position = ? WHERE id = ?`)
		if err != nil {
			return err
		}
		defer seen.Close()
		add, err := tx.PrepareContext(ctx, `INSERT INTO library_spans (list, item_uri, added_at, added_by, position,
			first_seen_sync, last_seen_sync, json) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer add.Close()
		now := s.now().UnixMilli()
		for i := range items {
			o := &items[i]
			k := o.key()
			if ids := present[k]; len(ids) > 0 {
				present[k] = ids[1:]
				if complete {
					if _, err := seen.ExecContext(ctx, sync, o.pos, ids[0]); err != nil {
						return err
					}
				}
				continue
			}
			var addedAt, addedBy any
			if o.addedAt > 0 {
				addedAt = o.addedAt
			}
			if o.addedBy != "" {
				addedBy = o.addedBy
			}
			if _, err := add.ExecContext(ctx, list, o.uri, addedAt, addedBy, o.pos, sync, sync, string(o.raw)); err != nil {
				return err
			}
			changed++
			switch {
			case o.track != nil:
				err = hydrate.StoreLibraryTrack(ctx, tx, o.track, now)
			case o.album != nil:
				err = hydrate.StoreLibraryAlbum(ctx, tx, o.album, now)
			}
			if err != nil {
				return err
			}
		}
		if !complete {
			return nil
		}
		for _, ids := range present {
			for _, id := range ids {
				if _, err := tx.ExecContext(ctx, `UPDATE library_spans SET gone_sync = ? WHERE id = ?`, sync, id); err != nil {
					return err
				}
				changed++
			}
		}
		return nil
	})
	return changed, err
}

// syncPlaylists walks the playlist list, then the items of every own or
// collaborative playlist whose snapshot changed.
func (r *run) syncPlaylists(ctx context.Context) error {
	var all []spotify.PlaylistSummary
	var raws []json.RawMessage
	total, complete := 0, true
	for off := 0; ; off += spotify.LibraryPageSize {
		r.request()
		p, err := r.s.sp.MyPlaylists(ctx, off)
		if err != nil {
			return err
		}
		if off == 0 {
			total = p.Total
		} else if p.Total != total {
			complete = false
			break
		}
		all = append(all, p.Items...)
		raws = append(raws, p.Raw...)
		if len(p.Items) == 0 || off+spotify.LibraryPageSize >= total {
			break
		}
	}
	n, err := r.s.applyPlaylists(ctx, r.sync, r.account, all, raws, complete)
	if err != nil {
		return err
	}
	r.changed += n
	if err := r.s.recordWalk(ctx, r.sync, ListPlaylists, "walk", complete, total, n); err != nil {
		return err
	}
	// Items of playlists the user can read whose snapshot moved.
	rows, err := r.s.db.Read.QueryContext(ctx, `SELECT id, COALESCE(snapshot_id, '') FROM library_playlists
		WHERE gone_sync IS NULL AND relation <> 'followed' AND id NOT LIKE ? || '%'
			AND (items_snapshot_id IS NULL OR items_snapshot_id <> COALESCE(snapshot_id, ''))
		ORDER BY position`, hydrate.SpotifyOwnedPrefix)
	if err != nil {
		return err
	}
	type due struct{ id, snap string }
	var todo []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.snap); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var first error
	for _, d := range todo {
		err := r.syncPlaylistItems(ctx, d.id, d.snap)
		if err != nil && first == nil {
			first = err
		}
		var rl *spotify.RateLimitedError
		if stop(err) || errors.As(err, &rl) {
			break
		}
	}
	return first
}

func (r *run) syncPlaylistItems(ctx context.Context, id, snap string) error {
	fetch := r.fetcher(id)
	r.request()
	total, n, first, err := fetch(ctx, 0)
	if code := spotify.StatusOf(err); code == http.StatusForbidden || code == http.StatusNotFound {
		// Not ours to read after all (a collaborative playlist we don't
		// collaborate on, or deleted since the list was read).
		_, err := r.s.db.Write.ExecContext(ctx, `UPDATE library_playlists SET items_available = 0,
			items_snapshot_id = ? WHERE id = ?`, snap, id)
		return err
	}
	if err != nil {
		return err
	}
	if err := r.walk(ctx, id, fetch, total, n, first); err != nil {
		return err
	}
	var complete bool
	if err := r.s.db.Read.QueryRowContext(ctx, `SELECT complete FROM library_walks WHERE sync_id = ? AND list = ?`,
		r.sync, id).Scan(&complete); err != nil {
		return err
	}
	if !complete {
		return nil
	}
	_, err = r.s.db.Write.ExecContext(ctx, `UPDATE library_playlists SET items_available = 1, items_snapshot_id = ?,
		items_total = ? WHERE id = ?`, snap, total, id)
	return err
}

// applyPlaylists upserts the listed playlists and, when the list is
// complete, closes playlists no longer in it (and their items).
func (s *Service) applyPlaylists(ctx context.Context, sync int64, account string, ps []spotify.PlaylistSummary,
	raws []json.RawMessage, complete bool) (int, error) {
	changed := 0
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixMilli()
		seen := map[string]bool{}
		for i := range ps {
			p := &ps[i]
			if p.ID == "" || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			rel := "followed"
			switch {
			case p.Owner.ID != "" && p.Owner.ID == account:
				rel = "own"
			case p.Collaborative:
				rel = "collaborative"
			}
			var public, total any
			if p.Public != nil {
				public = *p.Public
			}
			if n := p.ItemsTotal(); n >= 0 {
				total = n
			}
			var image any
			if len(p.Images) > 0 && p.Images[0].URL != "" {
				image = p.Images[0].URL
			}
			// A playlist back after being gone is present again; its items
			// are walked afresh (their spans were closed with it).
			res, err := tx.ExecContext(ctx, `UPDATE library_playlists SET name = ?, owner_id = ?, owner_name = ?,
				relation = ?, collaborative = ?, public = ?, image_url = ?, snapshot_id = ?, items_total = COALESCE(?, items_total),
				position = ?, last_seen_sync = ?, json = ?,
				items_snapshot_id = CASE WHEN gone_sync IS NULL THEN items_snapshot_id END, gone_sync = NULL
				WHERE id = ?`,
				p.Name, nullStr(p.Owner.ID), nullStr(p.Owner.DisplayName), rel, p.Collaborative, public, image,
				nullStr(p.SnapshotID), total, i, sync, string(raws[i]), p.ID)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				if _, err := tx.ExecContext(ctx, `INSERT INTO library_playlists (id, name, owner_id, owner_name, relation,
					collaborative, public, image_url, snapshot_id, items_total, position, first_seen_sync, last_seen_sync, json)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					p.ID, p.Name, nullStr(p.Owner.ID), nullStr(p.Owner.DisplayName), rel, p.Collaborative, public, image,
					nullStr(p.SnapshotID), total, i, sync, sync, string(raws[i])); err != nil {
					return err
				}
				changed++
			}
			if err := hydrate.NamePlaylistContext(ctx, tx, p, now); err != nil {
				return err
			}
		}
		if !complete {
			return nil
		}
		rows, err := tx.QueryContext(ctx, `SELECT id FROM library_playlists WHERE gone_sync IS NULL`)
		if err != nil {
			return err
		}
		var gone []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			if !seen[id] {
				gone = append(gone, id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range gone {
			if _, err := tx.ExecContext(ctx, `UPDATE library_playlists SET gone_sync = ? WHERE id = ?`, sync, id); err != nil {
				return err
			}
			res, err := tx.ExecContext(ctx, `UPDATE library_spans SET gone_sync = ? WHERE list = ? AND gone_sync IS NULL`, sync, id)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			changed += 1 + int(n)
		}
		return nil
	})
	return changed, err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// IsSpotifyOwned reports whether a playlist ID is Spotify-made.
func IsSpotifyOwned(id string) bool { return strings.HasPrefix(id, hydrate.SpotifyOwnedPrefix) }
