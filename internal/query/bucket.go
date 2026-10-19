package query

import (
	"fmt"
	"time"
)

// Bucket is a time-series bucket size.
type Bucket string

const (
	BucketAuto  Bucket = "auto"
	BucketHour  Bucket = "hour"
	BucketDay   Bucket = "day"
	BucketWeek  Bucket = "week"
	BucketMonth Bucket = "month"
	BucketYear  Bucket = "year"
)

// MaxBuckets caps the number of points a series may have.
const MaxBuckets = 10_000

// ParseBucket validates a bucket parameter ("" means auto).
func ParseBucket(s string) (Bucket, error) {
	switch b := Bucket(s); b {
	case "":
		return BucketAuto, nil
	case BucketAuto, BucketHour, BucketDay, BucketWeek, BucketMonth, BucketYear:
		return b, nil
	}
	return "", fmt.Errorf("%w: unknown bucket %q", ErrInvalid, s)
}

// AutoBucket picks a bucket size from the nominal span of r.
func AutoBucket(r Range) Bucket {
	switch d := r.SpanDays(); {
	case d <= 2:
		return BucketHour
	case d <= 62:
		return BucketDay
	case d <= 190:
		return BucketWeek
	default:
		return BucketMonth
	}
}

// Span is one bucket: [Start, End) in absolute time, labelled by Key.
type Span struct {
	Key        string
	Start, End time.Time
}

// Buckets enumerates the buckets covering r's nominal period (so in-progress
// ranges include their empty future buckets). The first bucket starts at the
// bucket boundary at or before r.Start; data must still be clipped to r.
func Buckets(r Range, b Bucket, weekStart time.Weekday) (Bucket, []Span, error) {
	if b == BucketAuto || b == "" {
		b = AutoBucket(r)
	}
	loc := r.Loc
	if loc == nil {
		loc = time.UTC
	}
	var t time.Time
	var next func(time.Time) time.Time
	var layout string
	st := r.Start.In(loc)
	switch b {
	case BucketHour:
		t = time.Date(st.Year(), st.Month(), st.Day(), st.Hour(), 0, 0, 0, loc)
		next = func(x time.Time) time.Time { return x.Add(time.Hour) }
		layout = "2006-01-02T15"
	case BucketDay:
		t = midnight(st, loc)
		next = func(x time.Time) time.Time { return x.AddDate(0, 0, 1) }
		layout = DateLayout
	case BucketWeek:
		t = WeekStartOf(st, loc, weekStart)
		next = func(x time.Time) time.Time { return x.AddDate(0, 0, 7) }
		layout = DateLayout
	case BucketMonth:
		t = time.Date(st.Year(), st.Month(), 1, 0, 0, 0, 0, loc)
		next = func(x time.Time) time.Time { return x.AddDate(0, 1, 0) }
		layout = "2006-01"
	case BucketYear:
		t = time.Date(st.Year(), 1, 1, 0, 0, 0, 0, loc)
		next = func(x time.Time) time.Time { return x.AddDate(1, 0, 0) }
		layout = "2006"
	default:
		return "", nil, fmt.Errorf("%w: unknown bucket %q", ErrInvalid, b)
	}
	var out []Span
	for t.Before(r.NominalEnd) {
		n := next(t)
		out = append(out, Span{Key: t.In(loc).Format(layout), Start: t, End: n})
		if len(out) > MaxBuckets {
			return "", nil, fmt.Errorf("%w: more than %d %s buckets; use a larger bucket", ErrInvalid, MaxBuckets, b)
		}
		t = n
	}
	return b, out, nil
}

// AlignedBuckets returns n buckets of size b for prev, aligned by position
// with a series of n buckets for the current period: bucket i of the result
// is the i-th bucket of prev. When prev's period has fewer buckets (e.g. a
// shorter month), the series continues past its nominal end so the lengths
// match; data must still be clipped to prev.
func AlignedBuckets(prev Range, b Bucket, weekStart time.Weekday, n int) ([]Span, error) {
	r := prev
	// End the enumeration n+1 buckets after the start (the first bucket
	// starts at or before it, so n buckets always fit).
	switch b {
	case BucketHour:
		r.NominalEnd = prev.Start.Add(time.Duration(n+1) * time.Hour)
	case BucketDay:
		r.NominalEnd = prev.Start.AddDate(0, 0, n+1)
	case BucketWeek:
		r.NominalEnd = prev.Start.AddDate(0, 0, 7*(n+1))
	case BucketMonth:
		r.NominalEnd = prev.Start.AddDate(0, n+1, 0)
	case BucketYear:
		r.NominalEnd = prev.Start.AddDate(n+1, 0, 0)
	default:
		return nil, fmt.Errorf("%w: bucket %q must be resolved", ErrInvalid, b)
	}
	_, spans, err := Buckets(r, b, weekStart)
	if err != nil {
		return nil, err
	}
	if len(spans) < n {
		return nil, fmt.Errorf("%w: cannot align %d buckets", ErrInvalid, n)
	}
	return spans[:n], nil
}

// Metric selects the ranking measure (D7 Plays/Time toggle).
type Metric string

const (
	MetricPlays Metric = "plays"
	MetricTime  Metric = "time"
)

// ParseMetric validates a metric parameter ("" means plays).
func ParseMetric(s string) (Metric, error) {
	switch m := Metric(s); m {
	case "":
		return MetricPlays, nil
	case MetricPlays, MetricTime:
		return m, nil
	}
	return "", fmt.Errorf("%w: metric must be plays or time", ErrInvalid)
}
