package refdata

import (
	"math/rand/v2"
	"strings"
)

// Word lists for synthetic names. Everything is invented; nothing refers to
// real artists or releases.
var (
	adjectives = strings.Fields(`velvet golden silent neon paper broken quiet hollow electric crystal
		wild lunar amber restless distant faded burning frozen hidden midnight
		scarlet silver gentle savage lonely cosmic rusty violet endless sleepy
		bitter tender northern southern secret wandering fragile heavy bright
		dusty gilded crimson pale static early late empty sacred tangled
		weightless honest strange lucky crooked salty radiant shallow cobalt`)
	nouns = strings.Fields(`orchid harbor signal engine river garden mirror lantern ocean forest
		machine echo comet window season shadow canyon ember feather horizon
		satellite island highway thunder ribbon circuit meadow tide prism atlas
		rocket compass cathedral postcard balcony anthem cassette mountain desert
		orbit paradise lighthouse avalanche monsoon carousel voltage parade
		velocity citadel frontier glacier mosaic pavilion reverie skyline`)
	verbs = strings.Fields(`falling running dreaming burning waiting dancing breathing drifting
		calling chasing fading holding leaving shining sinking turning`)
	firstNames = strings.Fields(`mara lio juno ezra noor ivo saga tove remi otis
		lena kai nina felix ada milo iris theo vera hugo
		zara luca elin bram cleo aksel yara oren lumi tess`)
	syllables = strings.Fields(`ka ri mo len sa vi to na el ar is do ne lu ma
		ko ve ra ti on al be zu fe ga ho ny qui sel dar mon`)
	familyNames = []string{"pop", "rock", "indie", "electronic", "hip hop", "r&b", "jazz", "classical",
		"metal", "folk", "ambient", "latin", "country", "punk", "soul", "house"}
	genreModifiers = strings.Fields(`indie dark nu post dream chill melodic alt art deep
		lo-fi neo progressive acoustic experimental modern classic vapor future
		garage dub minimal noise psych swedish german uk nordic french dutch
		japanese brazilian atlantic coastal underground bedroom stomp math`)
	playlistWords = strings.Fields(`focus running chill commute sunday mornings late night roadtrip
		workout dinner rainy days throwbacks summer winter gym coding study
		party sleep kitchen discoveries favourites`)
)

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.IntN(len(xs))] }

func title(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		if i > 0 && (w == "of" || w == "the" || w == "and" || w == "in") {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func madeUpWord(r *rand.Rand) string {
	n := 2 + r.IntN(2)
	var b strings.Builder
	for range n {
		b.WriteString(pick(r, syllables))
	}
	return b.String()
}

func artistName(r *rand.Rand) string {
	switch r.IntN(7) {
	case 0:
		return title("the " + pick(r, adjectives) + " " + pick(r, nouns) + "s")
	case 1:
		return title(pick(r, firstNames) + " " + madeUpWord(r))
	case 2:
		return title(madeUpWord(r))
	case 3:
		return title(pick(r, adjectives) + " " + pick(r, nouns))
	case 4:
		return "DJ " + title(madeUpWord(r))
	case 5:
		return title(pick(r, nouns) + " " + pick(r, nouns))
	default:
		return title(pick(r, firstNames) + " & the " + pick(r, nouns) + "s")
	}
}

func albumName(r *rand.Rand) string {
	switch r.IntN(5) {
	case 0:
		return title(pick(r, adjectives) + " " + pick(r, nouns))
	case 1:
		return title("the " + pick(r, nouns) + " of " + pick(r, nouns))
	case 2:
		return title(pick(r, verbs) + " " + pick(r, nouns) + "s")
	case 3:
		return title(madeUpWord(r))
	default:
		return title(pick(r, nouns) + "s")
	}
}

func trackName(r *rand.Rand) string {
	switch r.IntN(6) {
	case 0:
		return title(pick(r, adjectives) + " " + pick(r, nouns))
	case 1:
		return title(pick(r, verbs))
	case 2:
		return title(pick(r, verbs) + " " + pick(r, adjectives))
	case 3:
		return title(pick(r, nouns))
	case 4:
		return title("the " + pick(r, adjectives) + " " + pick(r, nouns))
	default:
		return title(pick(r, nouns) + " in the " + pick(r, nouns))
	}
}

func playlistName(r *rand.Rand) string {
	switch r.IntN(4) {
	case 0:
		return title(pick(r, playlistWords) + " " + pick(r, playlistWords))
	case 1:
		return title(pick(r, adjectives) + " " + pick(r, playlistWords))
	case 2:
		return title(pick(r, playlistWords))
	default:
		return title(pick(r, nouns) + " mix")
	}
}

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// spotifyID returns a random 22-character base62 ID, the Spotify ID shape.
func spotifyID(r *rand.Rand) string {
	var b [22]byte
	for i := range b {
		b[i] = base62[r.IntN(62)]
	}
	return string(b[:])
}

// slug makes a stable genre ID.
func slug(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer("&", "n", " ", "-", "'", "").Replace(s)
	return s
}
