//go:build spike

// The M4 spike against the real Spotify Web API (D9, D17): playlist
// lookups (own private, own public, Spotify-made), which catalog fields
// development-mode apps still get, and the request rate one burst
// sustains. It needs a data directory with a connected Spotify account
// (a local server with CAPTURE_ENABLED=false, connected once) and sends
// about 160 requests, which count against the developer app's shared
// quota. It prints status codes, counts and field presence only, never
// names or other listening data.
//
//	SPIKE_DATA_DIR=./data mise exec -- go test -tags spike -run Spike -v ./internal/hydrate/
//
// SPIKE_QUOTA=N instead counts how many track lookups one quota window
// allows (up to N); run it just after the daily reset.
package hydrate_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"spotussi/internal/db"
	"spotussi/internal/keyring"
	"spotussi/internal/spotify"
)

func TestSpike(t *testing.T) {
	dir := os.Getenv("SPIKE_DATA_DIR")
	if dir == "" {
		t.Skip("SPIKE_DATA_DIR not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	keys, err := keyring.LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := spotify.New(spotify.Options{DB: d, Keyring: keys,
		Limiter: spotify.NewLimiter(spotify.LimiterConfig{Rate: 50, Burst: 50}, nil)})
	sent := 0
	var mu sync.Mutex
	get := func(path string) (int, map[string]any, http.Header) {
		mu.Lock()
		sent++
		mu.Unlock()
		// A 429 pauses our limiter for Retry-After, which can be long:
		// give up on a request after 20 s instead of hanging.
		rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		resp, err := c.Do(rctx, http.MethodGet, path)
		var rl *spotify.RateLimitedError
		if errors.As(err, &rl) {
			t.Logf("429 on %s: Retry-After %s, reason %q", strings.SplitN(path, "?", 2)[0][:min(len(path), 9)], rl.RetryAfter, rl.Reason)
			return 429, nil, http.Header{"Retry-After": {rl.RetryAfter.String()}, "Reason": {rl.Reason}}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("request blocked by the limiter for 20 s (paused after a 429; stats %+v)", c.LimiterStats())
		}
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return resp.StatusCode, m, resp.Header
	}
	keysOf := func(m map[string]any) string {
		var ks []string
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return strings.Join(ks, ",")
	}
	errMsg := func(m map[string]any) string {
		if e, ok := m["error"].(map[string]any); ok {
			s, _ := e["message"].(string)
			return s
		}
		return ""
	}

	st, err := c.Status(ctx)
	if err != nil || st.State != spotify.StateConnected {
		t.Fatalf("not connected: %v %v", st, err)
	}
	t.Logf("scopes: %s", strings.Join(st.Scopes, " "))
	if os.Getenv("SPIKE_PROBE") != "" {
		// One request per endpoint family, each through its own limiter
		// (a 429 pauses a limiter): which ones does a quota cover?
		for _, path := range strings.Split(os.Getenv("SPIKE_PROBE"), ",") {
			pc := spotify.New(spotify.Options{DB: d, Keyring: keys,
				Limiter: spotify.NewLimiter(spotify.LimiterConfig{Rate: 50, Burst: 50}, nil)})
			resp, err := pc.Do(ctx, http.MethodGet, path)
			var rl *spotify.RateLimitedError
			switch {
			case errors.As(err, &rl):
				t.Logf("probe %s: 429 Retry-After %s reason %q", path, rl.RetryAfter, rl.Reason)
			case err != nil:
				t.Logf("probe %s: %v", path, err)
			default:
				resp.Body.Close()
				t.Logf("probe %s: %d", path, resp.StatusCode)
			}
		}
		return
	}
	if n, _ := strconv.Atoi(os.Getenv("SPIKE_QUOTA")); n > 0 {
		// Quota size: sequential track lookups at 10/s until the first
		// QUOTA_EXCEEDED (or n lookups). Run it right after a reset so the
		// window is whole. Plain rate-limit 429s are waited out, not counted.
		qc := spotify.New(spotify.Options{DB: d, Keyring: keys,
			Limiter: spotify.NewLimiter(spotify.LimiterConfig{Rate: 10, Burst: 10}, nil)})
		const track = "11dFghVXANMlKmJXsNCbNl"
		ok, other, rate429 := 0, 0, 0
		start := time.Now()
		for ok+other < n {
			resp, err := qc.Do(ctx, http.MethodGet, "/tracks/"+track)
			var rl *spotify.RateLimitedError
			if errors.As(err, &rl) {
				if rl.QuotaExceeded {
					t.Logf("quota: %d lookups succeeded (%d other statuses) in %s, then QUOTA_EXCEEDED, Retry-After %s",
						ok, other, time.Since(start).Round(time.Second), rl.RetryAfter)
					return
				}
				rate429++
				continue
			}
			if err != nil {
				t.Fatalf("after %d lookups: %v", ok, err)
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ok++
			} else {
				other++
			}
			if (ok+other)%500 == 0 {
				t.Logf("quota: %d lookups so far (%d rate-limit 429s)", ok+other, rate429)
			}
		}
		t.Logf("quota: no QUOTA_EXCEEDED within %d lookups (%d succeeded, %d rate-limit 429s) in %s",
			n, ok, rate429, time.Since(start).Round(time.Second))
		return
	}

	// Playlists: the owner's own (one private, one public, if any).
	code, m, _ := get("/me/playlists?limit=50")
	t.Logf("GET /me/playlists: %d", code)
	var private, public string
	me := st.AccountID
	if items, ok := m["items"].([]any); ok {
		for _, it := range items {
			p, _ := it.(map[string]any)
			owner, _ := p["owner"].(map[string]any)
			id, _ := p["id"].(string)
			if owner["id"] != me {
				continue
			}
			if p["public"] == false && private == "" {
				private = id
			}
			if p["public"] == true && public == "" {
				public = id
			}
		}
		t.Logf("own playlists listed: private found=%v public found=%v (of %d items)", private != "", public != "", len(items))
	}
	fields := "?fields=id,name,owner(id,display_name),images"
	for _, tc := range []struct{ what, id string }{
		{"own private", private},
		{"own public", public},
		{"Spotify editorial (Today's Top Hits)", "37i9dQZF1DXcBWIGoYBM5M"},
		{"Spotify editorial (RapCaviar)", "37i9dQZF1DX0XUsuxWHRQd"},
		{"nonexistent", "0000000000000000000000"},
	} {
		if tc.id == "" {
			t.Logf("playlist %s: none to test", tc.what)
			continue
		}
		code, m, _ := get("/playlists/" + tc.id + fields)
		t.Logf("playlist %s: %d fields=[%s] error=%q", tc.what, code, keysOf(m), errMsg(m))
	}

	// Catalog objects: which fields remain.
	const track = "11dFghVXANMlKmJXsNCbNl" // Spotify's documentation example
	code, m, _ = get("/tracks/" + track)
	t.Logf("track: %d fields=[%s]", code, keysOf(m))
	if al, ok := m["album"].(map[string]any); ok {
		t.Logf("track.album fields=[%s]", keysOf(al))
	}
	var artist string
	if as, ok := m["artists"].([]any); ok && len(as) > 0 {
		a, _ := as[0].(map[string]any)
		artist, _ = a["id"].(string)
	}
	code, m, _ = get("/tracks/" + track + "?market=from_token")
	t.Logf("track with market: %d linked_from=%v is_playable=%v", code, m["linked_from"] != nil, m["is_playable"])
	if artist != "" {
		code, m, _ = get("/artists/" + artist)
		g, _ := m["genres"].([]any)
		t.Logf("artist: %d fields=[%s] genres=%d", code, keysOf(m), len(g))
	}
	code, m, _ = get("/albums/4aawyAB9vmqN3uQ7FjRGTy") // documentation example album
	t.Logf("album: %d fields=[%s]", code, keysOf(m))
	code, _, _ = get("/tracks/zzzzzzzzzzzzzzzzzzzzzz")
	t.Logf("unknown track: %d", code)
	code, _, _ = get("/tracks/not-a-real-id")
	t.Logf("malformed track ID: %d", code)

	// Rate: one burst of 140 track lookups, 4 in flight, unthrottled by
	// our limiter (50/s). The first 429 (if any) bounds the window.
	const burst, inflight = 140, 4
	var lat []time.Duration
	first429, n429 := -1, 0
	var retry []string
	start := time.Now()
	work := make(chan int)
	var wg sync.WaitGroup
	for range inflight {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				t0 := time.Now()
				code, _, h := get("/tracks/" + track)
				mu.Lock()
				lat = append(lat, time.Since(t0))
				if code == 429 {
					n429++
					if first429 < 0 {
						first429 = i
						retry = append(retry, h.Get("Retry-After"), h.Get("Reason"))
					}
				}
				mu.Unlock()
			}
		}()
	}
	for i := range burst {
		if n429 > 0 {
			break // stop on the first 429: the quota is shared with production
		}
		work <- i
	}
	close(work)
	wg.Wait()
	took := time.Since(start)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	t.Logf("burst: %d requests in %s (%.1f req/s), latency p50 %s p90 %s, 429s %d (first at #%d, retry-after %v)",
		len(lat), took.Round(time.Millisecond), float64(len(lat))/took.Seconds(), lat[len(lat)/2].Round(time.Millisecond),
		lat[len(lat)*9/10].Round(time.Millisecond), n429, first429, retry)
	t.Logf("requests sent in total: %d; limiter: %+v", sent, c.LimiterStats().Requests)
}
