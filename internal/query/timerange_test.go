package query

import (
	"errors"
	"testing"
	"time"
)

var london, _ = time.LoadLocation("Europe/London")

// Friday 17 Oct 2026, 14:30:45 local.
var now = time.Date(2026, 10, 17, 14, 30, 45, 0, london)

func env() Env {
	return Env{Now: now, Loc: london, WeekStart: time.Monday,
		FirstPlay: time.Date(2011, 3, 4, 9, 0, 0, 0, london)}
}

func resolve(t *testing.T, p Preset) (Range, *Range) {
	t.Helper()
	cur, prev, err := Resolve(Spec{Preset: p}, env())
	if err != nil {
		t.Fatal(err)
	}
	return cur, prev
}

func TestResolvePresets(t *testing.T) {
	nowMin := time.Date(2026, 10, 17, 14, 30, 0, 0, london)
	cases := []struct {
		p          Preset
		from, to   string
		end        time.Time
		pfrom, pto string
		pend       time.Time
	}{
		{Today, "2026-10-17", "2026-10-17", nowMin, "2026-10-16", "2026-10-16", time.Date(2026, 10, 16, 14, 30, 0, 0, london)},
		{ThisWeek, "2026-10-12", "2026-10-18", nowMin, "2026-10-05", "2026-10-11", time.Date(2026, 10, 10, 14, 30, 0, 0, london)},
		{ThisMonth, "2026-10-01", "2026-10-31", nowMin, "2026-09-01", "2026-09-30", time.Date(2026, 9, 17, 14, 30, 0, 0, london)},
		{Last7d, "2026-10-11", "2026-10-17", nowMin, "2026-10-04", "2026-10-10", time.Date(2026, 10, 10, 14, 30, 0, 0, london)},
		{Last4w, "2026-09-20", "2026-10-17", nowMin, "2026-08-23", "2026-09-19", time.Date(2026, 9, 19, 14, 30, 0, 0, london)},
		{Last12m, "2025-10-18", "2026-10-17", nowMin, "2024-10-18", "2025-10-17", time.Date(2025, 10, 17, 14, 30, 0, 0, london)},
		{PrevDay, "2026-10-16", "2026-10-16", time.Date(2026, 10, 17, 0, 0, 0, 0, london), "2026-10-15", "2026-10-15", time.Date(2026, 10, 16, 0, 0, 0, 0, london)},
		{PrevWeek, "2026-10-05", "2026-10-11", time.Date(2026, 10, 12, 0, 0, 0, 0, london), "2026-09-28", "2026-10-04", time.Date(2026, 10, 5, 0, 0, 0, 0, london)},
		{PrevMonth, "2026-09-01", "2026-09-30", time.Date(2026, 10, 1, 0, 0, 0, 0, london), "2026-08-01", "2026-08-31", time.Date(2026, 9, 1, 0, 0, 0, 0, london)},
		{PrevYear, "2025-01-01", "2025-12-31", time.Date(2026, 1, 1, 0, 0, 0, 0, london), "2024-01-01", "2024-12-31", time.Date(2025, 1, 1, 0, 0, 0, 0, london)},
	}
	for _, c := range cases {
		cur, prev := resolve(t, c.p)
		if cur.From != c.from || cur.To != c.to || !cur.End.Equal(c.end) {
			t.Errorf("%s: got %s..%s end %v; want %s..%s end %v", c.p, cur.From, cur.To, cur.End, c.from, c.to, c.end)
		}
		if prev == nil {
			t.Errorf("%s: no previous", c.p)
			continue
		}
		if prev.From != c.pfrom || prev.To != c.pto || !prev.End.Equal(c.pend) {
			t.Errorf("%s prev: got %s..%s end %v; want %s..%s end %v", c.p, prev.From, prev.To, prev.End, c.pfrom, c.pto, c.pend)
		}
	}
}

func TestResolveAllAndCustom(t *testing.T) {
	cur, prev := resolve(t, All)
	if cur.From != "2011-03-04" || cur.To != "2026-10-17" || prev != nil {
		t.Errorf("all: %+v prev %v", cur, prev)
	}
	spec, err := ParseSpec("", "2026-01-01", "2026-01-10", Last4w)
	if err != nil {
		t.Fatal(err)
	}
	cur, prev, err = Resolve(spec, env())
	if err != nil {
		t.Fatal(err)
	}
	if cur.Preset != Custom || cur.SpanDays() != 10 || prev.From != "2025-12-22" || prev.To != "2025-12-31" {
		t.Errorf("custom: %+v prev %+v", cur, prev)
	}
	if !cur.End.Equal(cur.NominalEnd) {
		t.Error("completed custom range should end at its nominal end")
	}
}

func TestParseSpecErrors(t *testing.T) {
	bad := [][3]string{
		{"nope", "", ""},
		{"custom", "2026-01-01", ""},
		{"today", "2026-01-01", "2026-01-02"},
		{"", "2026-02-01", "2026-01-01"},
		{"", "2026-13-01", "2026-12-01"},
	}
	for _, b := range bad {
		if _, err := ParseSpec(b[0], b[1], b[2], Last4w); !errors.Is(err, ErrInvalid) {
			t.Errorf("%v: expected ErrInvalid, got %v", b, err)
		}
	}
	if s, err := ParseSpec("", "", "", Last4w); err != nil || s.Preset != Last4w {
		t.Errorf("default: %v %v", s, err)
	}
}

func TestBuckets(t *testing.T) {
	cur, _ := resolve(t, ThisMonth)
	b, spans, err := Buckets(cur, BucketAuto, time.Monday)
	if err != nil || b != BucketDay || len(spans) != 31 || spans[0].Key != "2026-10-01" || spans[30].Key != "2026-10-31" {
		t.Fatalf("this_month auto: %v %d %v", b, len(spans), err)
	}
	// DST ends in London on 25 Oct 2026: that day has 25 hours.
	if d := spans[24].End.Sub(spans[24].Start); d != 25*time.Hour {
		t.Errorf("DST day length = %v", d)
	}
	cur, _ = resolve(t, Today)
	if b, spans, _ := Buckets(cur, BucketAuto, time.Monday); b != BucketHour || len(spans) != 24 || spans[13].Key != "2026-10-17T13" {
		t.Errorf("today: %v %d", b, len(spans))
	}
	cur, _ = resolve(t, Last6m)
	if b, spans, _ := Buckets(cur, BucketAuto, time.Monday); b != BucketWeek || spans[0].Start.Weekday() != time.Monday {
		t.Errorf("last_6m: %v %v", b, spans[0])
	}
	cur, _ = resolve(t, All)
	if b, spans, _ := Buckets(cur, BucketAuto, time.Monday); b != BucketMonth || spans[0].Key != "2011-03" || spans[len(spans)-1].Key != "2026-10" {
		t.Errorf("all: %v %v..%v", b, spans[0].Key, spans[len(spans)-1].Key)
	}
	if _, _, err := Buckets(cur, BucketHour, time.Monday); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected too-many-buckets error, got %v", err)
	}
}

func TestAlignedBuckets(t *testing.T) {
	// September by day (30 buckets) against August (31 days).
	cur, prev := resolve(t, PrevMonth)
	bk, spans, err := Buckets(cur, BucketAuto, time.Monday)
	if err != nil || bk != BucketDay || len(spans) != 30 {
		t.Fatalf("current: %s %d %v", bk, len(spans), err)
	}
	ps, err := AlignedBuckets(*prev, bk, time.Monday, len(spans))
	if err != nil || len(ps) != 30 || ps[0].Key != "2026-08-01" || ps[29].Key != "2026-08-30" {
		t.Fatalf("aligned: %d %v %v", len(ps), ps, err)
	}
	// A 31-day custom range: the previous 31 days, then no overflow.
	cur, p2, _ := Resolve(Spec{Preset: Custom, From: "2026-03-01", To: "2026-03-31"}, env())
	_, spans, _ = Buckets(cur, BucketDay, time.Monday)
	ps, _ = AlignedBuckets(*p2, BucketDay, time.Monday, len(spans))
	if len(ps) != 31 || ps[0].Key != "2026-01-29" || ps[30].Key != "2026-02-28" {
		t.Fatalf("custom aligned: %d %s..%s", len(ps), ps[0].Key, ps[len(ps)-1].Key)
	}
	// Weeks of the last 6 months may need one bucket past the period.
	cur, prev = resolve(t, Last6m)
	_, spans, _ = Buckets(cur, BucketWeek, time.Monday)
	if ps, err = AlignedBuckets(*prev, BucketWeek, time.Monday, len(spans)); err != nil || len(ps) != len(spans) {
		t.Fatalf("weeks aligned: %d vs %d %v", len(ps), len(spans), err)
	}
	if got := cur.FirstPlayDate(); got != "2011-03-04" {
		t.Errorf("first play date %q", got)
	}
}
