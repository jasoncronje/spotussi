package derive

import (
	"cmp"
	"slices"
	"strings"
)

// Cross-source merge (D8). Three raw sources describe the same listening:
//
//   - the export (Spotify's own record): ts is the stream's end; it has the
//     real ms_played, reasons, shuffle, skipped, platform and country;
//   - the poller: measured start and end, ms_played, skipped, shuffle,
//     device and context;
//   - recently-played: proof that a play happened (Spotify counted it),
//     its context, and played_at, which marks the end unless the capture
//     calibration measured it to be the start.
//
// Merge is a pure function of its inputs, so rebuilding from the raw
// tables always gives the same plays.

// Source bits of Play.Sources (and plays.sources).
const (
	SrcExport uint8 = 1 << iota
	SrcPoller
	SrcRecent
)

const (
	// exportMatchMs bounds |export end - live end| when an export row
	// confirms a live play: min(30 s, half the track) (D8), so back-to-back
	// repeats of one track never merge.
	exportMatchMs = 30_000
	// recentWindowMs widens a poller play's [start, end] when matching a
	// recently-played item to it (as the capture calibration does).
	recentWindowMs = 60_000
	// recentMinMs: recently-played lists only plays Spotify counted
	// (30 s or more), so a shorter poller play has no item of its own.
	// Without this, a skip right after an unpolled play takes that
	// play's item. Poller listening time is measured between polls, so
	// a few seconds of slack.
	recentMinMs = 25_000
)

// ExportRow is a music row of raw_export_rows.
type ExportRow struct {
	ID          int64
	TsMs        int64 // end of the stream
	MsPlayed    int64
	TrackID     string // from the URI
	ReasonStart string
	ReasonEnd   string
	Platform    string
	Country     string
	Shuffle     Tri
	Skipped     Tri
	Offline     Tri
}

// PollerRow is a raw_poller_plays row.
type PollerRow struct {
	ID         int64
	TrackID    string
	DurationMs int64 // 0: unknown
	StartedMs  int64
	EndedMs    int64
	MsPlayed   int64
	Skipped    bool
	Shuffle    Tri
	DeviceName string
	DeviceType string
	ContextURI string
}

// RecentRow is a raw_recent_plays row.
type RecentRow struct {
	PlayedAtMs int64
	TrackID    string
	DurationMs int64 // 0: unknown
	ContextURI string
}

// Tri is a nullable boolean.
type Tri int8

const (
	Unknown Tri = iota
	False
	True
)

// TriOf converts a known boolean.
func TriOf(b bool) Tri {
	if b {
		return True
	}
	return False
}

// Play is one merged play, before entity resolution and settings.
type Play struct {
	ID          int64 // assigned by the caller (start order)
	TrackID     string
	StartedMs   int64
	EndedMs     int64
	MsPlayed    int64
	Estimated   bool // recently-played only: MsPlayed is the track length
	Skipped     bool
	Shuffle     Tri
	Platform    string
	DeviceName  string
	DeviceType  string
	ContextURI  string
	Country     string
	ReasonStart string
	ReasonEnd   string
	Offline     Tri
	Sources     uint8
	// Contradicted: a poller play inside the export's span that the export
	// does not confirm (D8; doubles as a poller correctness check).
	Contradicted bool

	ExportID       int64 // 0: none
	PollerID       int64
	RecentPlayedAt int64

	// matchMs is the live sources' track length, for the export match
	// tolerance only (track durations come from entity resolution).
	matchMs int64
}

// Has reports whether every bit of s is set.
func (p *Play) Has(s uint8) bool { return p.Sources&s == s }

// RecentOnly reports a play known only from recently-played.
func (p *Play) RecentOnly() bool { return p.Sources == SrcRecent }

// MergeInput is everything Merge reads.
type MergeInput struct {
	Export []ExportRow
	Poller []PollerRow
	Recent []RecentRow
	// PlayedAtIsStart: the capture calibration measured recently-played's
	// played_at to be the play's start (default: its end).
	PlayedAtIsStart bool
	// ExportSpan is [first start, last end] over every export row ever
	// imported (not just Export, which incremental merges window); zero
	// when there is no export.
	ExportSpan [2]int64
}

// Merge combines the raw rows into plays sorted by SortPlays, without IDs.
func Merge(in MergeInput) []Play {
	plays := make([]Play, 0, len(in.Poller)+len(in.Recent)+len(in.Export))
	for _, r := range in.Poller {
		plays = append(plays, Play{TrackID: r.TrackID, StartedMs: r.StartedMs, EndedMs: r.EndedMs, MsPlayed: r.MsPlayed,
			Skipped: r.Skipped, Shuffle: r.Shuffle, DeviceName: r.DeviceName, DeviceType: r.DeviceType,
			ContextURI: r.ContextURI, Sources: SrcPoller, PollerID: r.ID, matchMs: r.DurationMs})
	}

	// Recently-played items join the poller play of the same track whose
	// anchor (end, or start per calibration) is nearest to played_at
	// within its widened span; the rest become recent-only plays whose
	// length is estimated as the track's (D7). Pairs are taken nearest
	// first across all items, so when a track repeats, an item of the
	// previous (unpolled) play cannot take the next play's own item.
	byTrack := indexByTrack(plays)
	recent := slices.Clone(in.Recent)
	slices.SortFunc(recent, func(a, b RecentRow) int {
		return cmp.Or(cmp.Compare(a.PlayedAtMs, b.PlayedAtMs), strings.Compare(a.TrackID, b.TrackID))
	})
	type pair struct {
		r, p int
		d    int64
	}
	var pairs []pair
	for ri, r := range recent {
		for _, i := range byTrack[r.TrackID] {
			p := &plays[i]
			if p.MsPlayed < recentMinMs || r.PlayedAtMs < p.StartedMs-recentWindowMs || r.PlayedAtMs > p.EndedMs+recentWindowMs {
				continue
			}
			anchor := p.EndedMs
			if in.PlayedAtIsStart {
				anchor = p.StartedMs
			}
			pairs = append(pairs, pair{ri, i, abs(r.PlayedAtMs - anchor)})
		}
	}
	slices.SortFunc(pairs, func(a, b pair) int {
		return cmp.Or(cmp.Compare(a.d, b.d), cmp.Compare(a.r, b.r), cmp.Compare(a.p, b.p))
	})
	matched := make([]bool, len(recent))
	for _, pr := range pairs {
		p := &plays[pr.p]
		if matched[pr.r] || p.Sources&SrcRecent != 0 {
			continue
		}
		r := recent[pr.r]
		matched[pr.r] = true
		p.Sources |= SrcRecent
		p.RecentPlayedAt = r.PlayedAtMs
		if p.ContextURI == "" {
			p.ContextURI = r.ContextURI
		}
	}
	for ri, r := range recent {
		if matched[ri] {
			continue
		}
		p := Play{TrackID: r.TrackID, MsPlayed: r.DurationMs, Estimated: true, ContextURI: r.ContextURI,
			Sources: SrcRecent, RecentPlayedAt: r.PlayedAtMs, matchMs: r.DurationMs}
		p.StartedMs, p.EndedMs = r.PlayedAtMs-r.DurationMs, r.PlayedAtMs
		if in.PlayedAtIsStart {
			p.StartedMs, p.EndedMs = r.PlayedAtMs, r.PlayedAtMs+r.DurationMs
		}
		plays = append(plays, p)
		byTrack[r.TrackID] = append(byTrack[r.TrackID], len(plays)-1)
	}

	// Export rows confirm the live play of the same track whose end is
	// nearest theirs (within min(30 s, half the track)); the export's
	// fields win where it has them (D8). Unmatched rows are plays of their
	// own.
	for _, i := range byTrack {
		slices.SortFunc(i, func(a, b int) int { return cmp.Compare(plays[a].EndedMs, plays[b].EndedMs) })
	}
	// Pairs are taken nearest first across all rows, so a row cannot
	// take a neighbour's play just because it comes first.
	export := slices.Clone(in.Export)
	slices.SortFunc(export, func(a, b ExportRow) int { return cmp.Or(cmp.Compare(a.TsMs, b.TsMs), cmp.Compare(a.ID, b.ID)) })
	type epair struct {
		e, p int
		d    int64
	}
	var epairs []epair
	for ei, e := range export {
		live := byTrack[e.TrackID]
		k, _ := slices.BinarySearchFunc(live, e.TsMs-exportMatchMs, func(i int, t int64) int { return cmp.Compare(plays[i].EndedMs, t) })
		for ; k < len(live) && plays[live[k]].EndedMs <= e.TsMs+exportMatchMs; k++ {
			p := &plays[live[k]]
			tol := int64(exportMatchMs)
			if p.matchMs > 0 {
				tol = min(tol, p.matchMs/2)
			}
			if d := abs(p.EndedMs - e.TsMs); d <= tol {
				epairs = append(epairs, epair{ei, live[k], d})
			}
		}
	}
	slices.SortFunc(epairs, func(a, b epair) int {
		return cmp.Or(cmp.Compare(a.d, b.d), cmp.Compare(a.e, b.e), cmp.Compare(a.p, b.p))
	})
	target := make([]int, len(export))
	for i := range target {
		target[i] = -1
	}
	taken := make([]bool, len(plays))
	for _, pr := range epairs {
		if target[pr.e] < 0 && !taken[pr.p] {
			target[pr.e], taken[pr.p] = pr.p, true
		}
	}
	for ei, e := range export {
		best := target[ei]
		if best < 0 {
			plays = append(plays, Play{TrackID: e.TrackID})
			best = len(plays) - 1
		}
		p := &plays[best]
		if p.Sources&SrcPoller == 0 { // recently-played or export only: the export's times
			p.StartedMs, p.EndedMs = e.TsMs-e.MsPlayed, e.TsMs
		}
		p.Sources |= SrcExport
		p.ExportID = e.ID
		p.MsPlayed, p.Estimated = e.MsPlayed, false
		p.ReasonStart, p.ReasonEnd, p.Platform, p.Country, p.Offline = e.ReasonStart, e.ReasonEnd, e.Platform, e.Country, e.Offline
		if e.Shuffle != Unknown {
			p.Shuffle = e.Shuffle
		}
		switch {
		case e.Skipped != Unknown:
			p.Skipped = e.Skipped == True
		case p.Sources&SrcPoller == 0:
			// Older exports leave skipped null; a forward-button end is
			// a skip.
			p.Skipped = e.ReasonEnd == "fwdbtn"
		}
	}

	if in.ExportSpan[1] > 0 {
		for i := range plays {
			p := &plays[i]
			p.Contradicted = p.Sources&(SrcExport|SrcPoller) == SrcPoller &&
				p.StartedMs >= in.ExportSpan[0] && p.StartedMs <= in.ExportSpan[1]
		}
	}
	SortPlays(plays)
	return plays
}

// SortPlays orders plays by start, then track, end and source IDs, which
// is total for merged plays.
func SortPlays(plays []Play) {
	slices.SortFunc(plays, comparePlays)
}

func comparePlays(a, b Play) int {
	return cmp.Or(cmp.Compare(a.StartedMs, b.StartedMs), strings.Compare(a.TrackID, b.TrackID),
		cmp.Compare(a.EndedMs, b.EndedMs), cmp.Compare(a.PollerID, b.PollerID), cmp.Compare(a.ExportID, b.ExportID),
		cmp.Compare(a.RecentPlayedAt, b.RecentPlayedAt))
}

func indexByTrack(plays []Play) map[string][]int {
	m := make(map[string][]int, len(plays))
	for i := range plays {
		m[plays[i].TrackID] = append(m[plays[i].TrackID], i)
	}
	return m
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// TrackIDFromURI is the track key of a URI: the Spotify ID of
// spotify:track:ID, else the URI itself (local files), as the poller
// keys plays.
func TrackIDFromURI(uri string) string {
	if id, ok := strings.CutPrefix(uri, "spotify:track:"); ok {
		return id
	}
	return uri
}
