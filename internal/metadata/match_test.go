package metadata

import "testing"

func TestFoldAndCore(t *testing.T) {
	for in, want := range map[string]string{
		"Beyoncé":              "beyonce",
		"  Simon & Garfunkel ": "simon and garfunkel",
		"Don't Stop Me Now":    "dont stop me now",
		"AC/DC":                "ac dc",
		"M.I.A.":               "mia",
		"Sigur Rós":            "sigur ros",
		"東京事変":                 "東京事変",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"Here Comes the Sun - Remastered 2009": "here comes the sun",
		"Song (feat. Someone)":                 "song",
		"Song feat. Someone":                   "song",
		"Stay With Me":                         "stay with me",
		"(Intro)":                              "intro",
	} {
		if got := Core(in); got != want {
			t.Errorf("Core(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTitleSim(t *testing.T) {
	cases := []struct {
		a, b   string
		lo, hi float64
	}{
		{"Never Gonna Give You Up", "Never Gonna Give You Up", 1, 1},
		{"Here Comes the Sun - Remastered 2009", "Here Comes The Sun (Remastered 2009)", 1, 1},
		{"Song - 2015 Remaster", "Song", 1, 1},
		{"Song (feat. X)", "Song", 0.9, 1},
		{"Song", "Song - Live at Wembley", 0, 0.65},
		{"Song", "Song (Acoustic)", 0, 0.65},
		{"Song - Radio Edit", "Song (Radio Edit)", 1, 1},
		{"Completely Different", "Song", 0, 0.1},
		{"Whenever You Need Somebody", "Whenever You Need Somebody (Deluxe Edition / Remastered 2022)", 1, 1},
	}
	for _, c := range cases {
		if got := TitleSim(c.a, c.b); got < c.lo || got > c.hi {
			t.Errorf("TitleSim(%q, %q) = %.2f, want [%.2f, %.2f]", c.a, c.b, got, c.lo, c.hi)
		}
	}
}

func TestNameSim(t *testing.T) {
	cases := []struct {
		a, b   string
		lo, hi float64
	}{
		{"The Beatles", "Beatles", 1, 1},
		{"JAY-Z", "Jay Z", 1, 1},
		{"Simon & Garfunkel", "Simon and Garfunkel", 1, 1},
		{"Daft Punk", "Daft Punk & Pharrell Williams", 0.85, 0.85},
		{"Rick Astley", "Barry White", 0, 0},
	}
	for _, c := range cases {
		if got := NameSim(c.a, c.b); got < c.lo || got > c.hi {
			t.Errorf("NameSim(%q, %q) = %.2f, want [%.2f, %.2f]", c.a, c.b, got, c.lo, c.hi)
		}
	}
}

func TestDurationScore(t *testing.T) {
	d := Duration{ExpectedMs: 213573, MinMs: 213600}
	if d.Score(213000) != 1 || d.Score(218000) != 0.6 || d.Score(347000) != 0 {
		t.Error("expected-length scores")
	}
	lb := Duration{MinMs: 300000}
	if lb.Score(213000) != 0 || lb.Score(310000) != 0.5 {
		t.Error("lower-bound scores")
	}
	if (Duration{}).Score(1000) != 0.5 || d.Score(0) != 0.5 {
		t.Error("unknown scores")
	}
}

func TestTrackScore(t *testing.T) {
	q := TrackQuery{Title: "Never Gonna Give You Up", Artist: "Rick Astley", Album: "Whenever You Need Somebody",
		Duration: Duration{ExpectedMs: 213573}}
	right := DeezerTrack{Title: "Never Gonna Give You Up", Duration: 213, Artist: DeezerArtist{Name: "Rick Astley"},
		Album: DeezerAlbum{Title: "Whenever You Need Somebody"}}
	if s := TrackScore(q, right); s < 0.99 {
		t.Errorf("exact: %.2f", s)
	}
	compilation := right
	compilation.Album.Title = "The Best of Me"
	if s := TrackScore(q, compilation); s < MinTrackScore {
		t.Errorf("same recording on a compilation: %.2f", s)
	}
	remix := right
	remix.Title, remix.Duration = "Never Gonna Give You Up (Cake Mix)", 347
	if s := TrackScore(q, remix); s != 0 {
		t.Errorf("remix of another length: %.2f", s)
	}
	cover := right
	cover.Artist.Name = "Barry White"
	if s := TrackScore(q, cover); s != 0 {
		t.Errorf("other artist: %.2f", s)
	}
	q.Duration = Duration{}
	if s := TrackScore(q, right); s < MinTrackScore {
		t.Errorf("unknown length, exact names: %.2f", s)
	}
}

// Without a stream that played to the end, the longest stream (which can
// outlast the track) no longer rejects an exact title and artist match on
// its own; anything less than exact still needs a plausible length, and
// a known length keeps rejecting.
func TestTrackScoreWeakDuration(t *testing.T) {
	weak := Duration{MinMs: 400000} // streamed 6:40 of a 3:33 track
	right := DeezerTrack{Title: "Never Gonna Give You Up", Duration: 213, Artist: DeezerArtist{Name: "Rick Astley"},
		Album: DeezerAlbum{Title: "Some Compilation"}}
	q := TrackQuery{Title: "Never Gonna Give You Up", Artist: "Rick Astley", Album: "Whenever You Need Somebody", Duration: weak}
	if s := TrackScore(q, right); s < MinTrackScore {
		t.Errorf("exact names, weak duration: %.2f", s)
	}
	noAlbum := q
	noAlbum.Album = ""
	if s := TrackScore(noAlbum, right); s < MinTrackScore {
		t.Errorf("exact names, weak duration, no album: %.2f", s)
	}
	near := q
	near.Title = "Never Gonna Give You Up (Live)"
	if s := TrackScore(near, right); s != 0 {
		t.Errorf("inexact title, weak duration: %.2f", s)
	}
	credit := q
	credit.Artist = "Rick Astley & Friends"
	if s := TrackScore(credit, right); s != 0 {
		t.Errorf("inexact artist, weak duration: %.2f", s)
	}
	known := q
	known.Duration = Duration{ExpectedMs: 400000, MinMs: 400000}
	if s := TrackScore(known, right); s != 0 {
		t.Errorf("exact names, known other length: %.2f", s)
	}
}
