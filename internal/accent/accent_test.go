package accent

import "testing"

func TestParse(t *testing.T) {
	for _, value := range []string{"amber", "f2ab3e", "#fff", "#f2ab3eff", "#zzzzzz", "#123456 "} {
		if _, err := Parse(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	for _, value := range []string{"", "#1DB954"} {
		if seed, err := Parse(value); err != nil || seed.CSS() != "" {
			t.Errorf("built-in green %q should not emit overrides: %v", value, err)
		}
	}
}

func TestDerivedContrast(t *testing.T) {
	// Check a grid including the sRGB gamut boundaries and the gray axis.
	for r := 0; r <= 255; r += 17 {
		for g := 0; g <= 255; g += 17 {
			for b := 0; b <= 255; b += 17 {
				seed := Seed{rgb: [3]byte{byte(r), byte(g), byte(b)}, set: true}
				p := seed.palette()
				for _, check := range []struct {
					c           rgb8
					bg, minimum float64
					dark        bool
				}{
					{p.lightSignal, daySurface, 3, false},
					{p.lightInk, daySurface, 4.5, false},
					{p.darkSignal, nightSurface, 3, true},
					{p.darkInk, nightSurface, 4.5, true},
				} {
					a, b := check.c.linear().luminance()+0.05, check.bg+0.05
					ratio := b / a
					if check.dark {
						ratio = a / b
					}
					if ratio < check.minimum {
						t.Fatalf("seed %s, shade %s: contrast %.3f < %.1f", rgb8(seed.rgb), check.c, ratio, check.minimum)
					}
				}
			}
		}
	}
}

func TestAmberStaysWarm(t *testing.T) {
	seed, err := Parse("#F2AB3E")
	if err != nil {
		t.Fatal(err)
	}
	p := seed.palette()
	for _, c := range []rgb8{p.lightSignal, p.lightInk, p.darkSignal, p.darkInk} {
		if !(c[0] > c[1] && c[1] > c[2]) {
			t.Errorf("expected warm amber, got %s", c)
		}
	}
}
