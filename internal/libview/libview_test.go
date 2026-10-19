package libview

import (
	"testing"

	"spotussi/internal/api"
)

const d = int64(dayMs)

func filled(added int64, starts ...int64) Track {
	t := Track{AddedAt: added}
	var s Starts
	for _, st := range starts {
		s.Add(st, 1000)
	}
	s.AddMs(500) // a play that didn't count
	s.Fill(&t)
	return t
}

func TestFill(t *testing.T) {
	tr := filled(10*d, 30*d, 2*d, 11*d, 12*d, 13*d, 14*d)
	if tr.Plays != 6 || tr.Ms != 6500 || tr.First != 2*d || tr.Last != 30*d {
		t.Fatalf("plays %d ms %d first %d last %d", tr.Plays, tr.Ms, tr.First, tr.Last)
	}
	// Sorted: 2, 11, 12, 13, 14, 30 → the 5th play is day 14.
	if tr.Rotation != 14*d || tr.FirstAfter != 11*d {
		t.Fatalf("rotation %d first after %d", tr.Rotation/d, tr.FirstAfter/d)
	}
	if tr := filled(40*d, 2*d); tr.FirstAfter != 0 || tr.Rotation != 0 {
		t.Fatalf("no play after saving: %+v", tr)
	}
}

func TestStatus(t *testing.T) {
	for _, c := range []struct {
		plays int64
		same  bool
		want  string
	}{{3, false, api.StatusPlayed}, {1, true, api.StatusPlayed}, {0, true, api.StatusMaybePlayed}, {0, false, api.StatusNeverPlayed}} {
		if got := Status(c.plays, c.same); got != c.want {
			t.Errorf("Status(%d, %v) = %s", c.plays, c.same, got)
		}
	}
	if MatchKey("Song (Remastered)", "The Band") == MatchKey("Other", "The Band") {
		t.Error("different titles share a key")
	}
}

func TestSummaryRotationMix(t *testing.T) {
	five := func(added, from int64) Track {
		tr := filled(added, from, from+1, from+2, from+3, from+4)
		tr.Status = api.StatusPlayed
		return tr
	}
	tracks := []Track{
		five(10*d, 10*d),   // within a day
		five(10*d, 15*d),   // within a week
		five(10*d, 30*d),   // within a month
		five(10*d, 90*d),   // later
		five(10*d, 1*d),    // before saving
		five(0, 5*d),       // no save date
		filled(10*d, 12*d), // not yet (one play)
		{Status: api.StatusMaybePlayed},
	}
	s := Summary(tracks, 2, 3, 99)
	m := s.RotationMix
	if m.WithinDay != 1 || m.WithinWeek != 1 || m.WithinMonth != 1 || m.Later != 1 || m.BeforeSaving != 2 || m.NotYet != 2 {
		t.Fatalf("mix %+v", m)
	}
	if s.Tracks.Played != 6 || s.Tracks.MaybePlayed != 1 || s.Tracks.NeverPlayed != 1 || s.SavedTracks != 8 {
		t.Fatalf("tracks %+v", s.Tracks)
	}
	// Days to first play after saving: 0, 5, 20, 80 and 2 → median 5.
	if s.DaysToFirstPlay.Median == nil || *s.DaysToFirstPlay.Median != 5 || s.DaysToFirstPlay.Count != 5 {
		t.Fatalf("first play %+v", s.DaysToFirstPlay)
	}
}
