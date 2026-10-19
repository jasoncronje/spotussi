//go:build !unix

package db

import "os"

// lockFile is a no-op where flock is unavailable (no release targets
// these platforms; restore then relies on the operator stopping the
// server first).
func lockFile(path string, exclusive bool) (*os.File, error) { return nil, nil }
