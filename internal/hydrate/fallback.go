package hydrate

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"spotussi/internal/derive"
	"spotussi/internal/metadata"
)

// Fallback metadata (D27): free public sources fill what Spotify's daily
// lookup quota leaves unhydrated, as lower-priority stages of the worker.
//
//   - oEmbed (exact, by URI): a queued track's title and album cover, a
//     queued artist's image.
//   - Deezer (name search, scored): a track's duration, ISRC and explicit
//     flag; its artist's picture; its album's release date, type, track
//     count and cover.
//   - MusicBrainz: the artist's genres, by the Deezer ISRC or else an
//     exact-name artist search.
//
// Each source walks the Spotify queue (hydration_queue) most-played
// first, with its own limiter, and records its outcome per entity in
// fallback_state, so it never repeats a lookup. The stages run while the
// Spotify worker isn't making progress (quota spent, not connected,
// backing off), so Spotify keeps the top priority while it has quota.
//
// Identities: a track is its Spotify ID (from the export URI); its row in
// tracks keeps hydrated_at null and source 'fallback', so it stays queued
// for Spotify. Artists and albums known only by export names use
// derivation's provisional IDs (D20) in fallback_artists, fallback_albums
// and artist_genres. Every written field gets a catalog_provenance row
// (source, confidence); a higher-confidence source's field is never
// overwritten by a lower one, and a Spotify write clears them all
// (clearFallback).

// Fallbacks are the fallback sources (a nil client is off).
type Fallbacks struct {
	OEmbed      *metadata.OEmbed
	Deezer      *metadata.Deezer
	MusicBrainz *metadata.MusicBrainz
	// BatchSize is how many queue entries a stage takes at a time
	// (default 24).
	BatchSize int
	// Always runs the stages even while Spotify is hydrating (tests and
	// measurement).
	Always bool
}

func (f Fallbacks) enabled(src string) bool {
	switch src {
	case metadata.SourceOEmbed:
		return f.OEmbed != nil
	case metadata.SourceDeezer:
		return f.Deezer != nil
	case metadata.SourceMusicBrainz:
		return f.MusicBrainz != nil
	}
	return false
}

// FallbackSources lists the sources in stage order.
var FallbackSources = []string{metadata.SourceOEmbed, metadata.SourceDeezer, metadata.SourceMusicBrainz}

// Fallback stage states reported in system status.
const (
	FallbackIdle       = "idle"        // nothing to do
	FallbackRunning    = "running"     // working through the queue
	FallbackPaused     = "paused"      // Spotify is hydrating: waiting its turn
	FallbackBackingOff = "backing_off" // the source is rate limiting or failing
	FallbackDisabled   = "disabled"    // turned off (METADATA_*=false)
)

// Confidence thresholds.
const (
	// minNameConf is the lowest confidence an artist or album name match
	// is accepted at.
	minNameConf = 0.7
	// fbMaxBackoff caps the retry delay of a failed fallback entry.
	fbMaxBackoff = 6 * time.Hour
)

// fbStats is one stage's runtime state.
type fbStats struct {
	state       string
	lastError   string
	lastErrorAt time.Time
	done        []time.Time
}

func (h *Hydrator) fbStat(src string) *fbStats {
	s := h.fb[src]
	if s == nil {
		s = &fbStats{state: FallbackIdle}
		h.fb[src] = s
	}
	return s
}

func (h *Hydrator) setFBState(src, state string) {
	h.mu.Lock()
	h.fbStat(src).state = state
	h.mu.Unlock()
}

func (h *Hydrator) fbFail(src string, err error) {
	h.mu.Lock()
	s := h.fbStat(src)
	s.lastError, s.lastErrorAt = err.Error(), h.now()
	h.mu.Unlock()
}

func (h *Hydrator) fbDone(src string) {
	now := h.now()
	h.mu.Lock()
	s := h.fbStat(src)
	s.done = append(s.done, now)
	cut := 0
	for cut < len(s.done) && now.Sub(s.done[cut]) > throughputWindow {
		cut++
	}
	s.done = s.done[cut:]
	h.mu.Unlock()
}

// fbClient is a source's client (nil: off).
func (h *Hydrator) fbClient(src string) interface {
	Requests() int64
	Limiter() *metadata.Limiter
} {
	f := h.o.Fallbacks
	switch {
	case src == metadata.SourceOEmbed && f.OEmbed != nil:
		return f.OEmbed
	case src == metadata.SourceDeezer && f.Deezer != nil:
		return f.Deezer
	case src == metadata.SourceMusicBrainz && f.MusicBrainz != nil:
		return f.MusicBrainz
	}
	return nil
}

// fallbackTurn reports whether the fallback stages may run: always when
// configured so, else whenever the Spotify worker isn't hydrating.
func (h *Hydrator) fallbackTurn() bool {
	if h.o.Fallbacks.Always || h.sp == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state != StateHydrating
}

// runFallback runs one source's stage until ctx is done.
func (h *Hydrator) runFallback(ctx context.Context, src string) {
	backoff := time.Minute
	for ctx.Err() == nil {
		var wait time.Duration
		if !h.fallbackTurn() {
			h.setFBState(src, FallbackPaused)
			wait = 30 * time.Second
		} else {
			h.setFBState(src, FallbackRunning)
			n, err := h.FallbackStep(ctx, src)
			switch {
			case ctx.Err() != nil:
				return
			case err != nil:
				slog.Warn("hydrate: fallback step failed", "source", src, "err", err)
				h.fbFail(src, err)
				h.setFBState(src, FallbackBackingOff)
				wait, backoff = backoff, min(backoff*2, 30*time.Minute)
			case n == 0:
				h.setFBState(src, FallbackIdle)
				h.maybeBorrow(ctx)
				h.announce(true)
				wait = 10 * time.Minute // or until new tracks are queued
			default:
				backoff = time.Minute
				h.maybeBorrow(ctx)
				h.announce(false)
				h.wakeFallbacks() // MusicBrainz waits for Deezer's ISRCs
				continue
			}
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
		case <-t.C:
		case <-h.fbWake[src]:
		}
		t.Stop()
	}
}

// wakeFallbacks re-checks every fallback stage now.
func (h *Hydrator) wakeFallbacks() {
	for _, ch := range h.fbWake {
		signal(ch)
	}
}

// fbItem is a queue entry a stage works on.
type fbItem struct {
	typ, id  string
	priority int64
	attempts int
}

// fbTypes are the queue entity types each source handles.
var fbTypes = map[string]string{
	metadata.SourceOEmbed:      `'track', 'artist'`,
	metadata.SourceDeezer:      `'track'`,
	metadata.SourceMusicBrainz: `'track'`,
}

// FallbackStep runs one batch of a source's stage and reports how many
// entries it settled (0: nothing to do).
func (h *Hydrator) FallbackStep(ctx context.Context, src string) (int, error) {
	if !h.o.Fallbacks.enabled(src) {
		return 0, nil
	}
	items, err := h.fbCandidates(ctx, src)
	if err != nil || len(items) == 0 {
		return 0, err
	}
	workers := 3
	if src == metadata.SourceMusicBrainz {
		workers = 1 // one request a second: more workers only queue up
	}
	work := make(chan fbItem)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	n := 0
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range work {
				err := h.fbOne(ctx, src, it)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				if err == nil {
					n++
				}
				mu.Unlock()
			}
		}()
	}
	for _, it := range items {
		if ctx.Err() != nil {
			break
		}
		work <- it
	}
	close(work)
	wg.Wait()
	if n == 0 && firstErr != nil {
		return 0, firstErr
	}
	return n, ctx.Err()
}

// fbCandidates lists the most-played queue entries the source has not
// settled. MusicBrainz waits for Deezer (its ISRC is the exact route) when
// Deezer is on.
func (h *Hydrator) fbCandidates(ctx context.Context, src string) ([]fbItem, error) {
	batch := h.o.Fallbacks.BatchSize
	if batch <= 0 {
		batch = 24
	}
	extra := ""
	if src == metadata.SourceMusicBrainz && h.o.Fallbacks.Deezer != nil {
		extra = `AND EXISTS (SELECT 1 FROM fallback_state d WHERE d.source = 'deezer' AND d.entity_type = q.entity_type
			AND d.id = q.id AND (d.status <> 'failed' OR d.attempts >= 3))`
	}
	rs, err := h.db.Read.QueryContext(ctx, `SELECT q.entity_type, q.id, q.priority, COALESCE(f.attempts, 0)
		FROM hydration_queue q
		LEFT JOIN fallback_state f ON f.source = ?1 AND f.entity_type = q.entity_type AND f.id = q.id
		WHERE q.entity_type IN (`+fbTypes[src]+`) AND (f.id IS NULL OR (f.status = 'failed' AND f.next_attempt_at <= ?2))
		`+extra+`
		ORDER BY q.priority DESC, q.entity_type DESC, q.id LIMIT ?3`, src, h.now().UnixMilli(), batch)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []fbItem
	for rs.Next() {
		var it fbItem
		if err := rs.Scan(&it.typ, &it.id, &it.priority, &it.attempts); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rs.Err()
}

// fbResult is the outcome of one entry.
type fbResult struct {
	matched    bool
	externalID string
	conf       float64
}

// fbOne processes one entry and records its state. Transient failures
// back off on the entry; only database errors are returned.
func (h *Hydrator) fbOne(ctx context.Context, src string, it fbItem) error {
	var res fbResult
	var err error
	switch src {
	case metadata.SourceOEmbed:
		res, err = h.fbOEmbed(ctx, it)
	case metadata.SourceDeezer:
		res, err = h.fbDeezer(ctx, it)
	case metadata.SourceMusicBrainz:
		res, err = h.fbMusicBrainz(ctx, it)
	}
	if ctx.Err() != nil {
		return nil // shutting down: tried again later
	}
	now := h.now()
	if err != nil {
		h.fbFail(src, err)
		var rl *metadata.RateLimitedError
		next := now.Add(min(time.Minute<<min(it.attempts, 20), fbMaxBackoff))
		if errors.As(err, &rl) {
			next = now.Add(max(rl.Backoff, time.Minute))
			h.setFBState(src, FallbackBackingOff)
		}
		msg := err.Error()
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return h.tx(ctx, func(tx *sql.Tx) error {
			return setFBState(ctx, tx, src, it.typ, it.id, "failed", "", 0, it.attempts+1, next.UnixMilli(), msg, now.UnixMilli())
		})
	}
	h.fbDone(src)
	status := "no_match"
	if res.matched {
		status = "matched"
	}
	return h.tx(ctx, func(tx *sql.Tx) error {
		return setFBState(ctx, tx, src, it.typ, it.id, status, res.externalID, res.conf, it.attempts, 0, "", now.UnixMilli())
	})
}

func setFBState(ctx context.Context, tx *sql.Tx, src, typ, id, status, ext string, conf float64, attempts int,
	next int64, lastErr string, now int64) error {
	var c any
	if status == "matched" {
		c = conf
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO fallback_state (source, entity_type, id, status, external_id, confidence,
			attempts, next_attempt_at, last_error, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source, entity_type, id) DO UPDATE SET status = excluded.status, external_id = excluded.external_id,
			confidence = excluded.confidence, attempts = excluded.attempts, next_attempt_at = excluded.next_attempt_at,
			last_error = excluded.last_error, updated_at = excluded.updated_at`,
		src, typ, id, status, nullStr(ext), c, attempts, next, nullStr(lastErr), now)
	return err
}

// fbFinal reports a settled (matched or no_match) state of an entity,
// for the shared artists and albums several tracks lead to.
func (h *Hydrator) fbFinal(ctx context.Context, src, typ, id string) (status, ext string, conf float64, ok bool, err error) {
	var e sql.NullString
	var c sql.NullFloat64
	err = h.db.Read.QueryRowContext(ctx, `SELECT status, external_id, confidence FROM fallback_state
		WHERE source = ? AND entity_type = ? AND id = ? AND status <> 'failed'`, src, typ, id).Scan(&status, &e, &c)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", 0, false, nil
	}
	return status, e.String, c.Float64, err == nil, err
}

// matcherKey stores the metadata.MatcherVersion Deezer's track states
// were settled with.
const matcherKey = "fallback_deezer_matcher_version"

// Rematch makes Deezer re-try the tracks it found no match for when the
// matcher has changed since (metadata.MatcherVersion): their no_match
// states are dropped, so they queue again. Matches and the artist and
// album states (found by name, not by the track matcher) stay. It
// reports how many tracks it re-queued.
func (h *Hydrator) Rematch(ctx context.Context) (int64, error) {
	v, err := readMark(ctx, h.db.Read, matcherKey)
	if err != nil || v >= metadata.MatcherVersion {
		return 0, err
	}
	var n int64
	err = h.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM fallback_state WHERE source = 'deezer' AND entity_type = 'track'
			AND status = 'no_match'`)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		return writeMark(ctx, tx, matcherKey, metadata.MatcherVersion, h.now().UnixMilli())
	})
	if err == nil && n > 0 {
		slog.Info("hydrate: Deezer matcher changed; re-trying its unmatched tracks", "tracks", n,
			"version", metadata.MatcherVersion)
	}
	return n, err
}

// ---------- provisional identities (D20, as derivation makes them) ----------

// ProvArtistID is the provisional ID derivation gives an artist known only
// by name (D20, derive.ProvArtistID).
func ProvArtistID(name string) string { return derive.ProvArtistID(name) }

// ProvAlbumID is the provisional ID of an album name by an artist.
func ProvAlbumID(artist, album string) string { return derive.ProvAlbumID(artist, album) }

// ---------- what the export says about a track ----------

type exportTrack struct {
	track, artist, album string
	dur                  metadata.Duration
}

// exportInfo reads a track's newest export names (the ones derivation
// uses) and what its streams say about its length: streams that ended
// with trackdone played it in full.
func (h *Hydrator) exportInfo(ctx context.Context, id string) (exportTrack, bool, error) {
	var e exportTrack
	uri := trackURIPrefix + id
	err := h.db.Read.QueryRowContext(ctx, `SELECT COALESCE(track_name, ''), COALESCE(artist_name, ''), COALESCE(album_name, '')
		FROM raw_export_rows WHERE uri = ? AND track_name IS NOT NULL ORDER BY ts_ms DESC, id DESC LIMIT 1`, uri).
		Scan(&e.track, &e.artist, &e.album)
	if errors.Is(err, sql.ErrNoRows) {
		return e, false, nil
	}
	if err != nil {
		return e, false, err
	}
	rs, err := h.db.Read.QueryContext(ctx, `SELECT ms_played, COALESCE(reason_end, '') FROM raw_export_rows WHERE uri = ?`, uri)
	if err != nil {
		return e, false, err
	}
	defer rs.Close()
	var full []int64
	for rs.Next() {
		var ms int64
		var reason string
		if err := rs.Scan(&ms, &reason); err != nil {
			return e, false, err
		}
		e.dur.MinMs = max(e.dur.MinMs, ms)
		if reason == "trackdone" && ms >= 30000 {
			full = append(full, ms)
		}
	}
	if len(full) > 0 {
		sort.Slice(full, func(i, j int) bool { return full[i] < full[j] })
		e.dur.ExpectedMs = full[len(full)/2]
	}
	return e, strings.TrimSpace(e.track) != "" && strings.TrimSpace(e.artist) != "", rs.Err()
}

// ---------- catalog writes with provenance ----------

// claim records that src writes field of an entity at confidence conf
// and reports whether it may: not when another source wrote it with at
// least that confidence.
func claim(ctx context.Context, tx *sql.Tx, typ, id, field, src string, conf float64, now int64) (bool, error) {
	var old string
	var oldConf float64
	err := tx.QueryRowContext(ctx, `SELECT source, confidence FROM catalog_provenance
		WHERE entity_type = ? AND entity_id = ? AND field = ?`, typ, id, field).Scan(&old, &oldConf)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return false, err
	case old != src && oldConf >= conf:
		return false, nil
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO catalog_provenance (entity_type, entity_id, field, source, confidence, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (entity_type, entity_id, field) DO UPDATE SET source = excluded.source,
			confidence = excluded.confidence, updated_at = excluded.updated_at`, typ, id, field, src, conf, now)
	return err == nil, err
}

// fbTrackRow makes sure a not-yet-hydrated track has a row and reports
// whether fallbacks may write it (false: Spotify hydrated it meanwhile).
func fbTrackRow(ctx context.Context, tx *sql.Tx, id, name string) (bool, error) {
	if name == "" {
		name = id
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tracks (id, name, source) VALUES (?, ?, 'fallback')
		ON CONFLICT (id) DO NOTHING`, id, name); err != nil {
		return false, err
	}
	var hydrated bool
	err := tx.QueryRowContext(ctx, `SELECT hydrated_at IS NOT NULL FROM tracks WHERE id = ?`, id).Scan(&hydrated)
	return !hydrated, err
}

// setTrackField writes one fallback field of a track (columns are
// constants of this file).
func setTrackField(ctx context.Context, tx *sql.Tx, id, field string, v any, src string, conf float64, now int64) error {
	ok, err := claim(ctx, tx, "track", id, field, src, conf, now)
	if err != nil || !ok {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE tracks SET `+field+` = ? WHERE id = ? AND hydrated_at IS NULL`, v, id)
	return err
}

func fbAlbumRow(ctx context.Context, tx *sql.Tx, id, artist, album string, now int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO fallback_albums (id, name, artist_name, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET updated_at = excluded.updated_at`, id, strings.Trim(album, " "), strings.Trim(artist, " "), now)
	return err
}

func fbArtistRow(ctx context.Context, tx *sql.Tx, id, name string, now int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO fallback_artists (id, name, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET updated_at = excluded.updated_at`, id, strings.Trim(name, " "), now)
	return err
}

// setImages writes an entity's image pair (one provenance field,
// image_url, covers both) into table.
func setImages(ctx context.Context, tx *sql.Tx, table, typ, id, large, small, src string, conf float64, now int64) error {
	if large == "" {
		return nil
	}
	if small == "" {
		small = large
	}
	ok, err := claim(ctx, tx, typ, id, "image_url", src, conf, now)
	if err != nil || !ok {
		return err
	}
	q := `UPDATE ` + table + ` SET image_url = ?, image_url_small = ? WHERE id = ?`
	if table == "artists" {
		q += ` AND hydrated_at IS NULL`
	}
	_, err = tx.ExecContext(ctx, q, large, small, id)
	return err
}

// clearFallback runs before a Spotify write of an entity: every field a
// fallback wrote is nulled (so Spotify's value, or its absence, wins
// cleanly) and its provenance dropped.
func clearFallback(ctx context.Context, tx *sql.Tx, typ, id string) error {
	rs, err := tx.QueryContext(ctx, `DELETE FROM catalog_provenance WHERE entity_type = ? AND entity_id = ? RETURNING field`, typ, id)
	if err != nil {
		return err
	}
	var fields []string
	for rs.Next() {
		var f string
		if err := rs.Scan(&f); err != nil {
			rs.Close()
			return err
		}
		fields = append(fields, f)
	}
	rs.Close()
	if err := rs.Err(); err != nil {
		return err
	}
	for _, f := range fields {
		var q string
		switch {
		case typ == "track" && (f == "duration_ms" || f == "isrc" || f == "explicit"):
			q = `UPDATE tracks SET ` + f + ` = NULL WHERE id = ?`
		case typ == "artist" && f == "image_url":
			q = `UPDATE artists SET image_url = NULL, image_url_small = NULL WHERE id = ?`
		default:
			continue // name: Spotify always writes it
		}
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

// ---------- oEmbed ----------

func (h *Hydrator) fbOEmbed(ctx context.Context, it fbItem) (fbResult, error) {
	o := h.o.Fallbacks.OEmbed
	r, err := o.Lookup(ctx, "spotify:"+it.typ+":"+it.id)
	if errors.Is(err, metadata.ErrNotFound) {
		return fbResult{}, nil
	}
	if err != nil {
		return fbResult{}, err
	}
	now := h.now().UnixMilli()
	large, small := "", ""
	if r.ThumbnailURL != "" {
		large, small = metadata.SpotifyImages(r.ThumbnailURL)
	}
	if it.typ == "artist" {
		err = h.tx(ctx, func(tx *sql.Tx) error {
			return setImages(ctx, tx, "artists", "artist", it.id, large, small, metadata.SourceOEmbed, 1, now)
		})
		h.changed(1)
		return fbResult{matched: true, conf: 1}, err
	}
	e, _, err := h.exportInfo(ctx, it.id)
	if err != nil {
		return fbResult{}, err
	}
	err = h.tx(ctx, func(tx *sql.Tx) error {
		ok, err := fbTrackRow(ctx, tx, it.id, firstNonEmpty(r.Title, e.track))
		if err != nil || !ok {
			return err
		}
		if r.Title != "" {
			if err := setTrackField(ctx, tx, it.id, "name", r.Title, metadata.SourceOEmbed, 1, now); err != nil {
				return err
			}
		}
		if large == "" || e.album == "" {
			return nil
		}
		al := ProvAlbumID(e.artist, e.album)
		if err := fbAlbumRow(ctx, tx, al, e.artist, e.album, now); err != nil {
			return err
		}
		return setImages(ctx, tx, "fallback_albums", "album", al, large, small, metadata.SourceOEmbed, 1, now)
	})
	h.changed(1)
	return fbResult{matched: true, conf: 1}, err
}

// ---------- Deezer ----------

func (h *Hydrator) fbDeezer(ctx context.Context, it fbItem) (fbResult, error) {
	dz := h.o.Fallbacks.Deezer
	e, ok, err := h.exportInfo(ctx, it.id)
	if err != nil || !ok {
		return fbResult{}, err
	}
	// oEmbed's title is Spotify's current one: a second chance for the
	// title score.
	var alt string
	_ = h.db.Read.QueryRowContext(ctx, `SELECT t.name FROM tracks t JOIN catalog_provenance p ON p.entity_type = 'track'
		AND p.entity_id = t.id AND p.field = 'name' AND p.source = 'oembed' WHERE t.id = ?`, it.id).Scan(&alt)
	q := metadata.TrackQuery{Title: e.track, Artist: e.artist, Album: e.album, AltTitle: alt, Duration: e.dur}
	title := metadata.StripQualifiers(e.track)
	queries := []string{e.artist + " track:" + metadata.Quote(title)}
	if e.album != "" {
		queries = append(queries, "track:"+metadata.Quote(title)+" album:"+metadata.Quote(metadata.StripQualifiers(e.album)))
	}
	queries = append(queries, e.artist+" "+title)
	var best metadata.DeezerTrack
	bestScore := 0.0
	for _, s := range queries {
		ts, err := dz.SearchTracks(ctx, s, 10)
		if errors.Is(err, metadata.ErrNotFound) {
			continue
		}
		if err != nil {
			return fbResult{}, err
		}
		for _, t := range ts {
			sc := metadata.TrackScore(q, t)
			// Ties (same recording on several releases): prefer the
			// export's album, then the more popular release.
			if sc > bestScore+1e-9 || (sc > 0 && sc >= bestScore-1e-9 && t.Rank > best.Rank && best.Album.Title != e.album) {
				best, bestScore = t, sc
			}
		}
		if bestScore >= 0.92 {
			break
		}
	}
	now := h.now().UnixMilli()
	var res fbResult
	if bestScore >= metadata.MinTrackScore {
		res = fbResult{matched: true, externalID: strconv.FormatInt(best.ID, 10), conf: round2(bestScore)}
		err := h.tx(ctx, func(tx *sql.Tx) error {
			ok, err := fbTrackRow(ctx, tx, it.id, firstNonEmpty(alt, e.track))
			if err != nil || !ok {
				return err
			}
			if best.Duration > 0 {
				if err := setTrackField(ctx, tx, it.id, "duration_ms", int64(best.Duration)*1000, metadata.SourceDeezer, res.conf, now); err != nil {
					return err
				}
			}
			if best.ISRC != "" {
				if err := setTrackField(ctx, tx, it.id, "isrc", best.ISRC, metadata.SourceDeezer, res.conf, now); err != nil {
					return err
				}
			}
			return setTrackField(ctx, tx, it.id, "explicit", best.Explicit, metadata.SourceDeezer, res.conf, now)
		})
		if err != nil {
			return fbResult{}, err
		}
		h.changed(1)
	}
	if err := h.fbDeezerArtist(ctx, e, best, res.conf); err != nil {
		return fbResult{}, err
	}
	if err := h.fbDeezerAlbum(ctx, e, best, res.conf); err != nil {
		return fbResult{}, err
	}
	return res, nil
}

// fbDeezerArtist finds the picture of a track's (provisional) artist:
// from the matched track's artist when the names agree, else by an
// artist search.
func (h *Hydrator) fbDeezerArtist(ctx context.Context, e exportTrack, t metadata.DeezerTrack, trackConf float64) error {
	id := ProvArtistID(e.artist)
	if _, _, _, done, err := h.fbFinal(ctx, metadata.SourceDeezer, "artist", id); err != nil || done {
		return err
	}
	var a metadata.DeezerArtist
	var conf float64
	if trackConf > 0 && metadata.NameSim(e.artist, t.Artist.Name) == 1 {
		a, conf = t.Artist, trackConf
	} else {
		as, err := h.o.Fallbacks.Deezer.SearchArtists(ctx, e.artist, 10)
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			return err
		}
		// Exact names only; homonyms are common, so the one with most
		// fans wins at a lower confidence.
		var exact []metadata.DeezerArtist
		for _, x := range as {
			if metadata.NameSim(e.artist, x.Name) == 1 {
				exact = append(exact, x)
			}
		}
		sort.SliceStable(exact, func(i, j int) bool { return exact[i].Fans > exact[j].Fans })
		switch {
		case len(exact) == 1:
			a, conf = exact[0], 0.85
		case len(exact) > 1 && exact[0].Fans >= 10*max(exact[1].Fans, 1):
			a, conf = exact[0], 0.75
		}
	}
	now := h.now().UnixMilli()
	return h.tx(ctx, func(tx *sql.Tx) error {
		if conf < minNameConf || a.ID == 0 {
			return setFBState(ctx, tx, metadata.SourceDeezer, "artist", id, "no_match", "", 0, 0, 0, "", now)
		}
		if err := fbArtistRow(ctx, tx, id, e.artist, now); err != nil {
			return err
		}
		ext := strconv.FormatInt(a.ID, 10)
		if _, err := tx.ExecContext(ctx, `UPDATE fallback_artists SET deezer_id = ? WHERE id = ?`, ext, id); err != nil {
			return err
		}
		if metadata.DeezerImage(a.PictureXL) {
			if err := setImages(ctx, tx, "fallback_artists", "artist", id, a.PictureXL, a.PictureMedium,
				metadata.SourceDeezer, conf, now); err != nil {
				return err
			}
			h.changed(1)
		}
		return setFBState(ctx, tx, metadata.SourceDeezer, "artist", id, "matched", ext, conf, 0, 0, "", now)
	})
}

// fbDeezerAlbum finds a track's (provisional) album: the matched track's
// album when the titles agree, else by an album search; then its release
// date, type, track count and cover from /album/{id}.
func (h *Hydrator) fbDeezerAlbum(ctx context.Context, e exportTrack, t metadata.DeezerTrack, trackConf float64) error {
	if e.album == "" {
		return nil
	}
	dz := h.o.Fallbacks.Deezer
	id := ProvAlbumID(e.artist, e.album)
	if _, _, _, done, err := h.fbFinal(ctx, metadata.SourceDeezer, "album", id); err != nil || done {
		return err
	}
	var albumID int64
	var conf float64
	if sim := metadata.TitleSim(e.album, t.Album.Title); trackConf > 0 && sim >= 0.9 {
		albumID, conf = t.Album.ID, min(trackConf, sim)
	} else {
		as, err := dz.SearchAlbums(ctx, e.artist+" "+metadata.StripQualifiers(e.album), 10)
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			return err
		}
		for _, a := range as {
			ts, as := metadata.TitleSim(e.album, a.Title), metadata.NameSim(e.artist, a.Artist.Name)
			if ts < 0.85 || as < 0.85 {
				continue
			}
			if c := round2(0.6*ts + 0.4*as); c > conf {
				albumID, conf = a.ID, c
			}
		}
	}
	var full *metadata.DeezerAlbum
	if albumID != 0 && conf >= minNameConf {
		a, err := dz.Album(ctx, albumID)
		switch {
		case errors.Is(err, metadata.ErrNotFound):
		case err != nil:
			return err
		default:
			full = a
		}
	}
	now := h.now().UnixMilli()
	return h.tx(ctx, func(tx *sql.Tx) error {
		if full == nil {
			return setFBState(ctx, tx, metadata.SourceDeezer, "album", id, "no_match", "", 0, 0, 0, "", now)
		}
		if err := fbAlbumRow(ctx, tx, id, e.artist, e.album, now); err != nil {
			return err
		}
		ext := strconv.FormatInt(full.ID, 10)
		if _, err := tx.ExecContext(ctx, `UPDATE fallback_albums SET deezer_id = ? WHERE id = ?`, ext, id); err != nil {
			return err
		}
		set := func(field string, v any) error {
			if v == nil {
				return nil
			}
			ok, err := claim(ctx, tx, "album", id, field, metadata.SourceDeezer, conf, now)
			if err != nil || !ok {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE fallback_albums SET `+field+` = ? WHERE id = ?`, v, id)
			return err
		}
		date, precision := deezerDate(full.ReleaseDate)
		if err := set("release_date", date); err != nil {
			return err
		}
		if err := set("release_date_precision", precision); err != nil {
			return err
		}
		if err := set("album_type", nullStr(metadata.SpotifyAlbumType(full.RecordType))); err != nil {
			return err
		}
		if err := set("total_tracks", positive(int64(full.Tracks))); err != nil {
			return err
		}
		if metadata.DeezerImage(full.CoverXL) {
			if err := setImages(ctx, tx, "fallback_albums", "album", id, full.CoverXL, full.CoverMedium,
				metadata.SourceDeezer, conf, now); err != nil {
				return err
			}
		}
		h.changed(1)
		return setFBState(ctx, tx, metadata.SourceDeezer, "album", id, "matched", ext, conf, 0, 0, "", now)
	})
}

// deezerDate validates a Deezer release date (YYYY-MM-DD; "0000-00-00"
// when unknown).
func deezerDate(s string) (date, precision any) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' || strings.HasPrefix(s, "0000") {
		return nil, nil
	}
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return nil, nil
	}
	return s, "day"
}

// ---------- MusicBrainz ----------

// maxGenres is how many MusicBrainz genres an artist keeps.
const maxGenres = 5

func (h *Hydrator) fbMusicBrainz(ctx context.Context, it fbItem) (fbResult, error) {
	mb := h.o.Fallbacks.MusicBrainz
	e, ok, err := h.exportInfo(ctx, it.id)
	if err != nil || !ok {
		return fbResult{}, err
	}
	artistID := ProvArtistID(e.artist)
	// Artists are shared by many tracks: look each up once.
	if status, ext, conf, done, err := h.fbFinal(ctx, metadata.SourceMusicBrainz, "artist", artistID); err != nil || done {
		return fbResult{matched: status == "matched", externalID: ext, conf: conf}, err
	}
	var mbid string
	var conf float64
	// Exact route: the ISRC (Deezer's, or Spotify's) to the recording's
	// credited artist of that name.
	var isrc sql.NullString
	var trackConf sql.NullFloat64
	if err := h.db.Read.QueryRowContext(ctx, `SELECT t.isrc, p.confidence FROM tracks t
		LEFT JOIN catalog_provenance p ON p.entity_type = 'track' AND p.entity_id = t.id AND p.field = 'isrc'
		WHERE t.id = ?`, it.id).Scan(&isrc, &trackConf); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fbResult{}, err
	}
	if isrc.String != "" {
		recs, err := mb.ISRC(ctx, isrc.String)
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			return fbResult{}, err
		}
		votes := map[string]int{}
		for _, r := range recs {
			for _, c := range r.Credits {
				if metadata.NameSim(e.artist, c.Artist.Name) >= 0.9 || metadata.NameSim(e.artist, c.Name) >= 0.9 {
					votes[c.Artist.ID]++
				}
			}
		}
		for id, n := range votes {
			if mbid == "" || n > votes[mbid] || (n == votes[mbid] && id < mbid) {
				mbid = id
			}
		}
		if mbid != "" {
			conf = 0.95
			if trackConf.Valid { // the ISRC came from a name match
				conf = round2(0.95 * trackConf.Float64)
			}
		}
	}
	if mbid == "" {
		as, err := mb.SearchArtists(ctx, e.artist, 5)
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			return fbResult{}, err
		}
		var exact []metadata.MBArtist
		for _, a := range as {
			if metadata.NameSim(e.artist, a.Name) == 1 {
				exact = append(exact, a)
			}
		}
		sort.SliceStable(exact, func(i, j int) bool { return exact[i].Score > exact[j].Score })
		switch {
		case len(exact) == 1 && exact[0].Score >= 90:
			mbid, conf = exact[0].ID, 0.85
		case len(exact) > 1 && exact[0].Score >= 90 && exact[0].Score-exact[1].Score >= 15:
			mbid, conf = exact[0].ID, 0.7
		}
	}
	now := h.now().UnixMilli()
	if mbid == "" || conf < minNameConf {
		return fbResult{}, h.tx(ctx, func(tx *sql.Tx) error {
			return setFBState(ctx, tx, metadata.SourceMusicBrainz, "artist", artistID, "no_match", "", 0, 0, 0, "", now)
		})
	}
	a, err := mb.Artist(ctx, mbid)
	if errors.Is(err, metadata.ErrNotFound) {
		return fbResult{}, h.tx(ctx, func(tx *sql.Tx) error {
			return setFBState(ctx, tx, metadata.SourceMusicBrainz, "artist", artistID, "no_match", "", 0, 0, 0, "", now)
		})
	}
	if err != nil {
		return fbResult{}, err
	}
	genres := metadata.TopGenres(a.Genres, maxGenres)
	err = h.tx(ctx, func(tx *sql.Tx) error {
		if err := fbArtistRow(ctx, tx, artistID, e.artist, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE fallback_artists SET mbid = ? WHERE id = ?`, mbid, artistID); err != nil {
			return err
		}
		if len(genres) > 0 {
			ok, err := claim(ctx, tx, "artist", artistID, "genres", metadata.SourceMusicBrainz, conf, now)
			if err != nil {
				return err
			}
			if ok {
				if _, err := tx.ExecContext(ctx, `DELETE FROM artist_genres WHERE artist_id = ?`, artistID); err != nil {
					return err
				}
				for _, g := range genres {
					if _, err := tx.ExecContext(ctx, `INSERT INTO artist_genres (artist_id, genre) VALUES (?, ?)
						ON CONFLICT DO NOTHING`, artistID, g); err != nil {
						return err
					}
				}
			}
		}
		return setFBState(ctx, tx, metadata.SourceMusicBrainz, "artist", artistID, "matched", mbid, conf, 0, 0, "", now)
	})
	if err != nil {
		return fbResult{}, err
	}
	h.changed(1)
	return fbResult{matched: true, externalID: mbid, conf: conf}, nil
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
