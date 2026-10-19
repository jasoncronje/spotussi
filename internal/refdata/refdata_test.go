package refdata

import (
	"reflect"
	"testing"
	"time"
)

var testEnd = time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)

func TestDeterministic(t *testing.T) {
	a := Generate(Config{Size: Tiny, Seed: 7, End: testEnd})
	b := Generate(Config{Size: Tiny, Seed: 7, End: testEnd})
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same config produced different datasets")
	}
	c := Generate(Config{Size: Tiny, Seed: 8, End: testEnd})
	if reflect.DeepEqual(a.Plays, c.Plays) {
		t.Fatal("different seeds produced identical plays")
	}
}

func TestInvariants(t *testing.T) {
	for _, size := range []Size{Tiny, Small} {
		ds := Generate(Config{Size: size, Seed: 1, End: testEnd})
		p := sizes[size]
		if n := len(ds.Plays); n < p.rows*85/100 || n > p.rows*105/100 {
			t.Errorf("%s: %d rows, want about %d", size, n, p.rows)
		}
		if n := len(ds.Tracks); n < p.tracks*7/10 || n > p.tracks*13/10 {
			t.Errorf("%s: %d tracks, want about %d", size, n, p.tracks)
		}
		endMs := ds.End.UnixMilli()
		var prev int64
		for i, pl := range ds.Plays {
			if pl.StartMs < prev {
				t.Fatalf("%s: plays not sorted at %d", size, i)
			}
			prev = pl.StartMs
			if pl.StartMs < ds.Start.UnixMilli() || pl.StartMs >= endMs {
				t.Fatalf("%s: play %d outside [start, end)", size, i)
			}
			tr := ds.Tracks[pl.Track]
			if pl.MsPlayed < 0 || pl.MsPlayed > tr.DurationMs {
				t.Fatalf("%s: play %d ms_played %d outside [0, %d]", size, i, pl.MsPlayed, tr.DurationMs)
			}
			if pl.Flags&(FlagSrcExport|FlagSrcPoller|FlagSrcRecent) == 0 {
				t.Fatalf("%s: play %d has no source", size, i)
			}
			if pl.StartMs < ds.PollerSince && (pl.Context >= 0 || pl.Device >= 0) {
				t.Fatalf("%s: export-era play %d has poller fields", size, i)
			}
			// No track is played before its album is released.
			rel := ds.Albums[tr.Album].ReleaseYear
			if y := time.UnixMilli(pl.StartMs).UTC().Year(); y < rel {
				t.Fatalf("%s: play %d in %d of an album released %d", size, i, y, rel)
			}
		}
	}
}
