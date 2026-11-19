// Package accent derives the web UI's highlight shades from a deployment seed.
package accent

import (
	"encoding/hex"
	"fmt"
	"math"
)

// Seed is an opaque sRGB color. Its zero value keeps the built-in green palette.
type Seed struct {
	rgb [3]byte
	set bool
}

// Parse accepts an empty value or a color in #RRGGBB notation.
func Parse(value string) (Seed, error) {
	if value == "" {
		return Seed{}, nil
	}
	if len(value) != 7 || value[0] != '#' {
		return Seed{}, fmt.Errorf("must be a hex color in #RRGGBB format, got %q", value)
	}
	seed := Seed{set: true}
	if _, err := hex.Decode(seed.rgb[:], []byte(value[1:])); err != nil {
		return Seed{}, fmt.Errorf("must be a hex color in #RRGGBB format, got %q", value)
	}
	return seed, nil
}

// CSS returns the document-level overrides. The built-in green is left as-is,
// including when its seed is set explicitly.
func (s Seed) CSS() string {
	if !s.set || s.rgb == [3]byte{0x1d, 0xb9, 0x54} {
		return ""
	}
	p := s.palette()
	return fmt.Sprintf(":root{--instance-signal-light:%s;--instance-signal-ink-light:%s;--instance-signal-dark:%s;--instance-signal-ink-dark:%s}",
		p.lightSignal, p.lightInk, p.darkSignal, p.darkInk)
}

type palette struct {
	lightSignal, lightInk, darkSignal, darkInk rgb8
}

func (s Seed) palette() palette {
	l, a, b := toOKLab(rgb8(s.rgb).linear())
	// Use the stock green's role lightnesses as anchors. Bound the seed
	// lightness so even a very dark or light seed gives useful shades.
	base := math.Max(0.62, math.Min(0.77, l))
	return palette{
		lightSignal: shade(base*0.607/0.689, a/base, b/base, daySurface, 3, false),
		lightInk:    shade(base*0.508/0.689, a/base, b/base, daySurface, 4.5, false),
		darkSignal:  shade(base, a/base, b/base, nightSurface, 3, true),
		darkInk:     shade(base*0.77/0.689, a/base, b/base, nightSurface, 4.5, true),
	}
}

// Light and dark --accent in web/src/routes/layout.css. They also bound
// contrast on the page, card, popover and (for dark) onboarding dial.
var (
	daySurface   = surface(0.925, 0.005, 265)
	nightSurface = surface(0.27, 0.008, 265)
)

func surface(l, c, h float64) float64 {
	sin, cos := math.Sincos(h * math.Pi / 180)
	return fromOKLab(l, c*cos, c*sin).luminance()
}

func shade(wanted, a, b, background, minimum float64, dark bool) rgb8 {
	at := func(l float64) rgb8 { return inGamut(l, a*l, b*l) }
	passes := func(color rgb8) bool {
		y := color.linear().luminance() + 0.05
		if dark {
			return y/(background+0.05) >= minimum
		}
		return (background+0.05)/y >= minimum
	}
	// Test the serialized (8-bit) result instead of a higher-precision color
	// that could fall below the contrast floor after rounding.
	if initial := at(wanted); passes(initial) {
		return initial
	}
	fail, pass := wanted, 0.0
	if dark {
		pass = 1
	}
	result := at(pass)
	for range 16 {
		mid := (fail + pass) / 2
		if candidate := at(mid); passes(candidate) {
			pass, result = mid, candidate
		} else {
			fail = mid
		}
	}
	return result
}

func inGamut(l, a, b float64) rgb8 {
	if rgb := fromOKLab(l, a, b); rgb.inGamut() {
		return rgb.encode()
	}
	// Reduce chroma at fixed hue and lightness instead of clipping channels.
	low, high := 0.0, 1.0
	for range 16 {
		mid := (low + high) / 2
		if fromOKLab(l, a*mid, b*mid).inGamut() {
			low = mid
		} else {
			high = mid
		}
	}
	return fromOKLab(l, a*low, b*low).encode()
}

type rgb8 [3]byte

type linearRGB [3]float64

func (c rgb8) String() string { return "#" + hex.EncodeToString(c[:]) }

func (c rgb8) linear() linearRGB {
	var result linearRGB
	for i, v := range c {
		x := float64(v) / 255
		if x <= 0.04045 {
			result[i] = x / 12.92
		} else {
			result[i] = math.Pow((x+0.055)/1.055, 2.4)
		}
	}
	return result
}

func (c linearRGB) encode() rgb8 {
	var result rgb8
	for i, x := range c {
		if x <= 0.0031308 {
			x *= 12.92
		} else {
			x = 1.055*math.Pow(x, 1/2.4) - 0.055
		}
		result[i] = byte(math.Round(255 * x))
	}
	return result
}

func (c linearRGB) luminance() float64 { return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2] }

func (c linearRGB) inGamut() bool {
	const epsilon = 1e-7 // allow floating-point error at the gamut boundary
	for _, x := range c {
		if x < -epsilon || x > 1+epsilon {
			return false
		}
	}
	return true
}

// The standard OKLab matrices, applied to linear sRGB channels.
func toOKLab(c linearRGB) (l, a, b float64) {
	r, g, blue := c[0], c[1], c[2]
	x := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*blue)
	y := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*blue)
	z := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*blue)
	return 0.2104542553*x + 0.793617785*y - 0.0040720468*z,
		1.9779984951*x - 2.428592205*y + 0.4505937099*z,
		0.0259040371*x + 0.7827717662*y - 0.808675766*z
}

func fromOKLab(l, a, b float64) linearRGB {
	x, y, z := l+0.3963377774*a+0.2158037573*b, l-0.1055613458*a-0.0638541728*b, l-0.0894841775*a-1.291485548*b
	x, y, z = x*x*x, y*y*y, z*z*z
	return linearRGB{
		4.0767416621*x - 3.3077115913*y + 0.2309699292*z,
		-1.2684380046*x + 2.6097574011*y - 0.3413193965*z,
		-0.0041960863*x - 0.7034186147*y + 1.707614701*z,
	}
}
