package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func doGet(t *testing.T, h http.Handler, path string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSPAServing(t *testing.T) {
	ui := fstest.MapFS{
		"index.html":                         {Data: []byte("<html>app</html>")},
		"_app/immutable/entry/app.abc.js":    {Data: []byte("console.log(1)")},
		"_app/immutable/entry/app.abc.js.br": {Data: []byte("BR")},
		"robots.txt":                         {Data: []byte("User-agent: *")},
	}
	h := NewHandler(Options{Version: "t", Mode: "live", UI: ui})

	for _, p := range []string{"/", "/artists/abc", "/index.html", "/stats/deep/link"} {
		rec := doGet(t, h, p)
		if rec.Code != 200 || rec.Body.String() != "<html>app</html>" || rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d %q cache=%q", p, rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
		}
	}
	rec := doGet(t, h, "/_app/immutable/entry/app.abc.js")
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") {
		t.Errorf("asset: %d %v", rec.Code, rec.Header())
	}
	rec = doGet(t, h, "/_app/immutable/entry/app.abc.js", "Accept-Encoding", "gzip, br")
	if rec.Header().Get("Content-Encoding") != "br" || rec.Body.String() != "BR" {
		t.Errorf("precompressed: %v %q", rec.Header(), rec.Body.String())
	}
	if rec := doGet(t, h, "/missing.js"); rec.Code != 404 {
		t.Errorf("missing asset: %d", rec.Code)
	}
	etag := doGet(t, h, "/").Header().Get("ETag")
	if rec := doGet(t, h, "/", "If-None-Match", etag); rec.Code != 304 {
		t.Errorf("index revalidation: %d", rec.Code)
	}
}

func TestPlaceholderWithoutUI(t *testing.T) {
	h := NewHandler(Options{Version: "t", Mode: "live"})
	if rec := doGet(t, h, "/"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "not built") {
		t.Errorf("placeholder: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLiveModeAPI(t *testing.T) {
	h := NewHandler(Options{Version: "1.2.3", Mode: "live"})
	rec := doGet(t, h, "/api/v1/health")
	if rec.Code != 200 || rec.Body.String() != `{"status":"ok","version":"1.2.3","mode":"live"}` {
		t.Errorf("health: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doGet(t, h, "/api/v1/top/tracks"); rec.Code != 501 || !strings.Contains(rec.Body.String(), "not_implemented") {
		t.Errorf("unimplemented: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAPIDocs(t *testing.T) {
	h := NewHandler(Options{Version: "t", Mode: "live"})
	rec := doGet(t, h, "/api/docs")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `spec-url="/api/docs/openapi.yaml"`) ||
		!strings.Contains(rec.Body.String(), `integrity="sha384-`) {
		t.Errorf("viewer: %d %s", rec.Code, rec.Body.String())
	}
	rec = doGet(t, h, "/api/docs/openapi.yaml")
	if rec.Code != 200 || !strings.HasPrefix(rec.Body.String(), "openapi: 3.1") ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "application/yaml") {
		t.Errorf("spec: %d %.40q", rec.Code, rec.Body.String())
	}
}

func TestWriteJSONGzipAndETag(t *testing.T) {
	big := map[string]string{"x": strings.Repeat("a", 4000)}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, r, 200, big) })
	rec := doGet(t, h, "/", "Accept-Encoding", "gzip")
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip: %v", rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	if !strings.Contains(string(body), "aaaa") {
		t.Error("bad gzip body")
	}
	if rec := doGet(t, h, "/", "Accept-Encoding", "gzip;q=0"); rec.Header().Get("Content-Encoding") != "" {
		t.Error("gzip;q=0 must disable gzip")
	}
	etag := rec.Header().Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("etag %q", etag)
	}
	for _, inm := range []string{etag, `"other", ` + etag, strings.TrimPrefix(etag, "W/"), "*"} {
		if rec := doGet(t, h, "/", "If-None-Match", inm); rec.Code != 304 {
			t.Errorf("If-None-Match %q: %d", inm, rec.Code)
		}
	}
}
