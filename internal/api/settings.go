package api

import (
	"fmt"
	"time"

	"spotussi/internal/derive"
)

// DefaultSettings are the settings of a fresh instance whose display
// timezone defaults to tz.
func DefaultSettings(tz string) Settings {
	return Settings{
		PlayRule: string(derive.RuleFixed), PlayRuleThresholdMs: derive.FallbackThresholdMs,
		Timezone: tz, WeekStart: "monday", DateFormat: "auto",
		SessionGapMinutes: 15,
	}
}

// ValidTimezone reports whether tz is an IANA zone name usable as the
// display timezone ("Local" and "" are rejected: they depend on the host).
func ValidTimezone(tz string) bool {
	if tz == "" || tz == "Local" {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// Apply validates p and returns s with it applied. Every backend uses it,
// so mock and live settings accept exactly the same values. Errors wrap
// ErrBadRequest.
func (s Settings) Apply(p SettingsPatch) (Settings, error) {
	bad := func(msg string) error { return fmt.Errorf("%w: %s", ErrBadRequest, msg) }
	if p.PlayRule != nil {
		// The half-track rule's legacy name is accepted and normalized.
		kind, ok := derive.ParseRuleKind(*p.PlayRule)
		if !ok {
			return s, bad("play_rule must be fixed or half_track")
		}
		s.PlayRule = string(kind)
	}
	if p.PlayRuleThresholdMs != nil {
		if v := *p.PlayRuleThresholdMs; v < 1000 || v > 600_000 {
			return s, bad("play_rule_threshold_ms must be 1000-600000")
		}
		s.PlayRuleThresholdMs = *p.PlayRuleThresholdMs
	}
	if p.Timezone != nil {
		if !ValidTimezone(*p.Timezone) {
			return s, bad("timezone must be an IANA timezone name")
		}
		s.Timezone = *p.Timezone
	}
	if p.WeekStart != nil {
		if *p.WeekStart != "monday" && *p.WeekStart != "sunday" {
			return s, bad("week_start must be monday or sunday")
		}
		s.WeekStart = *p.WeekStart
	}
	if p.DateFormat != nil {
		switch *p.DateFormat {
		case "auto", "ymd", "dmy", "mdy":
		default:
			return s, bad("date_format must be auto, ymd, dmy or mdy")
		}
		s.DateFormat = *p.DateFormat
	}
	if p.SessionGapMinutes != nil {
		if v := *p.SessionGapMinutes; v < 1 || v > 240 {
			return s, bad("session_gap_minutes must be 1-240")
		}
		s.SessionGapMinutes = *p.SessionGapMinutes
	}
	return s, nil
}
