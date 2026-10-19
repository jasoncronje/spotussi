package capture

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// Snapshot builders. Times and positions are in seconds (fractions
// allowed) to keep the recorded sequences readable.

func ms(sec float64) int64 { return int64(sec * 1000) }

type sb struct{ Snapshot }

func track(t float64, id string, durSec, posSec float64, playing bool) sb {
	return sb{Snapshot{At: ms(t), Kind: KindTrack, Playing: playing, ProgressMs: ms(posSec),
		Track:  &Track{ID: id, URI: "spotify:track:" + id, DurationMs: ms(durSec)},
		Repeat: "off", DeviceName: "Laptop", DeviceType: "Computer", Raw: json.RawMessage(`{"t":` + jsonNum(t) + `}`)}}
}

func jsonNum(f float64) string { b, _ := json.Marshal(f); return string(b) }

func on(t float64, id string, dur, pos float64) sb     { return track(t, id, dur, pos, true) }
func paused(t float64, id string, dur, pos float64) sb { return track(t, id, dur, pos, false) }
func nothing(t float64) sb                             { return sb{Snapshot{At: ms(t), Kind: KindNothing}} }
func other(t float64, k Kind, pos float64) sb {
	return sb{Snapshot{At: ms(t), Kind: k, Playing: true, ProgressMs: ms(pos)}}
}

func (s sb) dev(name, typ string) sb { s.DeviceName, s.DeviceType = name, typ; return s }
func (s sb) ctx(uri string) sb       { s.ContextURI = uri; return s }
func (s sb) shuffle() sb             { s.Shuffle = true; return s }
func (s sb) repeat(r string) sb      { s.Repeat = r; return s }
func (s sb) private() sb             { s.Private = true; return s }
func (s sb) snap() Snapshot          { return s.Snapshot }
func seq(ss ...sb) []Snapshot {
	out := make([]Snapshot, len(ss))
	for i, s := range ss {
		out[i] = s.Snapshot
	}
	return out
}

// steady plays id from position pos0 at time t0, polling every 12 s
// until just before its end, and returns the snapshots plus the time of
// the last one.
func steady(t0 float64, id string, dur, pos0 float64) ([]Snapshot, float64) {
	var out []Snapshot
	t, pos := t0, pos0
	for pos < dur {
		out = append(out, on(t, id, dur, pos).snap())
		step := min(12, dur-pos+1.5)
		if pos+step > dur {
			break
		}
		t, pos = t+step, pos+step
	}
	return out, t
}

// want is the expectation for one emitted play. Zero-valued optional
// fields are not checked (use the check func for those).
type want struct {
	track    string
	msPlayed float64 // seconds, exact
	skipped  bool
	reason   string
	started  *float64
	ended    *float64
	check    func(t *testing.T, p Play)
}

func f(v float64) *float64 { return &v }

func run(snaps []Snapshot) ([]Play, *Machine) {
	m := NewMachine(Config{}, MachineState{})
	var plays []Play
	for _, s := range snaps {
		plays = append(plays, m.Observe(s)...)
	}
	return plays, m
}

func TestMachine(t *testing.T) {
	normal, last := steady(0, "A", 200, 3) // last poll at t=192, pos 195
	normal = append(normal, on(198.5, "B", 180, 1.5).snap(), nothing(210).snap())

	cases := []struct {
		name  string
		snaps []Snapshot
		want  []want
		open  string // track of the play still open at the end ("" = none)
	}{
		{
			name: "natural end: end-of-track poll sees the next track",
			// A is first seen at 3 s (credited in full: nothing before it),
			// B starts at 197 s, A gets the remaining 5 s.
			snaps: normal,
			want: []want{
				{track: "A", msPlayed: 200, reason: EndTrackChange, started: f(-3), ended: f(197),
					check: func(t *testing.T, p Play) {
						if p.Observations != int(last/12)+1 || p.Seeks != 0 || p.Pauses != 0 {
							t.Errorf("counters %+v", p)
						}
					}},
				// B: started at 197, credited 1.5 s then stopped 11.5 s later.
				{track: "B", msPlayed: 13, reason: EndStopped, started: f(197), ended: f(210)},
			},
		},
		{
			name: "skip: track changes well before the end",
			snaps: seq(on(0, "A", 200, 3), on(12, "A", 200, 15),
				on(24, "B", 180, 4)), // B started at 20: A was skipped at 23 s
			want: []want{{track: "A", msPlayed: 23, skipped: true, reason: EndTrackChange, started: f(-3), ended: f(20)}},
			open: "B",
		},
		{
			name: "crossfade is not a skip",
			snaps: seq(on(0, "A", 200, 180), on(12, "A", 200, 192),
				on(18, "B", 180, 8)), // B started at 10 s, 10 s before A's end
			// B's start (10 s) is before A's last sighting: nothing more to
			// credit, and A ended 8 s before its end: within the margin.
			want: []want{{track: "A", msPlayed: 180 + 12, skipped: false, reason: EndTrackChange}},
			open: "B",
		},
		{
			name: "pause and resume: paused time is not credited",
			snaps: seq(on(0, "A", 300, 10), on(12, "A", 300, 22),
				paused(24, "A", 300, 30), // paused at 20 (pos 30)
				paused(114, "A", 300, 30),
				on(126, "A", 300, 35), // resumed at 121
				on(138, "A", 300, 47),
				on(150, "B", 200, 2)), // A skipped at 148 (pos 57)
			want: []want{{track: "A", msPlayed: 10 + 12 + 8 + 0 + 5 + 12 + 10, skipped: true, reason: EndTrackChange,
				check: func(t *testing.T, p Play) {
					if p.Pauses != 1 || p.Seeks != 0 {
						t.Errorf("pauses %d seeks %d", p.Pauses, p.Seeks)
					}
				}}},
			open: "B",
		},
		{
			name: "seek forward while playing: wall-clock time is credited",
			snaps: seq(on(0, "A", 300, 10), on(12, "A", 300, 100), on(24, "A", 300, 112),
				nothing(30)),
			want: []want{{track: "A", msPlayed: 10 + 12 + 12 + 6, reason: EndStopped,
				check: func(t *testing.T, p Play) {
					if p.Seeks != 1 {
						t.Errorf("seeks %d", p.Seeks)
					}
				}}},
		},
		{
			name:  "seek back while playing (not to the start) stays one play",
			snaps: seq(on(0, "A", 300, 100), on(12, "A", 300, 50), on(24, "A", 300, 62), nothing(30)),
			want: []want{{track: "A", msPlayed: 100 + 12 + 12 + 6, reason: EndStopped,
				check: func(t *testing.T, p Play) {
					if p.Seeks != 1 {
						t.Errorf("seeks %d", p.Seeks)
					}
				}}},
		},
		{
			name:  "seek while paused is not credited",
			snaps: seq(on(0, "A", 300, 10), paused(12, "A", 300, 100), on(24, "A", 300, 105), nothing(30)),
			want: []want{{track: "A", msPlayed: 10 + 0 + 5 + 6, reason: EndStopped,
				check: func(t *testing.T, p Play) {
					if p.Seeks != 1 || p.Pauses != 1 {
						t.Errorf("seeks %d pauses %d", p.Seeks, p.Pauses)
					}
				}}},
		},
		{
			name: "repeat-one: the same track starting over is a new play",
			snaps: seq(on(0, "A", 60, 2).repeat("track"), on(12, "A", 60, 14), on(24, "A", 60, 26),
				on(36, "A", 60, 38), on(48, "A", 60, 50),
				on(61.5, "A", 60, 3.5), // ended and restarted at 58
				on(73.5, "A", 60, 15.5), nothing(80)),
			want: []want{
				{track: "A", msPlayed: 60, reason: EndRestart, skipped: false, started: f(-2), ended: f(58)},
				{track: "A", msPlayed: 22, reason: EndStopped, started: f(58), ended: f(80)},
			},
		},
		{
			name: "previous: back to the start mid-track is a restart",
			snaps: seq(on(0, "A", 200, 30), on(12, "A", 200, 42),
				on(24, "A", 200, 5), // restarted at 19 (pos 49)
				nothing(30)),
			want: []want{
				{track: "A", msPlayed: 49, reason: EndRestart, skipped: true, ended: f(19)},
				{track: "A", msPlayed: 11, reason: EndStopped, started: f(19)},
			},
		},
		{
			name: "device switch keeps the play; the main device wins",
			snaps: seq(on(0, "A", 200, 10).dev("Laptop", "Computer"),
				on(12, "A", 200, 22).dev("Phone", "Smartphone"),
				on(24, "A", 200, 34).dev("Phone", "Smartphone"),
				on(36, "A", 200, 46).dev("Phone", "Smartphone"),
				nothing(40)),
			want: []want{{track: "A", msPlayed: 50, reason: EndStopped, check: func(t *testing.T, p Play) {
				if p.DeviceSwitches != 1 || p.DeviceName != "Phone" || p.DeviceType != "Smartphone" {
					t.Errorf("device %q/%q switches %d", p.DeviceName, p.DeviceType, p.DeviceSwitches)
				}
			}}},
		},
		{
			name: "ad between tracks is a gap",
			snaps: seq(on(0, "A", 200, 188), other(13, KindAd, 1), // A ended at 12, then the ad
				other(25, KindAd, 13), on(42, "B", 180, 1)),
			want: []want{{track: "A", msPlayed: 200, reason: EndGap, skipped: false, ended: f(12)}},
			open: "B",
		},
		{
			name: "podcast episode is a gap and never a play (D11)",
			snaps: seq(on(0, "A", 200, 100), other(12, KindEpisode, 5), // switched at 7
				other(102, KindEpisode, 95), on(192, "B", 180, 4)),
			want: []want{{track: "A", msPlayed: 107, reason: EndGap}},
			open: "B",
		},
		{
			name:  "unknown item is a gap",
			snaps: seq(on(0, "A", 200, 100), other(12, KindUnknown, 0)),
			want:  []want{{track: "A", msPlayed: 112, reason: EndGap}},
		},
		{
			name:  "204 nothing playing ends the play",
			snaps: seq(on(0, "A", 200, 100), nothing(12)),
			want:  []want{{track: "A", msPlayed: 112, reason: EndStopped, skipped: false}},
		},
		{
			name:  "only ever seen paused: not a play",
			snaps: seq(paused(0, "A", 200, 80), paused(90, "A", 200, 80), nothing(180)),
			want:  nil,
		},
		{
			name: "paused at startup then resumed: only listening after the resume counts",
			snaps: seq(paused(0, "A", 200, 80), on(90, "A", 200, 85), on(102, "A", 200, 97),
				nothing(108)),
			want: []want{{track: "A", msPlayed: 5 + 12 + 6, reason: EndStopped}},
		},
		{
			name: "long gap (server down) between tracks: the old one finished",
			snaps: seq(on(0, "A", 200, 10),
				on(600, "B", 180, 30)), // 10 min later
			want: []want{{track: "A", msPlayed: 200, reason: EndTrackChange, skipped: false}},
			open: "B",
		},
		{
			name: "new track first seen after an idle poll gets its progress",
			snaps: seq(nothing(0), on(40, "A", 200, 25), on(52, "A", 200, 37),
				nothing(55)),
			want: []want{{track: "A", msPlayed: 25 + 12 + 3, reason: EndStopped, started: f(15)}},
		},
		{
			name: "shuffle, repeat, context, private session recorded",
			snaps: seq(on(0, "A", 200, 3).shuffle().repeat("context").ctx("spotify:playlist:p1").private(),
				on(12, "A", 200, 15), nothing(20)),
			want: []want{{track: "A", msPlayed: 3 + 12 + 8, reason: EndStopped, check: func(t *testing.T, p Play) {
				if !p.Shuffle || p.Repeat != "context" || p.ContextURI != "spotify:playlist:p1" || !p.Private ||
					p.TrackURI != "spotify:track:A" || p.DurationMs != 200_000 || string(p.Raw) != `{"t":12}` {
					t.Errorf("fields %+v", p)
				}
			}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plays, m := run(c.snaps)
			if len(plays) != len(c.want) {
				t.Fatalf("got %d plays, want %d: %+v", len(plays), len(c.want), plays)
			}
			for i, w := range c.want {
				p := plays[i]
				if p.TrackID != w.track || p.MsPlayed != ms(w.msPlayed) || p.Skipped != w.skipped || p.EndReason != w.reason {
					t.Errorf("play %d: track %s ms %d skipped %v reason %s; want %s %d %v %s",
						i, p.TrackID, p.MsPlayed, p.Skipped, p.EndReason, w.track, ms(w.msPlayed), w.skipped, w.reason)
				}
				if w.started != nil && p.StartedMs != ms(*w.started) {
					t.Errorf("play %d: started %d, want %d", i, p.StartedMs, ms(*w.started))
				}
				if w.ended != nil && p.EndedMs != ms(*w.ended) {
					t.Errorf("play %d: ended %d, want %d", i, p.EndedMs, ms(*w.ended))
				}
				if p.EndedMs < p.StartedMs || p.FirstSeenMs > p.LastSeenMs {
					t.Errorf("play %d: inconsistent times %+v", i, p)
				}
				if w.check != nil {
					w.check(t, p)
				}
			}
			open := ""
			if o := m.Current(); o != nil {
				open = o.TrackID
			}
			if open != c.open {
				t.Errorf("open play %q, want %q", open, c.open)
			}
		})
	}
}

// Persisting the state between any two snapshots (a restart) gives the
// same plays as an uninterrupted run.
func TestMachineStateRoundTrip(t *testing.T) {
	snaps, _ := steady(0, "A", 200, 3)
	snaps = append(snaps, seq(on(198.5, "B", 180, 1.5), paused(204, "B", 180, 5), on(300, "B", 180, 9),
		on(312, "B", 180, 21).dev("Phone", "Smartphone"), on(320, "C", 100, 2))...)
	want, _ := run(snaps)
	for cut := 1; cut < len(snaps); cut++ {
		m := NewMachine(Config{}, MachineState{})
		var got []Play
		for i, s := range snaps {
			if i == cut {
				b, err := json.Marshal(m.State())
				if err != nil {
					t.Fatal(err)
				}
				var st MachineState
				if err := json.Unmarshal(b, &st); err != nil {
					t.Fatal(err)
				}
				m = NewMachine(Config{}, st)
			}
			got = append(got, m.Observe(s)...)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("cut at %d:\n got %+v\nwant %+v", cut, got, want)
		}
	}
}

func TestNextPoll(t *testing.T) {
	iv := DefaultIntervals
	for name, c := range map[string]struct {
		s    Snapshot
		want time.Duration
	}{
		"playing mid-track":    {on(0, "A", 200, 50).snap(), 12 * time.Second},
		"playing near the end": {on(0, "A", 200, 195).snap(), 6500 * time.Millisecond},
		"at the very end":      {on(0, "A", 200, 200).snap(), 2 * time.Second},
		"paused":               {paused(0, "A", 200, 50).snap(), 90 * time.Second},
		"nothing":              {nothing(0).snap(), 90 * time.Second},
		"ad":                   {other(0, KindAd, 3).snap(), 12 * time.Second},
		"episode":              {other(0, KindEpisode, 3).snap(), 90 * time.Second},
	} {
		if got := NextPoll(c.s, iv); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}
