// Package query holds the read-side vocabulary shared by every query
// implementation: time ranges, buckets and metrics. The mock backend uses it
// today; the rollup-backed query layer (M5/M6) will use the same code, so
// both resolve "last 4 weeks" identically.
package query

import (
	"errors"
	"fmt"
	"time"
)

// Preset names a range preset (D14 global time-range picker).
type Preset string

const (
	Today     Preset = "today"
	ThisWeek  Preset = "this_week"
	ThisMonth Preset = "this_month"
	ThisYear  Preset = "this_year"
	Last7d    Preset = "last_7d"
	Last4w    Preset = "last_4w"
	Last6m    Preset = "last_6m"
	Last12m   Preset = "last_12m"
	PrevDay   Preset = "prev_day"
	PrevWeek  Preset = "prev_week"
	PrevMonth Preset = "prev_month"
	PrevYear  Preset = "prev_year"
	All       Preset = "all"
	Custom    Preset = "custom"
)

// Presets lists every preset in picker order.
var Presets = []Preset{Today, ThisWeek, ThisMonth, ThisYear, Last7d, Last4w, Last6m, Last12m,
	PrevDay, PrevWeek, PrevMonth, PrevYear, All, Custom}

// DateLayout is the wire format of local dates.
const DateLayout = "2006-01-02"

// ErrInvalid marks invalid query parameters (HTTP 400).
var ErrInvalid = errors.New("invalid parameter")

// Spec is an unresolved range as requested by a client.
type Spec struct {
	Preset   Preset
	From, To string // custom range, inclusive local dates (YYYY-MM-DD)
}

// ParseSpec validates raw query parameters. def is used when neither a
// preset nor from/to is given.
func ParseSpec(preset, from, to string, def Preset) (Spec, error) {
	if preset == "" {
		if from != "" || to != "" {
			preset = string(Custom)
		} else {
			preset = string(def)
		}
	}
	p := Preset(preset)
	known := false
	for _, k := range Presets {
		if k == p {
			known = true
			break
		}
	}
	if !known {
		return Spec{}, fmt.Errorf("%w: unknown range %q", ErrInvalid, preset)
	}
	if p != Custom {
		if from != "" || to != "" {
			return Spec{}, fmt.Errorf("%w: from/to are only valid with range=custom", ErrInvalid)
		}
		return Spec{Preset: p}, nil
	}
	if from == "" || to == "" {
		return Spec{}, fmt.Errorf("%w: custom range needs both from and to", ErrInvalid)
	}
	f, err1 := time.Parse(DateLayout, from)
	t, err2 := time.Parse(DateLayout, to)
	if err1 != nil || err2 != nil {
		return Spec{}, fmt.Errorf("%w: from/to must be YYYY-MM-DD", ErrInvalid)
	}
	if t.Before(f) {
		return Spec{}, fmt.Errorf("%w: to is before from", ErrInvalid)
	}
	return Spec{Preset: Custom, From: from, To: to}, nil
}

// Env is what range resolution depends on besides the request.
type Env struct {
	Now       time.Time
	Loc       *time.Location
	WeekStart time.Weekday
	// FirstPlay is the start of the earliest play (zero if there are none);
	// it anchors the "all" preset.
	FirstPlay time.Time
}

// Range is a resolved range. Start/End are the effective bounds (End
// exclusive, capped at the current minute while in progress); NominalEnd is
// the end of the period as a whole (midnight after To).
type Range struct {
	Preset     Preset
	From, To   string // inclusive local dates of the nominal period
	Start, End time.Time
	NominalEnd time.Time
	Loc        *time.Location
	// FirstPlay is Env.FirstPlay (the earliest play of the whole history,
	// zero if none), carried so responses can report it.
	FirstPlay time.Time
}

// FirstPlayDate is the local date of FirstPlay, or "" when there are no plays.
func (r Range) FirstPlayDate() string {
	if r.FirstPlay.IsZero() {
		return ""
	}
	loc := r.Loc
	if loc == nil {
		loc = time.UTC
	}
	return r.FirstPlay.In(loc).Format(DateLayout)
}

// Contains reports whether t falls inside [Start, End).
func (r Range) Contains(t time.Time) bool { return !t.Before(r.Start) && t.Before(r.End) }

// SpanDays is the length of the nominal period in local days.
func (r Range) SpanDays() int {
	return daysBetween(r.Start, r.NominalEnd)
}

func midnight(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func daysBetween(a, b time.Time) int {
	// Calendar days, robust to DST: compare civil dates at UTC noon.
	ua := time.Date(a.Year(), a.Month(), a.Day(), 12, 0, 0, 0, time.UTC)
	ub := time.Date(b.Year(), b.Month(), b.Day(), 12, 0, 0, 0, time.UTC)
	return int(ub.Sub(ua).Hours()+0.5) / 24
}

// WeekStartOf returns local midnight of the first day of t's week.
func WeekStartOf(t time.Time, loc *time.Location, weekStart time.Weekday) time.Time {
	d := midnight(t, loc)
	off := (int(d.Weekday()) - int(weekStart) + 7) % 7
	return d.AddDate(0, 0, -off)
}

// Resolve turns a Spec into the current range and its previous period
// (nil for "all").
//
// The previous period is the nominal period shifted back by one period
// (calendar-aware for calendar presets). When the current range is still in
// progress, the previous one is truncated to the same elapsed time, so
// "this month so far" is compared with "last month up to the same point".
func Resolve(spec Spec, env Env) (Range, *Range, error) {
	loc := env.Loc
	if loc == nil {
		loc = time.UTC
	}
	now := env.Now.In(loc).Truncate(time.Minute)
	today := midnight(now, loc)
	tomorrow := today.AddDate(0, 0, 1)
	week := WeekStartOf(now, loc, env.WeekStart)
	month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc)
	year := time.Date(today.Year(), 1, 1, 0, 0, 0, 0, loc)

	var s, e time.Time
	var shift func(time.Time) time.Time
	days := func(n int) func(time.Time) time.Time {
		return func(t time.Time) time.Time { return t.AddDate(0, 0, -n) }
	}
	months := func(n int) func(time.Time) time.Time {
		return func(t time.Time) time.Time { return t.AddDate(0, -n, 0) }
	}

	switch spec.Preset {
	case Today:
		s, e, shift = today, tomorrow, days(1)
	case ThisWeek:
		s, e, shift = week, week.AddDate(0, 0, 7), days(7)
	case ThisMonth:
		s, e, shift = month, month.AddDate(0, 1, 0), months(1)
	case ThisYear:
		s, e, shift = year, year.AddDate(1, 0, 0), months(12)
	case Last7d:
		s, e, shift = today.AddDate(0, 0, -6), tomorrow, days(7)
	case Last4w:
		s, e, shift = today.AddDate(0, 0, -27), tomorrow, days(28)
	case Last6m:
		s, e, shift = tomorrow.AddDate(0, -6, 0), tomorrow, months(6)
	case Last12m:
		s, e, shift = tomorrow.AddDate(-1, 0, 0), tomorrow, months(12)
	case PrevDay:
		s, e, shift = today.AddDate(0, 0, -1), today, days(1)
	case PrevWeek:
		s, e, shift = week.AddDate(0, 0, -7), week, days(7)
	case PrevMonth:
		s, e, shift = month.AddDate(0, -1, 0), month, months(1)
	case PrevYear:
		s, e, shift = year.AddDate(-1, 0, 0), year, months(12)
	case All:
		s, e = today, tomorrow
		if !env.FirstPlay.IsZero() {
			s = midnight(env.FirstPlay, loc)
		}
	case Custom:
		f, err1 := time.ParseInLocation(DateLayout, spec.From, loc)
		t, err2 := time.ParseInLocation(DateLayout, spec.To, loc)
		if err1 != nil || err2 != nil || t.Before(f) {
			return Range{}, nil, fmt.Errorf("%w: bad custom range %q..%q", ErrInvalid, spec.From, spec.To)
		}
		s, e = f, t.AddDate(0, 0, 1)
		shift = days(daysBetween(s, e))
	default:
		return Range{}, nil, fmt.Errorf("%w: unknown range %q", ErrInvalid, spec.Preset)
	}

	end := now
	if end.After(e) {
		end = e
	}
	if end.Before(s) {
		end = s
	}
	cur := Range{Preset: spec.Preset, From: s.Format(DateLayout), To: e.AddDate(0, 0, -1).Format(DateLayout),
		Start: s, End: end, NominalEnd: e, Loc: loc, FirstPlay: env.FirstPlay}
	if shift == nil {
		return cur, nil, nil
	}
	ps, pe := shift(s), shift(e)
	pend := pe
	if end.Before(e) {
		if cand := ps.Add(end.Sub(s)); cand.Before(pe) {
			pend = cand
		}
	}
	prev := Range{Preset: spec.Preset, From: ps.Format(DateLayout), To: pe.AddDate(0, 0, -1).Format(DateLayout),
		Start: ps, End: pend, NominalEnd: pe, Loc: loc, FirstPlay: env.FirstPlay}
	return cur, &prev, nil
}
