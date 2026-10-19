package live

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"spotussi/internal/api"
)

// Settings are stored one row per field in the settings table (JSON
// scalars, defaults in code: a missing row means "default").
const (
	keyPlayRule      = "play_rule"
	keyPlayThreshold = "play_rule_threshold_ms"
	keyTimezone      = "timezone"
	keyWeekStart     = "week_start"
	keyDateFormat    = "date_format"
	keySessionGap    = "session_gap_minutes"
)

func (b *Backend) Settings(ctx context.Context) (*api.Settings, error) {
	s, err := b.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (b *Backend) UpdateSettings(ctx context.Context, p api.SettingsPatch) (*api.Settings, error) {
	b.settingsMu.Lock()
	defer b.settingsMu.Unlock()
	cur, err := b.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	next, err := cur.Apply(p)
	if err != nil {
		return nil, err
	}
	// Every field in the patch is stored, even one equal to its default,
	// so a later change of the default (e.g. the host's zone) does not
	// move an explicit choice.
	values := map[string]any{}
	for _, k := range patchKeys(p) {
		values[k] = fieldValue(next, k)
	}
	if len(values) > 0 {
		if err := b.db.SetSettings(ctx, values); err != nil {
			return nil, err
		}
	}
	b.cacheMu.Lock()
	b.cached = &next
	b.cacheMu.Unlock()
	// The view (play rule, local hours, sessions) is recomputed at once;
	// local days in the plays table follow in a debounced rebuild (D3).
	b.engine.SettingsChanged(deriveSettings(next))
	return &next, nil
}

func patchKeys(p api.SettingsPatch) []string {
	var keys []string
	add := func(set bool, k string) {
		if set {
			keys = append(keys, k)
		}
	}
	add(p.PlayRule != nil, keyPlayRule)
	add(p.PlayRuleThresholdMs != nil, keyPlayThreshold)
	add(p.Timezone != nil, keyTimezone)
	add(p.WeekStart != nil, keyWeekStart)
	add(p.DateFormat != nil, keyDateFormat)
	add(p.SessionGapMinutes != nil, keySessionGap)
	return keys
}

func fieldValue(s api.Settings, key string) any {
	switch key {
	case keyPlayRule:
		return s.PlayRule
	case keyPlayThreshold:
		return s.PlayRuleThresholdMs
	case keyTimezone:
		return s.Timezone
	case keyWeekStart:
		return s.WeekStart
	case keyDateFormat:
		return s.DateFormat
	default:
		return s.SessionGapMinutes
	}
}

// loadSettings reads the stored settings over the defaults. A stored value
// that no longer validates (e.g. a timezone the tzdata dropped) falls back
// to the default with a warning rather than failing every request.
//
// Settings change only through UpdateSettings, so after the first read
// they are served from memory (every query reads them).
func (b *Backend) loadSettings(ctx context.Context) (api.Settings, error) {
	b.cacheMu.Lock()
	c := b.cached
	b.cacheMu.Unlock()
	if c != nil {
		return *c, nil
	}
	s, err := b.readSettings(ctx)
	if err == nil {
		b.cacheMu.Lock()
		if b.cached == nil {
			b.cached = &s
		}
		b.cacheMu.Unlock()
	}
	return s, err
}

func (b *Backend) readSettings(ctx context.Context) (api.Settings, error) {
	s := api.DefaultSettings(b.defaultTZ)
	rows, err := b.db.Read.QueryContext(ctx, `SELECT key, value FROM settings WHERE key IN (?, ?, ?, ?, ?, ?)`,
		keyPlayRule, keyPlayThreshold, keyTimezone, keyWeekStart, keyDateFormat, keySessionGap)
	if err != nil {
		return s, fmt.Errorf("read settings: %w", err)
	}
	defer rows.Close()
	var p api.SettingsPatch
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return s, err
		}
		var dst any
		switch key {
		case keyPlayRule:
			dst = &p.PlayRule
		case keyPlayThreshold:
			dst = &p.PlayRuleThresholdMs
		case keyTimezone:
			dst = &p.Timezone
		case keyWeekStart:
			dst = &p.WeekStart
		case keyDateFormat:
			dst = &p.DateFormat
		case keySessionGap:
			dst = &p.SessionGapMinutes
		}
		if err := json.Unmarshal([]byte(raw), dst); err != nil {
			slog.Warn("settings: ignoring undecodable value", "key", key, "err", err)
		}
	}
	if err := rows.Err(); err != nil {
		return s, err
	}
	// Apply field by field so one bad stored value doesn't drop the rest.
	for _, one := range splitPatch(p) {
		if next, err := s.Apply(one); err == nil {
			s = next
		} else {
			slog.Warn("settings: ignoring invalid stored value", "err", err)
		}
	}
	return s, nil
}

func splitPatch(p api.SettingsPatch) []api.SettingsPatch {
	var out []api.SettingsPatch
	if p.PlayRule != nil {
		out = append(out, api.SettingsPatch{PlayRule: p.PlayRule})
	}
	if p.PlayRuleThresholdMs != nil {
		out = append(out, api.SettingsPatch{PlayRuleThresholdMs: p.PlayRuleThresholdMs})
	}
	if p.Timezone != nil {
		out = append(out, api.SettingsPatch{Timezone: p.Timezone})
	}
	if p.WeekStart != nil {
		out = append(out, api.SettingsPatch{WeekStart: p.WeekStart})
	}
	if p.DateFormat != nil {
		out = append(out, api.SettingsPatch{DateFormat: p.DateFormat})
	}
	if p.SessionGapMinutes != nil {
		out = append(out, api.SettingsPatch{SessionGapMinutes: p.SessionGapMinutes})
	}
	return out
}
