package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// spaFile is one embedded UI file, held in memory with its precompressed
// variants (adapter-static writes .br and .gz next to each asset).
type spaFile struct {
	body, br, gz []byte
	ctype        string
	etag         string
}

type spaHandler struct {
	files map[string]*spaFile // keyed by path without leading slash
	index *spaFile
}

var extraTypes = map[string]string{
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".html":        "text/html; charset=utf-8",
	".svg":         "image/svg+xml",
	".json":        "application/json",
	".webmanifest": "application/manifest+json",
	".woff2":       "font/woff2",
	".woff":        "font/woff",
	".ico":         "image/x-icon",
	".txt":         "text/plain; charset=utf-8",
	".png":         "image/png",
	".webp":        "image/webp",
}

const placeholderHTML = `<!doctype html><meta charset="utf-8"><title>spotussi</title>
<p>The web UI is not built into this binary. Run <code>make build</code> (or <code>pnpm --dir web build</code>, then rebuild the Go binary).</p>`

// newSPAHandler serves the embedded single-page app from fsys (the
// adapter-static output). Unknown paths without a file extension fall back
// to index.html so client-side routes work on reload; unknown paths with an
// extension are real 404s. Hashed assets under _app/immutable are cached for
// a year; everything else must revalidate.
func newSPAHandler(fsys fs.FS) http.Handler {
	h := &spaHandler{files: map[string]*spaFile{}}
	if fsys != nil {
		_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.HasSuffix(p, ".br") || strings.HasSuffix(p, ".gz") || path.Base(p) == ".gitkeep" {
				return nil
			}
			body, err := fs.ReadFile(fsys, p)
			if err != nil {
				return nil
			}
			f := &spaFile{body: body, ctype: contentType(p)}
			if b, err := fs.ReadFile(fsys, p+".br"); err == nil {
				f.br = b
			}
			if b, err := fs.ReadFile(fsys, p+".gz"); err == nil {
				f.gz = b
			}
			sum := sha256.Sum256(body)
			f.etag = `"` + base64.RawURLEncoding.EncodeToString(sum[:12]) + `"`
			h.files[p] = f
			return nil
		})
	}
	h.index = h.files["index.html"]
	if h.index == nil {
		h.index = &spaFile{body: []byte(placeholderHTML), ctype: extraTypes[".html"], etag: `"placeholder"`}
	}
	return h
}

func contentType(p string) string {
	ext := strings.ToLower(path.Ext(p))
	if t, ok := extraTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	f, ok := h.files[p]
	cache := "public, max-age=3600"
	switch {
	case ok && strings.HasPrefix(p, "_app/immutable/"):
		cache = "public, max-age=31536000, immutable"
	case ok && p != "index.html":
	case !ok && path.Ext(p) != "":
		http.NotFound(w, r)
		return
	default: // index.html and SPA fallback
		f, cache = h.index, "no-cache"
	}
	hd := w.Header()
	hd.Set("Content-Type", f.ctype)
	hd.Set("Cache-Control", cache)
	hd.Set("ETag", f.etag)
	hd.Set("X-Content-Type-Options", "nosniff")
	if f.br != nil || f.gz != nil {
		hd.Add("Vary", "Accept-Encoding")
	}
	if etagMatches(r.Header.Get("If-None-Match"), f.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := f.body
	switch {
	case f.br != nil && acceptsEncoding(r, "br"):
		hd.Set("Content-Encoding", "br")
		body = f.br
	case f.gz != nil && acceptsEncoding(r, "gzip"):
		hd.Set("Content-Encoding", "gzip")
		body = f.gz
	}
	hd.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = bytes.NewReader(body).WriteTo(w)
	}
}
