package mock

import (
	"fmt"
	"hash/fnv"
	"html"
	"net/http"
	"strings"
	"unicode"
)

// ImageHandler serves deterministic placeholder artwork for mock entities
// at ImagePath{kind}/{id}.svg: a two-colour gradient derived from the ID
// with the entity's initials.
func (b *Backend) ImageHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := r.PathValue("kind")
		id, ok := strings.CutSuffix(r.PathValue("file"), ".svg")
		if !ok {
			http.NotFound(w, r)
			return
		}
		name, found := b.imageName(kind, id)
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		fmt.Fprint(w, placeholderSVG(id, name, kind == "artist"))
	})
}

func (b *Backend) imageName(kind, id string) (string, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	switch kind {
	case "artist":
		if i, ok := b.byID.artist[id]; ok {
			return b.ds.Artists[i].Name, true
		}
	case "album":
		if i, ok := b.byID.album[id]; ok {
			return b.ds.Albums[i].Name, true
		}
	case "playlist":
		if i, ok := b.byID.playlist[id]; ok {
			return b.ds.Contexts[i].Name, true
		}
	}
	return "", false
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		r := []rune(w)
		if len(r) == 0 || !unicode.IsLetter(r[0]) || strings.EqualFold(w, "the") || w == "&" {
			continue
		}
		out = append(out, unicode.ToUpper(r[0]))
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

func placeholderSVG(id, name string, round bool) string {
	h := fnv.New64a()
	h.Write([]byte(id))
	v := h.Sum64()
	h1 := v % 360
	h2 := (h1 + 40 + (v>>16)%100) % 360
	angle := (v >> 32) % 360
	shape := `<rect width="300" height="300" fill="url(#g)"/>`
	if round {
		shape = `<circle cx="150" cy="150" r="150" fill="url(#g)"/>`
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 300" width="300" height="300">`+
		`<defs><linearGradient id="g" gradientTransform="rotate(%d .5 .5)">`+
		`<stop offset="0" stop-color="hsl(%d 65%% 55%%)"/><stop offset="1" stop-color="hsl(%d 70%% 30%%)"/>`+
		`</linearGradient></defs>%s`+
		`<text x="150" y="150" dy=".35em" text-anchor="middle" font-family="system-ui,sans-serif" font-size="110" font-weight="600" fill="rgba(255,255,255,.85)">%s</text></svg>`,
		angle, h1, h2, shape, html.EscapeString(initials(name)))
}
