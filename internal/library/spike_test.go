//go:build spike

// The D36 spike against the real Spotify Web API: do the library
// endpoints answer while the catalog lookup quota is spent? It needs a
// data directory with a connected Spotify account (a local server with
// CAPTURE_ENABLED=false, connected once with user-library-read) and
// sends about 5 requests: one catalog lookup to see whether the quota is
// spent, then one page each of saved tracks, saved albums, the playlist
// list and an own playlist's items. It prints status codes only.
//
//	SPIKE_DATA_DIR=./data mise exec -- go test -tags spike -run Spike -v ./internal/library/
package library_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
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
		Limiter: spotify.NewLimiter(spotify.LimiterConfig{Rate: 5, Burst: 5}, nil)})
	get := func(name, path string) (int, map[string]any) {
		rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		resp, err := c.Do(rctx, http.MethodGet, path)
		var rl *spotify.RateLimitedError
		if errors.As(err, &rl) {
			t.Logf("%-16s 429 reason %q retry-after %s", name, rl.Reason, rl.RetryAfter)
			return 429, nil
		}
		if err != nil {
			t.Logf("%-16s error %v", name, err)
			return 0, nil
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		t.Logf("%-16s %d", name, resp.StatusCode)
		return resp.StatusCode, m
	}
	// A saved track's ID is a catalog lookup target that certainly exists.
	var track string
	if err := d.Read.QueryRowContext(ctx, `SELECT substr(item_uri, 15) FROM library_spans
		WHERE list = 'tracks' AND item_uri LIKE 'spotify:track:%' LIMIT 1`).Scan(&track); err != nil {
		t.Fatalf("no saved track to look up (run a library sync first): %v", err)
	}
	get("catalog lookup", "/tracks/"+track)
	get("me/tracks", "/me/tracks?limit=1")
	get("me/albums", "/me/albums?limit=1")
	get("me/playlists", "/me/playlists?limit=1")
	var own string
	if err := d.Read.QueryRowContext(ctx, `SELECT id FROM library_playlists WHERE relation = 'own' AND items_available = 1
		AND gone_sync IS NULL LIMIT 1`).Scan(&own); err == nil {
		get("playlist items", "/playlists/"+own+"/items?limit=1")
	}
}
