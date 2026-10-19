package api

import (
	"net/http"

	apispec "spotussi/api"
)

// API reference (D16): the embedded OpenAPI document and a Redoc viewer
// for it. Both sit under /api/, so they need a session or a token like
// the API itself.

// redocURL is pinned with subresource integrity.
const (
	redocURL = "https://cdn.jsdelivr.net/npm/redoc@2.1.5/bundles/redoc.standalone.js"
	redocSRI = "sha384-0GrsyTQc9Oqd8h+b2dbc4XdR2T/DYpy0tLNNstyx+LBMUyiBbcWPbEs9aRmUcaxD"
)

const docsPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>spotussi API</title>
<style>body { margin: 0; }</style>
</head>
<body>
<redoc spec-url="/api/docs/openapi.yaml"></redoc>
<noscript>The API reference needs JavaScript; the raw spec is at <a href="/api/docs/openapi.yaml">/api/docs/openapi.yaml</a>.</noscript>
<script src="` + redocURL + `" integrity="` + redocSRI + `" crossorigin="anonymous"></script>
</body>
</html>
`

func registerDocs(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte(docsPage)) //nolint:errcheck // client went away
	})
	mux.HandleFunc("GET /api/docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(apispec.OpenAPI) //nolint:errcheck // client went away
	})
}
