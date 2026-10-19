//go:build race

package live_test

import "spotussi/internal/refdata"

// The differential tests use the tiny dataset under the race detector,
// where generating and loading the small one takes minutes; plain runs
// cover the small one.
const diffSize = refdata.Tiny
