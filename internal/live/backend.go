// Package live is the api.Backend of a real instance: the query layer
// (M6) over the derived layer (M5, internal/derive). Every dashboard
// endpoint reads the derivation engine's current snapshot; settings,
// exclusions, playlist names, imports and capture status read and write
// the database directly.
//
// Semantics follow the mock backend (internal/mock), the correctness
// oracle: the differential test in this package compares the two
// endpoint by endpoint on the reference dataset.
package live

import (
	"context"
	"math"
	"sync"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/capture"
	"spotussi/internal/db"
	"spotussi/internal/derive"
	"spotussi/internal/hydrate"
	"spotussi/internal/importer"
	"spotussi/internal/library"
	"spotussi/internal/spotify"
)

// Options configure a Backend.
type Options struct {
	DB *db.DB
	// DefaultTimezone is the display timezone until the owner picks one
	// (the host's zone; D20).
	DefaultTimezone string
	// Now defaults to time.Now.
	Now func() time.Time
	// Capture is the running capture service (nil when disabled).
	Capture Capture
	// LimiterStats reports the shared Spotify rate limiter (optional).
	LimiterStats func() spotify.LimiterStats
	// Imports runs uploaded exports (nil: uploads answer 501; jobs are
	// still listed from the database).
	Imports *importer.Service
	// Catalog reports hydration progress (nil: hydration is off and the
	// catalog is counted from the database).
	Catalog func(context.Context) (hydrate.Status, error)
	// Library is the library sync job (nil: sync is off; synced data is
	// still served).
	Library *library.Service
}

// Backend implements api.Backend for live mode.
type Backend struct {
	db        *db.DB
	defaultTZ string
	now       func() time.Time
	capture   Capture
	limiter   func() spotify.LimiterStats
	imports   *importer.Service
	engine    *derive.Engine

	catalog func(context.Context) (hydrate.Status, error)
	library *library.Service

	settingsMu sync.Mutex // serialises settings read-modify-write
	cacheMu    sync.Mutex
	cached     *api.Settings // settings as last read or written
}

var _ api.Backend = (*Backend)(nil)

// New builds a Backend and its derivation engine (started by Run).
func New(o Options) *Backend {
	b := &Backend{db: o.DB, defaultTZ: o.DefaultTimezone, now: o.Now, limiter: o.LimiterStats, imports: o.Imports,
		catalog: o.Catalog, library: o.Library}
	if o.Capture != nil {
		b.capture = o.Capture
	}
	if !api.ValidTimezone(b.defaultTZ) {
		b.defaultTZ = "UTC"
	}
	if b.now == nil {
		b.now = time.Now
	}
	b.engine = derive.New(derive.Options{DB: o.DB, Now: b.now, PlayedAtIsStart: b.playedAtIsStart,
		Settings: func(ctx context.Context) (derive.Settings, error) {
			s, err := b.loadSettings(ctx)
			return deriveSettings(s), err
		}})
	return b
}

// Engine is the derivation engine; main wires its hooks (capture,
// imports, catalog hydration) and runs it.
func (b *Backend) Engine() *derive.Engine { return b.engine }

// Run runs the derivation engine until ctx ends.
func (b *Backend) Run(ctx context.Context) { b.engine.Run(ctx) }

// playedAtIsStart reads the played_at calibration verdict (D8): from the
// running capture service, else from the stored samples.
func (b *Backend) playedAtIsStart(ctx context.Context) bool {
	if b.capture != nil {
		c := b.capture.Status().Calibration
		return c != nil && c.Verdict == capture.AnchorStart
	}
	c, err := capture.ReadCalibration(ctx, b.db)
	return err == nil && c.Verdict == capture.AnchorStart
}

func deriveSettings(s api.Settings) derive.Settings {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return derive.Settings{Rule: playRule(s), Loc: loc, SessionGapMinutes: s.SessionGapMinutes}
}

func playRule(s api.Settings) derive.Rule {
	if s.PlayRule == string(derive.RuleFixed) {
		return derive.Rule{Kind: derive.RuleFixed, ThresholdMs: s.PlayRuleThresholdMs}
	}
	return derive.Rule{Kind: derive.RuleHalfTrack, ThresholdMs: s.PlayRuleThresholdMs}
}

// SystemStatus reports what the live backend knows; the HTTP layer
// overlays the auth mode and Spotify connection.
func (b *Backend) SystemStatus(ctx context.Context) (*api.SystemStatus, error) {
	jobs, err := importer.List(ctx, b.db, 5)
	if err != nil {
		return nil, err
	}
	cat, err := b.catalogStatus(ctx)
	if err != nil {
		return nil, err
	}
	st := &api.SystemStatus{
		Spotify: api.SpotifyStatus{State: string(spotify.StateNotConfigured)}, // overlaid by the HTTP layer
		Capture: b.captureStatus(),
		Imports: wireJobs(jobs),
		Catalog: cat,
	}
	ls, err := b.libraryStatus(ctx)
	if err != nil {
		return nil, err
	}
	st.Library = WireLibraryStatus(ls)
	ds := b.engine.Status()
	st.Derive = api.DeriveStatus{State: ds.State}
	if ds.State == "rebuilding" {
		st.Derive.Progress = &ds.Progress
	}
	if ds.LastRebuildAt > 0 {
		st.Derive.LastRebuildAt, st.Derive.LastRebuildMs = &ds.LastRebuildAt, &ds.LastRebuildMs
	}
	// Data counts need the snapshot; while the first one loads they are
	// reported as zero rather than holding up the status page.
	ctx2, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if q, err := b.q(ctx2); err == nil {
		st.Data = q.dataStatus()
	}
	return st, nil
}

// catalogStatus converts hydration progress.
func (b *Backend) catalogStatus(ctx context.Context) (api.CatalogStatus, error) {
	var st hydrate.Status
	var err error
	if b.catalog != nil {
		st, err = b.catalog(ctx)
	} else {
		st, err = hydrate.ReadStatus(ctx, b.db)
	}
	if err != nil {
		return api.CatalogStatus{}, err
	}
	out := api.CatalogStatus{State: st.State, TracksTotal: st.TracksTotal, TracksHydrated: st.TracksHydrated,
		AlbumsTotal: st.AlbumsTotal, AlbumsHydrated: st.AlbumsHydrated, ArtistsTotal: st.ArtistsTotal,
		ArtistsHydrated: st.ArtistsHydrated, ContextsTotal: st.ContextsTotal, ContextsResolved: st.ContextsResolved,
		QueueLength: st.QueueLength(), QueueFailed: st.Failed,
		Queue: api.CatalogQueueCounts{Track: st.Queue["track"], Album: st.Queue["album"], Artist: st.Queue["artist"],
			Context: st.Queue["context"]},
		EtaAt: msPtr(st.ETA), ResumesAt: msPtr(st.ResumesAt), LastError: strPtr(st.LastError), LastErrorAt: msPtr(st.LastErrorAt),
		LastHydratedAt: msPtr(st.LastDoneAt)}
	if st.QuotaLookups > 0 {
		q := st.QuotaLookups
		out.QuotaLookups = &q
	}
	if st.RatePerSec > 0 {
		r := math.Round(st.RatePerSec*100) / 100
		out.RatePerSec = &r
	}
	out.Fallbacks = []api.CatalogFallback{}
	for _, f := range st.Fallbacks {
		cf := api.CatalogFallback{Source: f.Source, State: f.State, Matched: f.Matched, Unmatched: f.Unmatched,
			Pending: f.Pending, Failed: f.Failed, ArtistsMatched: f.ArtistsMatched, AlbumsMatched: f.AlbumsMatched,
			Requests: f.Requests, PausedUntil: msPtr(f.PausedUntil), LastError: strPtr(f.LastError),
			LastErrorAt: msPtr(f.LastErrorAt)}
		if f.RatePerSec > 0 {
			r := math.Round(f.RatePerSec*100) / 100
			cf.RatePerSec = &r
		}
		out.Fallbacks = append(out.Fallbacks, cf)
	}
	return out, nil
}
