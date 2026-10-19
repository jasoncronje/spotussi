package api

import (
	"errors"
	"testing"
)

func TestPlayRuleSettings(t *testing.T) {
	s := DefaultSettings("UTC")
	if s.PlayRule != "fixed" || s.PlayRuleThresholdMs != 30_000 {
		t.Fatalf("default rule: %s %d", s.PlayRule, s.PlayRuleThresholdMs)
	}
	for in, want := range map[string]string{"half_track": "half_track", "lastfm": "half_track", "fixed": "fixed"} {
		next, err := s.Apply(SettingsPatch{PlayRule: &in})
		if err != nil || next.PlayRule != want {
			t.Errorf("play_rule %q: got %q, %v; want %q", in, next.PlayRule, err, want)
		}
	}
	bad := "scrobble"
	if _, err := s.Apply(SettingsPatch{PlayRule: &bad}); !errors.Is(err, ErrBadRequest) {
		t.Errorf("unknown rule: %v", err)
	}
}
