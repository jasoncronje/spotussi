// Package web embeds the built single-page app (SvelteKit adapter-static
// output in build/spa) into the Go binary (D6).
//
// build/.gitkeep is committed so this package compiles before the UI has
// been built; the server then serves a short "UI not built" page.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var build embed.FS

// UI returns the built SPA, or nil if it has not been built.
func UI() fs.FS {
	sub, err := fs.Sub(build, "build/spa")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
