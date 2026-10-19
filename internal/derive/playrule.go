// Package derive turns raw source rows into plays (D3, D7, D8).
//
// M0 only contains the play rule, because the mock API (the future
// correctness oracle) and the M5 derivation engine must share exactly one
// implementation of it.
package derive

// RuleKind selects the play-counting rule (D7).
type RuleKind string

const (
	// RuleFixed: at least Rule.ThresholdMs was played (the default, at
	// 30 s: Spotify's own stream threshold).
	RuleFixed RuleKind = "fixed"
	// RuleHalfTrack: the track is longer than 30 s and at least
	// min(50% of its duration, 4 minutes) was played.
	RuleHalfTrack RuleKind = "half_track"
	// ruleHalfTrackLegacy is RuleHalfTrack's name before 2026-10-18,
	// still accepted from stored settings and API clients.
	ruleHalfTrackLegacy RuleKind = "lastfm"
)

// ParseRuleKind maps a stored or requested rule name to its kind,
// accepting the legacy name of the half-track rule.
func ParseRuleKind(s string) (RuleKind, bool) {
	switch RuleKind(s) {
	case RuleFixed:
		return RuleFixed, true
	case RuleHalfTrack, ruleHalfTrackLegacy:
		return RuleHalfTrack, true
	}
	return "", false
}

// Rule is a configured play rule.
type Rule struct {
	Kind        RuleKind
	ThresholdMs int64 // used by RuleFixed
}

// DefaultRule counts every stream of at least 30 s, like Spotify itself
// (and stats.fm and Your Spotify), so counts compare across tools.
var DefaultRule = Rule{Kind: RuleFixed, ThresholdMs: FallbackThresholdMs}

const (
	// FallbackThresholdMs applies when the track duration is unknown
	// (export rows before hydration, D7) and is the default fixed threshold.
	FallbackThresholdMs = 30_000
	halfTrackMinMs      = 30_000
	halfTrackCapMs      = 4 * 60_000
)

// Row is the information about one listening row that the rule needs.
type Row struct {
	MsPlayed int64
	// DurationMs is the track length; 0 means unknown (not yet hydrated).
	DurationMs int64
	// RecentOnly marks rows known only from the recently-played endpoint:
	// Spotify already counted them, so they always count (D7).
	RecentOnly bool
}

// CountsAsPlay reports whether a row counts as a play under rule r.
func (r Rule) CountsAsPlay(row Row) bool {
	if row.RecentOnly {
		return true
	}
	switch r.Kind {
	case RuleFixed:
		return row.MsPlayed >= r.ThresholdMs
	default: // RuleHalfTrack
		if row.DurationMs <= 0 {
			return row.MsPlayed >= FallbackThresholdMs
		}
		if row.DurationMs <= halfTrackMinMs {
			return false
		}
		return row.MsPlayed >= min(row.DurationMs/2, halfTrackCapMs)
	}
}

// UnknownDurationThresholdMs is the ms_played a row of unknown duration
// needs to count under r (export rows before hydration, D7), so queries
// can filter on the rule without reading rows one by one. It agrees with
// CountsAsPlay for DurationMs 0.
func (r Rule) UnknownDurationThresholdMs() int64 {
	if r.Kind == RuleFixed {
		return r.ThresholdMs
	}
	return FallbackThresholdMs
}
