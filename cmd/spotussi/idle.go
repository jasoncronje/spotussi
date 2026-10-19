package main

import (
	"context"
	"net/http"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// idleRelease returns memory to the OS once requests have stopped for
// quiet. Serving builds garbage, and the Go runtime keeps the heap at
// its GC goal (about twice the live heap) until the next collection,
// which an idle server never reaches; the idle memory budget (< 60 MB,
// D25) is measured after the dashboard has been used.
type idleRelease struct {
	last  atomic.Int64 // unix nano of the latest request; 0 once released
	quiet time.Duration
}

func (r *idleRelease) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.last.Store(time.Now().UnixNano())
		h.ServeHTTP(w, req)
	})
}

func (r *idleRelease) run(ctx context.Context) {
	t := time.NewTicker(r.quiet / 2)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			last := r.last.Load()
			if last != 0 && now.Sub(time.Unix(0, last)) >= r.quiet && r.last.CompareAndSwap(last, 0) {
				debug.FreeOSMemory()
			}
		}
	}
}
