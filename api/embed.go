// Package apispec embeds the OpenAPI document, so the binary serves the
// contract it was built with at /api/docs (D16).
package apispec

import _ "embed"

// OpenAPI is api/openapi.yaml.
//
//go:embed openapi.yaml
var OpenAPI []byte
