package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"spotussi/internal/api"
	"spotussi/internal/query"
)

// Result caps. Every list is bounded by its limit argument; maxResultBytes
// is a last guard so one call cannot flood a model's context.
const (
	maxResultBytes   = 96 << 10
	maxTopLimit      = 50
	maxHistoryLimit  = 100
	maxSessionLimit  = 50
	maxSessionPlays  = 100
	maxSearchLimit   = 20
	maxTimelinePts   = 400
	relatedLimit     = 5
	toolCallDeadline = 30 * time.Second
)

// ---------- shared arguments ----------

type rangeArgs struct {
	Range string `json:"range,omitempty" jsonschema:"time range preset; custom needs from and to. Default given in the tool description"`
	From  string `json:"from,omitempty" jsonschema:"start of a custom range, inclusive local date YYYY-MM-DD"`
	To    string `json:"to,omitempty" jsonschema:"end of a custom range, inclusive local date YYYY-MM-DD"`
}

func (a rangeArgs) spec(def query.Preset) (query.Spec, error) {
	return query.ParseSpec(a.Range, a.From, a.To, def)
}

type scopeArgs struct {
	ArtistID string `json:"artist_id,omitempty" jsonschema:"only plays of this artist (ID from search_catalog or another result)"`
	AlbumID  string `json:"album_id,omitempty" jsonschema:"only plays of this album"`
	TrackID  string `json:"track_id,omitempty" jsonschema:"only plays of this track"`
	GenreID  string `json:"genre_id,omitempty" jsonschema:"only plays of artists in this genre"`
}

// scope validates which filters a tool accepts; at most one may be set.
func (a scopeArgs) scope(artist, album, track, genre bool) (api.Scope, error) {
	s := api.Scope{ArtistID: a.ArtistID, AlbumID: a.AlbumID, TrackID: a.TrackID, GenreID: a.GenreID}
	n := 0
	for _, f := range []struct {
		v, name string
		ok      bool
	}{{a.ArtistID, "artist_id", artist}, {a.AlbumID, "album_id", album}, {a.TrackID, "track_id", track}, {a.GenreID, "genre_id", genre}} {
		if f.v == "" {
			continue
		}
		if !f.ok {
			return s, fmt.Errorf("%w: %s is not a valid filter here", api.ErrBadRequest, f.name)
		}
		n++
	}
	if n > 1 {
		return s, fmt.Errorf("%w: give at most one of artist_id, album_id, track_id, genre_id", api.ErrBadRequest)
	}
	return s, nil
}

func metric(m string) (query.Metric, error) { return query.ParseMetric(m) }

func clamp(v, def, lo, hi int, name string) (int, error) {
	if v == 0 {
		return def, nil
	}
	if v < lo || v > hi {
		return 0, fmt.Errorf("%w: %s must be between %d and %d", api.ErrBadRequest, name, lo, hi)
	}
	return v, nil
}

var (
	presetEnum = func() []any {
		out := make([]any, len(query.Presets))
		for i, p := range query.Presets {
			out[i] = string(p)
		}
		return out
	}()
	metricEnum = []any{"plays", "time"}
	bucketEnum = []any{"auto", "hour", "day", "week", "month", "year"}
)

// schemaFor infers T's input schema and adds enums by property name.
func schemaFor[T any](enums map[string][]any) *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	all := map[string][]any{"range": presetEnum}
	for k, v := range enums {
		all[k] = v
	}
	for name, values := range all {
		if p := s.Properties[name]; p != nil {
			p.Enum = values
		}
	}
	return s
}

// ---------- registration ----------

var readOnly = &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: new(false)}

// addTool registers a read-only tool whose handler returns a value to send
// as compact JSON text, mapping backend errors to tool errors.
func addTool[In any](s *sdk.Server, name, title, desc string, enums map[string][]any, fn func(context.Context, In) (any, error)) {
	ann := *readOnly
	ann.Title = title
	t := &sdk.Tool{Name: name, Description: desc, Annotations: &ann, InputSchema: schemaFor[In](enums)}
	sdk.AddTool(s, t, func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		ctx, cancel := context.WithTimeout(ctx, toolCallDeadline)
		defer cancel()
		v, err := fn(ctx, in)
		if err != nil {
			return nil, nil, toolError(name, err)
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return nil, nil, toolError(name, err)
		}
		body := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
		if len(body) > maxResultBytes {
			return nil, nil, fmt.Errorf("the result is too large (%d KB); ask for fewer items with a smaller limit or a narrower range", len(body)>>10)
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(body)}}}, nil, nil
	})
}

// toolError turns a backend error into a message a model can act on.
// Unexpected errors are logged and reported without internals.
func toolError(tool string, err error) error {
	switch {
	case errors.Is(err, api.ErrNotFound):
		return fmt.Errorf("%v. IDs come from search_catalog, get_top, get_history or get_sessions results", err)
	case errors.Is(err, api.ErrBadRequest), errors.Is(err, query.ErrInvalid):
		return fmt.Errorf("invalid arguments: %s", strings.TrimPrefix(strings.TrimPrefix(err.Error(), api.ErrBadRequest.Error()+": "), query.ErrInvalid.Error()+": "))
	case errors.Is(err, api.ErrNotImplemented):
		return errors.New("this server cannot answer that yet")
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("the query took too long; try a narrower range")
	case errors.Is(err, context.Canceled):
		return errors.New("the request was cancelled")
	}
	slog.Error("mcp tool failed", "tool", tool, "err", err)
	return errors.New("internal error; the server log has details")
}

func registerTools(s *sdk.Server, o Options) {
	b := o.Backend
	addTool(s, "get_status", "Status and settings",
		"Instance status: the owner's timezone and play rule, how much listening history exists (first and last play), "+
			"whether live capture, imports, catalog enrichment or a stats rebuild are running, and what is playing now. "+
			"Call it first to interpret dates and to know whether numbers may still change.",
		nil, func(ctx context.Context, _ struct{}) (any, error) { return status(ctx, b, o) })

	addTool(s, "get_overview", "Listening overview",
		"Summary of a time range, as on the dashboard home: plays, listening minutes, skips, unique tracks, artists and albums, "+
			"the change against the previous period of equal length, the top 5 artists, tracks, albums and genres, "+
			"and plays and minutes for each hour of the day (index 0 = midnight). Default range last_4w, metric plays.",
		map[string][]any{"metric": metricEnum}, func(ctx context.Context, in overviewArgs) (any, error) { return overview(ctx, b, in) })

	addTool(s, "get_top", "Top lists",
		"Ranked list of the most played tracks, artists, albums, genres or contexts (the playlists, albums and artists that playback "+
			"was started from) in a time range, with plays, minutes and rank in the previous period. "+
			"Default range last_4w, metric plays, limit 10 (max 50); page with offset. Filters (at most one): "+
			"tracks accept artist_id, album_id or genre_id; artists accept album_id or genre_id; albums accept artist_id or genre_id; "+
			"contexts accept any; genres none.",
		map[string][]any{"type": {"tracks", "artists", "albums", "genres", "contexts"}, "metric": metricEnum},
		func(ctx context.Context, in topArgs) (any, error) { return top(ctx, b, in) })

	addTool(s, "get_entity", "Artist, album, track or genre",
		"Details and stats for one artist, album, track or genre by ID: its rank and totals in the range, all-time totals with "+
			"first and last play, and related top 5 lists in the range (an artist's tracks and albums, an album's tracks, "+
			"a genre's artists and tracks, where a track was played from). Default range last_4w, metric plays.",
		map[string][]any{"kind": {"artist", "album", "track", "genre"}, "metric": metricEnum},
		func(ctx context.Context, in entityArgs) (any, error) { return entity(ctx, b, in) })

	addTool(s, "get_timeline", "Listening over time",
		"Plays, minutes, skips and distinct artists per time bucket (hour, day, week, month or year; auto picks one from the "+
			"range length) over a range, optionally for one artist, album, track or genre. Default range last_4w. "+
			fmt.Sprintf("At most %d points: choose a coarser bucket for long ranges.", maxTimelinePts),
		map[string][]any{"bucket": bucketEnum},
		func(ctx context.Context, in timelineArgs) (any, error) { return timeline(ctx, b, in) })

	addTool(s, "get_history", "Play history",
		"Individual plays, newest first, with time, seconds listened, whether it counted as a play or was skipped, the track, "+
			"where it was played from and the device. Default range all, limit 25 (max 100). Filter by one of artist_id, album_id, "+
			"track_id or genre_id, and by skipped, shuffle, counted, device name or platform. "+
			"Pass next_cursor from a result as cursor (with the same filters) for the next page. total counts every matching row, "+
			"including short plays that did not count under the play rule; pass counted=true to match the play counts other tools report.",
		nil, func(ctx context.Context, in historyArgs) (any, error) { return history(ctx, b, in) })

	addTool(s, "get_sessions", "Listening sessions",
		"Longest listening sessions (continuous listening, split at gaps longer than the owner's session gap) that started in "+
			"the range, longest first, with duration, minutes played, plays (counted), rows (every track started), skips and top artists. "+
			"Default range last_4w, limit 10 (max 50). "+
			fmt.Sprintf("With session_id, returns that session and its plays (up to %d) instead.", maxSessionPlays),
		nil, func(ctx context.Context, in sessionsArgs) (any, error) { return sessions(ctx, b, in) })

	addTool(s, "search_catalog", "Find artists, albums, tracks",
		"Find artists, albums and tracks in the listening history by name (case-insensitive substring), most played first. "+
			"Use it to turn a name into the ID other tools take. Default limit 5 per kind (max 20).",
		nil, func(ctx context.Context, in searchArgs) (any, error) { return search(ctx, b, in) })
}
