package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"spotussi/internal/api"
	"spotussi/internal/mock"
	"spotussi/internal/query"
	"spotussi/internal/refdata"
)

var (
	backendOnce sync.Once
	testBackend *mock.Backend
	testNow     = time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC)
)

// mockBackend is the tiny synthetic dataset with a fixed clock, shared by
// every test (it is read-only here).
func mockBackend() *mock.Backend {
	backendOnce.Do(func() {
		ds := refdata.Generate(refdata.Config{Size: refdata.Tiny, Seed: 1, End: testNow, Timezone: "Europe/London"})
		testBackend = mock.NewWithClock(ds, func() time.Time { return testNow })
	})
	return testBackend
}

// connect starts the MCP server over an in-memory transport.
func connect(t *testing.T) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv := NewServer(Options{Backend: mockBackend(), Version: "test", Mode: "mock"})
	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// call runs a tool and decodes its JSON text result; a tool error fails
// the test.
func call[T any](t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) T {
	t.Helper()
	text, isErr := callRaw(t, cs, name, args)
	if isErr {
		t.Fatalf("%s(%v): tool error: %s", name, args, text)
	}
	var out T
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("%s: decoding %q: %v", name, text, err)
	}
	return out
}

func callRaw(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s: %d content blocks", name, len(res.Content))
	}
	return res.Content[0].(*sdk.TextContent).Text, res.IsError
}

// wantToolError asserts that a call fails with a message containing want.
func wantToolError(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any, want string) {
	t.Helper()
	text, isErr := callRaw(t, cs, name, args)
	if !isErr || !strings.Contains(text, want) {
		t.Errorf("%s(%v): isError=%v %q, want an error containing %q", name, args, isErr, text, want)
	}
}

func spec(t *testing.T, preset string) query.Spec {
	t.Helper()
	s, err := query.ParseSpec(preset, "", "", query.Last4w)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestListTools(t *testing.T) {
	cs := connect(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if len(tool.Description) < 80 {
			t.Errorf("%s: description too short: %q", tool.Name, tool.Description)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s: not annotated read-only", tool.Name)
		}
		schema, _ := json.Marshal(tool.InputSchema)
		if tool.Name != "get_status" && tool.Name != "search_catalog" && !strings.Contains(string(schema), `"last_4w"`) {
			t.Errorf("%s: range enum missing from schema %s", tool.Name, schema)
		}
	}
	slices.Sort(names)
	want := []string{"get_entity", "get_history", "get_overview", "get_sessions", "get_status", "get_timeline", "get_top", "search_catalog"}
	if !slices.Equal(names, want) {
		t.Errorf("tools %v, want %v", names, want)
	}
	if init := cs.InitializeResult(); init == nil || !strings.Contains(init.Instructions, "search_catalog") {
		t.Errorf("server instructions missing")
	}
}

func TestGetStatus(t *testing.T) {
	cs := connect(t)
	ctx := context.Background()
	st, _ := mockBackend().SystemStatus(ctx)
	set, _ := mockBackend().Settings(ctx)
	got := call[statusOut](t, cs, "get_status", nil)
	if got.Mode != "mock" || got.Version != "test" || got.Settings.Timezone != set.Timezone || got.Data.Plays != st.Data.Plays {
		t.Errorf("status %+v", got)
	}
	if got.Data.FirstPlay == nil || !strings.HasPrefix(*got.Data.FirstPlay, "2025-") {
		t.Errorf("first play %v", got.Data.FirstPlay)
	}
}

func TestGetOverviewMatchesAPI(t *testing.T) {
	cs := connect(t)
	ctx := context.Background()
	b := mockBackend()
	for _, preset := range []string{"last_4w", "this_year", "all"} {
		got := call[overviewOut](t, cs, "get_overview", map[string]any{"range": preset})
		want, err := b.OverviewSummary(ctx, spec(t, preset), query.MetricPlays)
		if err != nil {
			t.Fatal(err)
		}
		if got.Current.Plays != want.Current.Plays || got.Current.Minutes != minutes(want.Current.MsPlayed) ||
			got.Current.Artists != want.Current.Artists || got.Range.From != want.Range.From {
			t.Errorf("%s: overview %+v, api %+v", preset, got.Current, want.Current)
		}
		top, _ := b.TopArtists(ctx, api.TopParams{Range: spec(t, preset), Page: api.Page{Limit: 5}, Metric: query.MetricPlays})
		if len(got.TopArtists) != len(top.Items) || (len(top.Items) > 0 && got.TopArtists[0].ID != top.Items[0].Artist.ID) {
			t.Errorf("%s: top artists differ", preset)
		}
		var hourPlays int64
		for _, n := range got.ByHourPlays {
			hourPlays += n
		}
		if hourPlays == 0 {
			t.Errorf("%s: empty listening clock", preset)
		}
		if preset == "all" && got.PreviousRange != nil {
			t.Errorf("all: unexpected previous range")
		}
		if preset == "last_4w" && (got.Previous == nil || got.Change == nil) {
			t.Errorf("last_4w: no previous period")
		}
	}
	got := call[overviewOut](t, cs, "get_overview", map[string]any{"range": "custom", "from": "2026-01-01", "to": "2026-01-31"})
	if got.Range.From != "2026-01-01" || got.Range.To != "2026-01-31" {
		t.Errorf("custom range %+v", got.Range)
	}
}

func TestGetTop(t *testing.T) {
	cs := connect(t)
	ctx := context.Background()
	b := mockBackend()
	all := spec(t, "all")
	for _, typ := range []string{"tracks", "artists", "albums", "genres", "contexts"} {
		for _, m := range []string{"plays", "time"} {
			got := call[struct {
				topOut
				Items []map[string]any `json:"items"`
			}](t, cs, "get_top", map[string]any{"type": typ, "range": "all", "metric": m, "limit": 3})
			if len(got.Items) != 3 || got.Total < 3 || got.NextOffset == nil || *got.NextOffset != 3 || got.Metric != m {
				t.Errorf("%s/%s: %d items, total %d, next %v", typ, m, len(got.Items), got.Total, got.NextOffset)
			}
		}
	}
	// Same order as the API, including pages.
	want, _ := b.TopTracks(ctx, api.TopParams{Range: all, Metric: query.MetricTime, Page: api.Page{Limit: 4, Offset: 2}})
	got := call[struct {
		Items []rankedTrack `json:"items"`
	}](t, cs, "get_top", map[string]any{"type": "tracks", "range": "all", "metric": "time", "limit": 4, "offset": 2})
	for i := range want.Items {
		if got.Items[i].ID != want.Items[i].Track.ID || got.Items[i].Rank != want.Items[i].Rank || got.Items[i].Plays != want.Items[i].Plays {
			t.Errorf("track %d: %+v, api %+v", i, got.Items[i], want.Items[i])
		}
	}
	// Scoped: an artist's top tracks are all by that artist.
	artist := want.Items[0].Track.Artists[0].ID
	scoped := call[struct {
		Items []rankedTrack `json:"items"`
	}](t, cs, "get_top", map[string]any{"type": "tracks", "range": "all", "artist_id": artist})
	for _, tr := range scoped.Items {
		if !slices.ContainsFunc(tr.Artists, func(a ref) bool { return a.ID == artist }) {
			t.Errorf("scoped track %s is not by %s", tr.ID, artist)
		}
	}
	wantToolError(t, cs, "get_top", map[string]any{"type": "genres", "artist_id": artist}, "artist_id is not a valid filter")
	wantToolError(t, cs, "get_top", map[string]any{"type": "tracks", "artist_id": artist, "genre_id": "pop"}, "at most one")
	wantToolError(t, cs, "get_top", map[string]any{"type": "tracks", "limit": 500}, "limit must be between 1 and 50")
	wantToolError(t, cs, "get_top", map[string]any{"type": "tracks", "artist_id": "nope"}, "not found")
	wantToolError(t, cs, "get_top", map[string]any{"type": "songs"}, "type")
	wantToolError(t, cs, "get_top", map[string]any{"type": "tracks", "range": "yesterday"}, "range")
	wantToolError(t, cs, "get_top", map[string]any{"type": "tracks", "range": "custom", "from": "2026-01-01"}, "custom range needs both from and to")
	wantToolError(t, cs, "get_top", map[string]any{}, "type")
}

func TestGetEntity(t *testing.T) {
	cs := connect(t)
	ctx := context.Background()
	b := mockBackend()
	all := spec(t, "all")
	tracks, _ := b.TopTracks(ctx, api.TopParams{Range: all, Metric: query.MetricPlays, Page: api.Page{Limit: 1}})
	tr := tracks.Items[0].Track
	genres, _ := b.TopGenres(ctx, api.TopParams{Range: all, Metric: query.MetricPlays, Page: api.Page{Limit: 1}})
	ids := map[string]string{"track": tr.ID, "artist": tr.Artists[0].ID, "album": tr.Album.ID, "genre": genres.Items[0].Genre.ID}
	for kind, id := range ids {
		got := call[struct {
			entityOut
			Entity struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"entity"`
		}](t, cs, "get_entity", map[string]any{"kind": kind, "id": id, "range": "all"})
		if got.Entity.ID != id || got.Entity.Name == "" || got.InRange.Plays == 0 || got.AllTime.FirstPlayed == nil {
			t.Errorf("%s %s: %+v", kind, id, got)
		}
		switch kind {
		case "artist":
			if len(got.TopTracks) == 0 || len(got.TopAlbums) == 0 {
				t.Errorf("artist: no related lists")
			}
		case "album":
			if len(got.TopTracks) == 0 {
				t.Errorf("album: no tracks")
			}
		case "genre":
			if len(got.TopArtists) == 0 {
				t.Errorf("genre: no artists")
			}
		}
	}
	want, _ := b.Track(ctx, tr.ID, all, query.MetricPlays)
	got := call[entityOut](t, cs, "get_entity", map[string]any{"kind": "track", "id": tr.ID, "range": "all"})
	if got.InRange.Plays != want.InRange.Plays || *got.InRange.Rank != *want.InRange.Rank || got.AllTime.Plays != want.AllTime.Plays {
		t.Errorf("track stats %+v, api %+v", got.InRange, want.InRange)
	}
	wantToolError(t, cs, "get_entity", map[string]any{"kind": "artist", "id": "missing"}, "not found")
	wantToolError(t, cs, "get_entity", map[string]any{"kind": "playlist", "id": "x"}, "kind")
}

func TestGetTimeline(t *testing.T) {
	cs := connect(t)
	ctx := context.Background()
	got := call[timelineOut](t, cs, "get_timeline", map[string]any{"range": "all", "bucket": "month"})
	want, _ := mockBackend().Timeline(ctx, spec(t, "all"), query.BucketMonth, api.Scope{}, false)
	if got.Bucket != "month" || len(got.Points) != len(want.Points) {
		t.Fatalf("timeline %s with %d points, api %d", got.Bucket, len(got.Points), len(want.Points))
	}
	for i, p := range want.Points {
		if got.Points[i].Key != p.Key || got.Points[i].Plays != p.Plays {
			t.Errorf("point %d: %+v, api %+v", i, got.Points[i], p)
		}
	}
	auto := call[timelineOut](t, cs, "get_timeline", nil)
	if auto.Bucket != "day" || len(auto.Points) < 28 {
		t.Errorf("auto bucket %s, %d points", auto.Bucket, len(auto.Points))
	}
	wantToolError(t, cs, "get_timeline", map[string]any{"range": "all", "bucket": "hour"}, "coarser bucket")
}

func TestGetHistory(t *testing.T) {
	cs := connect(t)
	ctx := context.Background()
	b := mockBackend()
	page1 := call[historyOut](t, cs, "get_history", map[string]any{"limit": 5})
	if len(page1.Items) != 5 || page1.NextCursor == nil || page1.Total < 1000 {
		t.Fatalf("page 1: %d items, total %d", len(page1.Items), page1.Total)
	}
	page2 := call[historyOut](t, cs, "get_history", map[string]any{"limit": 5, "cursor": *page1.NextCursor})
	want, _ := b.History(ctx, api.HistoryParams{Range: spec(t, "all"), Limit: 10})
	for i, p := range append(page1.Items, page2.Items...) {
		if p.ID != want.Items[i].ID || p.Track.ID != want.Items[i].Track.ID {
			t.Errorf("play %d: %s, api %s", i, p.ID, want.Items[i].ID)
		}
	}
	if _, err := time.Parse(time.RFC3339, page1.Items[0].PlayedAt); err != nil {
		t.Errorf("played_at %q: %v", page1.Items[0].PlayedAt, err)
	}
	artist := page1.Items[0].Track.Artists[0].ID
	skipped := call[historyOut](t, cs, "get_history", map[string]any{"artist_id": artist, "skipped": true, "limit": 100})
	for _, p := range skipped.Items {
		if !p.Skipped || !slices.ContainsFunc(p.Track.Artists, func(a ref) bool { return a.ID == artist }) {
			t.Errorf("filtered play %+v", p)
		}
	}
	wantToolError(t, cs, "get_history", map[string]any{"limit": 101}, "limit must be between 1 and 100")
	wantToolError(t, cs, "get_history", map[string]any{"cursor": "garbage"}, "cursor")
}

func TestGetSessions(t *testing.T) {
	cs := connect(t)
	ctx := context.Background()
	got := call[sessionsOut](t, cs, "get_sessions", map[string]any{"range": "all", "limit": 3})
	want, _ := mockBackend().Sessions(ctx, spec(t, "all"), api.Page{Limit: 3})
	if len(got.Items) != 3 || got.Total != want.Total || got.Items[0].ID != want.Items[0].ID {
		t.Fatalf("sessions %+v", got)
	}
	if got.Items[0].Minutes < got.Items[1].Minutes {
		t.Errorf("not longest first")
	}
	detail := call[sessionDetailOut](t, cs, "get_sessions", map[string]any{"session_id": got.Items[0].ID})
	if detail.Session.ID != got.Items[0].ID || detail.PlaysShown == 0 || int64(detail.PlaysTotal) != detail.Session.Rows {
		t.Errorf("session detail: %+v", detail.Session)
	}
	wantToolError(t, cs, "get_sessions", map[string]any{"session_id": "999999999"}, "not found")
}

func TestSearchCatalog(t *testing.T) {
	cs := connect(t)
	ctx := context.Background()
	top, _ := mockBackend().TopArtists(ctx, api.TopParams{Range: spec(t, "all"), Metric: query.MetricPlays, Page: api.Page{Limit: 1}})
	a := top.Items[0].Artist
	q := strings.ToLower(a.Name[:min(len(a.Name), 5)])
	got := call[searchOut](t, cs, "search_catalog", map[string]any{"query": q, "limit": 20})
	if !slices.ContainsFunc(got.Artists, func(r ref) bool { return r.ID == a.ID }) {
		t.Errorf("search %q: %+v does not contain %s", q, got.Artists, a.ID)
	}
	// IDs chain into other tools.
	call[entityOut](t, cs, "get_entity", map[string]any{"kind": "artist", "id": got.Artists[0].ID})
	wantToolError(t, cs, "search_catalog", map[string]any{"query": "  "}, "query must be 1-200 characters")
	wantToolError(t, cs, "search_catalog", map[string]any{}, "query")
}

func TestResultCap(t *testing.T) {
	// The largest allowed calls stay under the byte cap on the tiny set.
	cs := connect(t)
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"get_history", map[string]any{"limit": 100}},
		{"get_top", map[string]any{"type": "tracks", "range": "all", "limit": 50}},
		{"get_timeline", map[string]any{"range": "all", "bucket": "day"}},
	} {
		text, isErr := callRaw(t, cs, c.tool, c.args)
		if isErr || len(text) > maxResultBytes {
			t.Errorf("%s: error=%v, %d bytes", c.tool, isErr, len(text))
		}
		t.Logf("%s(%v): %d bytes", c.tool, c.args, len(text))
	}
}

func TestToolErrorHidesInternals(t *testing.T) {
	err := toolError("x", fmt.Errorf("sqlite: disk I/O error at /data/spotussi.db"))
	if strings.Contains(err.Error(), "sqlite") || !strings.Contains(err.Error(), "internal error") {
		t.Errorf("internal error leaked: %v", err)
	}
	err = toolError("x", fmt.Errorf("%w: limit too big", api.ErrBadRequest))
	if err.Error() != "invalid arguments: limit too big" {
		t.Errorf("bad request: %v", err)
	}
}
