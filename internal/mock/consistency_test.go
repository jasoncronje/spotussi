package mock_test

import (
	"context"
	"testing"
	"time"

	"spotussi/internal/api"
	"spotussi/internal/mock"
	"spotussi/internal/query"
	"spotussi/internal/refdata"
)

// Different endpoints must agree on the same totals: the mock is the
// correctness oracle, so its views of one range have to reconcile.
func TestTotalsReconcile(t *testing.T) {
	end := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	ds := refdata.Generate(refdata.Config{Size: refdata.Small, Seed: 3, End: end})
	b := mock.NewWithClock(ds, func() time.Time { return end })
	ctx := context.Background()
	for _, p := range []query.Preset{query.Today, query.Last7d, query.ThisYear, query.PrevMonth, query.All} {
		spec := query.Spec{Preset: p}
		sum, err := b.OverviewSummary(ctx, spec, query.MetricPlays)
		if err != nil {
			t.Fatal(err)
		}
		cur := sum.Current

		tl, err := b.Timeline(ctx, spec, query.BucketAuto, api.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		var tp, tms, tsk int64
		for _, pt := range tl.Points {
			tp, tms, tsk = tp+pt.Plays, tms+pt.MsPlayed, tsk+pt.Skips
		}
		if tp != cur.Plays || tms != cur.MsPlayed || tsk != cur.Skips {
			t.Errorf("%s: timeline %d/%d/%d vs summary %d/%d/%d", p, tp, tms, tsk, cur.Plays, cur.MsPlayed, cur.Skips)
		}
		// The aligned previous series covers exactly the previous period.
		if prev := sum.PreviousSummary; prev != nil {
			if len(tl.PreviousPoints) != len(tl.Points) {
				t.Errorf("%s: %d previous points for %d points", p, len(tl.PreviousPoints), len(tl.Points))
			}
			var pp, pms int64
			for _, pt := range tl.PreviousPoints {
				pp, pms = pp+pt.Plays, pms+pt.MsPlayed
			}
			// A longer previous period (August before September) loses its
			// trailing buckets to the alignment; otherwise totals match.
			longer := days(tl.Previous.From, tl.Previous.To) > days(tl.Range.From, tl.Range.To)
			if pp > prev.Plays || pms > prev.MsPlayed || (!longer && (pp != prev.Plays || pms != prev.MsPlayed)) {
				t.Errorf("%s: previous timeline %d/%d vs previous summary %d/%d", p, pp, pms, prev.Plays, prev.MsPlayed)
			}
		} else if tl.Previous != nil || len(tl.PreviousPoints) != 0 {
			t.Errorf("%s: previous series without a previous period", p)
		}

		clk, _ := b.Clock(ctx, spec, api.Scope{})
		var cp, cms int64
		for _, h := range clk.Hours {
			cp, cms = cp+h.Plays, cms+h.MsPlayed
		}
		if cp != cur.Plays || cms != cur.MsPlayed {
			t.Errorf("%s: clock %d/%d vs summary %d/%d", p, cp, cms, cur.Plays, cur.MsPlayed)
		}

		// Each play has exactly one track: track ranking sums to the total.
		var trp int64
		var trackCount int64
		for off := 0; ; off += 500 {
			top, err := b.TopTracks(ctx, api.TopParams{Range: spec, Metric: query.MetricPlays, Page: api.Page{Limit: 500, Offset: off}})
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range top.Items {
				trp += it.Plays
				trackCount++
			}
			if off+500 >= top.Total {
				break
			}
		}
		if trp != cur.Plays || trackCount != cur.Tracks {
			t.Errorf("%s: top tracks %d plays over %d tracks vs summary %d over %d", p, trp, trackCount, cur.Plays, cur.Tracks)
		}

		ctxs, _ := b.Contexts(ctx, api.TopParams{Range: spec, Metric: query.MetricTime, Page: api.Page{Limit: 500}})
		cms = ctxs.Unknown.MsPlayed
		for _, it := range ctxs.Items {
			cms += it.MsPlayed
		}
		if ctxs.Total <= 500 && cms != cur.MsPlayed {
			t.Errorf("%s: contexts ms %d vs summary %d", p, cms, cur.MsPlayed)
		}

		// Genre page totals agree with the genre ranking and the scoped views.
		if gs, _ := b.TopGenres(ctx, api.TopParams{Range: spec, Metric: query.MetricPlays, Page: api.Page{Limit: 1}}); len(gs.Items) > 0 {
			top := gs.Items[0]
			gd, err := b.Genre(ctx, top.Genre.ID, spec, query.MetricPlays)
			if err != nil {
				t.Fatal(err)
			}
			if gd.InRange.Rank == nil || *gd.InRange.Rank != 1 || gd.InRange.Plays != top.Plays || gd.InRange.MsPlayed != top.MsPlayed {
				t.Errorf("%s: genre detail %+v vs top genre %+v", p, gd.InRange, top)
			}
			gtl, _ := b.Timeline(ctx, spec, query.BucketAuto, api.Scope{GenreID: top.Genre.ID}, false)
			var gp int64
			for _, pt := range gtl.Points {
				gp += pt.Plays
			}
			if gp != top.Plays {
				t.Errorf("%s: genre timeline %d plays vs top genre %d", p, gp, top.Plays)
			}
		}

		h, _ := b.History(ctx, api.HistoryParams{Range: spec, Limit: 1})
		hc, _ := b.History(ctx, api.HistoryParams{Range: spec, Limit: 1, Counted: ptr(true)})
		if int64(hc.Total) != cur.Plays || h.Total < hc.Total {
			t.Errorf("%s: history counted %d (of %d rows) vs summary plays %d", p, hc.Total, h.Total, cur.Plays)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// days is the inclusive length of a local-date span.
func days(from, to string) int {
	f, _ := time.Parse(query.DateLayout, from)
	t, _ := time.Parse(query.DateLayout, to)
	return int(t.Sub(f).Hours()/24) + 1
}
