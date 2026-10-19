package derive

import "testing"

func TestUnknownDurationThreshold(t *testing.T) {
	for _, r := range []Rule{DefaultRule, {Kind: RuleFixed, ThresholdMs: 45_000}, {Kind: RuleFixed, ThresholdMs: 1_000}} {
		th := r.UnknownDurationThresholdMs()
		if !r.CountsAsPlay(Row{MsPlayed: th}) || r.CountsAsPlay(Row{MsPlayed: th - 1}) {
			t.Errorf("%+v: threshold %d disagrees with CountsAsPlay", r, th)
		}
	}
}

func TestCountsAsPlay(t *testing.T) {
	fixed := Rule{Kind: RuleFixed, ThresholdMs: 30_000}
	half := Rule{Kind: RuleHalfTrack, ThresholdMs: 30_000}
	cases := []struct {
		name string
		rule Rule
		row  Row
		want bool
	}{
		{"half track half of short track", half, Row{MsPlayed: 60_000, DurationMs: 120_000}, true},
		{"half track just under half", half, Row{MsPlayed: 59_999, DurationMs: 120_000}, false},
		{"half track 4 min cap on long track", half, Row{MsPlayed: 240_000, DurationMs: 1_200_000}, true},
		{"half track under cap on long track", half, Row{MsPlayed: 239_999, DurationMs: 1_200_000}, false},
		{"half track track not longer than 30s", half, Row{MsPlayed: 30_000, DurationMs: 30_000}, false},
		{"half track track 31s played half", half, Row{MsPlayed: 15_500, DurationMs: 31_000}, true},
		{"half track unknown duration falls back to 30s", half, Row{MsPlayed: 30_000}, true},
		{"half track unknown duration under 30s", half, Row{MsPlayed: 29_999}, false},
		{"recent-only always counts", half, Row{MsPlayed: 0, DurationMs: 200_000, RecentOnly: true}, true},
		{"default is fixed 30s", DefaultRule, Row{MsPlayed: 30_000, DurationMs: 600_000}, true},
		{"default under 30s", DefaultRule, Row{MsPlayed: 29_999, DurationMs: 40_000}, false},
		{"fixed threshold met", fixed, Row{MsPlayed: 30_000, DurationMs: 600_000}, true},
		{"fixed threshold not met", fixed, Row{MsPlayed: 29_000, DurationMs: 40_000}, false},
		{"fixed ignores short track rule", fixed, Row{MsPlayed: 30_000, DurationMs: 30_000}, true},
	}
	for _, c := range cases {
		if got := c.rule.CountsAsPlay(c.row); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
