package metadata

import (
	"math"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Name matching for the sources that are searched by name (Deezer,
// MusicBrainz name search). Names are compared after normalization:
// accents folded, lower case, punctuation as spaces, "&" as "and".
// Titles are also compared without their qualifiers ("(feat. …)",
// "- Remastered 2011", "(Deluxe Edition)"), but version words (live,
// remix, acoustic, …) that only one side has count against a match.

// Fold lower-cases s, folds accents and turns punctuation into spaces.
func Fold(s string) string {
	s = strings.ReplaceAll(norm.NFKD.String(s), "&", " and ")
	var b strings.Builder
	space := true
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			space = false
		case r == '\'' || r == '’' || r == '.':
			// "don't" and "dont", "m.i.a." and "mia" are one word
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

var (
	// bracketed parts and " - " suffixes are title qualifiers
	bracketRe = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
	dashRe    = regexp.MustCompile(`\s+[-–—]\s+.*$`)
	featRe    = regexp.MustCompile(`(?i)\s+(feat\.?|ft\.?|featuring)\s+.*$`)
	// noise inside qualifiers that doesn't change the recording
	noiseWords = map[string]bool{"remaster": true, "remastered": true, "remasterizado": true, "deluxe": true,
		"edition": true, "expanded": true, "anniversary": true, "bonus": true, "track": true, "tracks": true,
		"version": true, "mono": true, "stereo": true, "original": true, "single": true, "album": true,
		"feat": true, "ft": true, "featuring": true, "with": true, "from": true, "the": true, "of": true,
		"digital": true, "reissue": true, "special": true, "explicit": true, "clean": true, "and": true}
	// words that mark another recording when only one side has them
	versionWords = []string{"live", "remix", "rmx", "acoustic", "instrumental", "demo", "karaoke", "cover",
		"unplugged", "edit", "mix", "dub", "orchestral", "piano", "sped", "slowed", "reverb", "nightcore",
		"acapella", "a cappella", "rework", "session", "bootleg", "tribute"}
)

// StripQualifiers removes a title's bracketed parts, " - …" suffix and
// "feat. …" credits (for search queries).
func StripQualifiers(title string) string {
	s := bracketRe.ReplaceAllString(title, "")
	s = dashRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(featRe.ReplaceAllString(s, ""))
	if Fold(s) == "" {
		return strings.TrimSpace(title)
	}
	return s
}

// Core is a title without its qualifiers, folded.
func Core(title string) string { return Fold(StripQualifiers(title)) }

// versions are the version words a title's qualifiers carry.
func versions(title string) map[string]bool {
	f := " " + Fold(title) + " "
	out := map[string]bool{}
	for _, w := range versionWords {
		if strings.Contains(f, " "+w+" ") {
			out[w] = true
		}
	}
	return out
}

func tokens(s string, dropNoise bool) map[string]bool {
	out := map[string]bool{}
	for _, t := range strings.Fields(s) {
		if dropNoise && (noiseWords[t] || isYear(t)) {
			continue
		}
		out[t] = true
	}
	return out
}

func isYear(t string) bool {
	if len(t) != 4 || (t[0] != '1' && t[0] != '2') {
		return false
	}
	for _, c := range t {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := 0
	for t := range a {
		if b[t] {
			n++
		}
	}
	return float64(n) / float64(len(a)+len(b)-n)
}

// TitleSim scores two track or album titles from 0 to 1: 1 when equal
// after folding (or equal apart from noise like "Remastered 2011"), 0.9
// when the cores match, less for partial overlaps, and 0.3 off when one
// side is a live, remix, acoustic … version and the other isn't.
func TitleSim(a, b string) float64 {
	fa, fb := Fold(a), Fold(b)
	if fa == "" || fb == "" {
		return 0
	}
	var s float64
	switch ca, cb := Core(a), Core(b); {
	case fa == fb:
		return 1
	case ca == cb:
		s = 0.9
		if ta, tb := tokens(fa, true), tokens(fb, true); jaccard(ta, tb) == 1 {
			s = 1
		}
	default:
		s = 0.85 * jaccard(tokens(ca, false), tokens(cb, false))
	}
	va, vb := versions(a), versions(b)
	for w := range va {
		if !vb[w] {
			s -= 0.3
			break
		}
	}
	for w := range vb {
		if !va[w] {
			s -= 0.3
			break
		}
	}
	return max(s, 0)
}

// NameSim scores two artist names from 0 to 1: 1 when equal after folding
// (and without a leading "the"), 0.85 when one's words contain the
// other's (a credit of several artists, "Artist & Other"), else the word
// overlap.
func NameSim(a, b string) float64 {
	fa, fb := strings.TrimPrefix(Fold(a), "the "), strings.TrimPrefix(Fold(b), "the ")
	if fa == "" || fb == "" {
		return 0
	}
	if fa == fb || strings.ReplaceAll(fa, " ", "") == strings.ReplaceAll(fb, " ", "") {
		return 1
	}
	ta, tb := tokens(fa, false), tokens(fb, false)
	sub := func(x, y map[string]bool) bool {
		for t := range x {
			if !y[t] {
				return false
			}
		}
		return true
	}
	if sub(ta, tb) || sub(tb, ta) {
		return 0.85
	}
	return 0.8 * jaccard(ta, tb)
}

// Duration is what the export says about a track's length.
type Duration struct {
	// ExpectedMs is the median length of streams that ended because the
	// track finished (0: none).
	ExpectedMs int64
	// MinMs is the longest stream: the track is at least that long.
	MinMs int64
}

// Known reports whether a stream played the track to the end, so its
// length is known. Without one the length is a weak signal: the longest
// stream can outlast the track (seeking back, a replay counted as one
// stream), so Score's lower bound can wrongly rule a candidate out.
func (d Duration) Known() bool { return d.ExpectedMs > 0 }

// Score rates a candidate length (ms) against the export: 1 within 2.5 s
// of the expected length, less further off, 0 when impossible. 0.5 when
// nothing is known.
func (d Duration) Score(ms int64) float64 {
	if ms <= 0 {
		return 0.5
	}
	if d.ExpectedMs > 0 {
		switch diff := abs(ms - d.ExpectedMs); {
		case diff <= 2500:
			return 1
		case diff <= 6000:
			return 0.6
		case diff <= 15000:
			return 0.2
		default:
			return 0
		}
	}
	if d.MinMs > 0 && ms+3000 < d.MinMs {
		return 0 // streamed longer than the candidate lasts
	}
	return 0.5
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// TrackQuery is what a track is matched on.
type TrackQuery struct {
	Title, Artist, Album string
	// AltTitle is another title for the same track (oEmbed's).
	AltTitle string
	Duration Duration
}

// Weights of a track match. A candidate needs title and artist scores of
// at least MinPart and a total of at least MinTrackScore.
const (
	MinPart       = 0.7
	MinTrackScore = 0.8
)

// MatcherVersion changes whenever TrackScore accepts tracks it rejected
// before, so a source re-tries its earlier no-match tracks (not its
// matches). 2: a weak duration no longer rejects an exact title and
// artist match on its own (D27).
const MatcherVersion = 2

// TrackScore rates a Deezer track against q (0 rejects).
func TrackScore(q TrackQuery, t DeezerTrack) float64 {
	title := TitleSim(q.Title, t.Title)
	if q.AltTitle != "" {
		title = max(title, TitleSim(q.AltTitle, t.Title))
	}
	artist := NameSim(q.Artist, t.Artist.Name)
	dur := q.Duration.Score(int64(t.Duration) * 1000)
	if dur == 0 && !q.Duration.Known() && title == 1 && artist == 1 {
		// Only the longest stream speaks against it, and that is no proof
		// (Duration.Known): an exact title and artist match stands, with
		// the duration as unknown.
		dur = 0.5
	}
	if title < MinPart || artist < MinPart || dur == 0 {
		return 0
	}
	album := 0.5
	if q.Album != "" && t.Album.Title != "" {
		album = TitleSim(q.Album, t.Album.Title)
		if Core(q.Album) == Core(t.Album.Title) {
			album = max(album, 0.9)
		}
	}
	// Rounded so sums at a threshold (0.4+0.3+0.1 is 0.7999…) meet it.
	return math.Round((0.4*title+0.3*artist+0.1*album+0.2*dur)*1e6) / 1e6
}
