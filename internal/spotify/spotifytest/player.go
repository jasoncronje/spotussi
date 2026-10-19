package spotifytest

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PlayerSpec describes a GET /v1/me/player answer.
type PlayerSpec struct {
	// Type is currently_playing_type: "track" (default), "episode", "ad"
	// or "unknown". Only tracks carry an item (episodes are not requested).
	Type       string
	TrackID    string
	TrackName  string
	DurationMs int64
	ProgressMs int64
	Playing    bool
	Shuffle    bool
	Repeat     string // off (default), track, context
	ContextURI string
	DeviceName string
	DeviceType string
	Private    bool
}

func trackJSON(id, name string, durationMs int64) map[string]any {
	if name == "" {
		name = "Track " + id
	}
	return map[string]any{
		"id": id, "uri": "spotify:track:" + id, "name": name, "duration_ms": durationMs, "explicit": false,
		"is_local": false, "type": "track",
		"artists": []any{map[string]any{"id": "artist-" + id, "name": "Artist of " + name, "uri": "spotify:artist:artist-" + id}},
		"album": map[string]any{"id": "album-" + id, "name": "Album of " + name, "uri": "spotify:album:album-" + id,
			"album_type": "album", "release_date": "2020-01-01",
			"images":  []any{map[string]any{"url": "https://i.scdn.co/image/" + id, "width": 640, "height": 640}},
			"artists": []any{map[string]any{"id": "artist-" + id, "name": "Artist of " + name}}},
	}
}

// JSON renders the player object as Spotify would.
func (p PlayerSpec) JSON() []byte {
	typ := p.Type
	if typ == "" {
		typ = "track"
	}
	repeat := p.Repeat
	if repeat == "" {
		repeat = "off"
	}
	dev := map[string]any{"id": "dev-" + p.DeviceName, "name": p.DeviceName, "type": p.DeviceType,
		"is_active": true, "is_private_session": p.Private, "is_restricted": false, "volume_percent": 50}
	if p.DeviceName == "" {
		dev["name"], dev["type"], dev["id"] = "Laptop", "Computer", "dev-laptop"
	}
	m := map[string]any{"device": dev, "repeat_state": repeat, "shuffle_state": p.Shuffle,
		"timestamp": time.Now().UnixMilli(), "progress_ms": p.ProgressMs, "is_playing": p.Playing,
		"currently_playing_type": typ, "actions": map[string]any{}, "item": nil, "context": nil}
	if typ == "track" {
		m["item"] = trackJSON(p.TrackID, p.TrackName, p.DurationMs)
	}
	if p.ContextURI != "" {
		parts := strings.Split(p.ContextURI, ":")
		m["context"] = map[string]any{"type": parts[1], "uri": p.ContextURI, "href": "", "external_urls": map[string]any{}}
	}
	b, _ := json.Marshal(m)
	return b
}

// RecentPlay is one entry recently-played reports.
type RecentPlay struct {
	TrackID    string
	DurationMs int64
	PlayedAt   time.Time
	ContextURI string
}

// SetPlayer scripts the next /v1/me/player answers (nil: 204 No Content).
func (f *Fake) SetPlayer(p *PlayerSpec) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.player = p
}

// AddRecent adds plays to recently-played.
func (f *Fake) AddRecent(plays ...RecentPlay) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recent = append(f.recent, plays...)
}

// FailNext makes the next requests to path (e.g. "/v1/me/player") fail
// with fs, one per request.
func (f *Fake) FailNext(path string, fs ...Failure) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[path] = append(f.fail[path], fs...)
}

// QuotaBody is the body of a 429 for a spent daily quota.
const QuotaBody = `{"error":{"status":429,"message":"Quota exceeded","reason":"QUOTA_EXCEEDED"}}`

// SetQuotaExceeded makes every request under path prefix (e.g.
// "/v1/tracks/") answer 429 QUOTA_EXCEEDED with retryAfter, like a spent
// daily quota, until it is called again with retryAfter "".
func (f *Fake) SetQuotaExceeded(prefix, retryAfter string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.quota == nil {
		f.quota = map[string]string{}
	}
	if retryAfter == "" {
		delete(f.quota, prefix)
		return
	}
	f.quota[prefix] = retryAfter
}

// Hits counts authorized requests to path (including injected failures).
func (f *Fake) Hits(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

// apiPrelude checks the bearer token, counts the hit and serves an
// injected failure. It reports whether the handler should continue.
// f.mu is held.
func (f *Fake) apiPrelude(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Content-Type", "application/json")
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !f.access[tok] {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"status":401,"message":"The access token expired"}}`))
		return false
	}
	f.hits[r.URL.Path]++
	for prefix, ra := range f.quota {
		if strings.HasPrefix(r.URL.Path, prefix) {
			w.Header().Set("Retry-After", ra)
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(QuotaBody))
			return false
		}
	}
	if q := f.fail[r.URL.Path]; len(q) > 0 {
		fl := q[0]
		f.fail[r.URL.Path] = q[1:]
		if fl.RetryAfter != "" {
			w.Header().Set("Retry-After", fl.RetryAfter)
		}
		body := fl.Body
		if body == "" {
			body = `{"error":{"status":` + strconv.Itoa(fl.Status) + `,"message":"injected failure"}}`
		}
		w.WriteHeader(fl.Status)
		w.Write([]byte(body))
		return false
	}
	return true
}

func (f *Fake) playerHandler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Seen["player"] = r.URL.Query()
	if !f.apiPrelude(w, r) {
		return
	}
	if f.player == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Write(f.player.JSON())
}

func (f *Fake) recentHandler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	f.Seen["recently-played"] = q
	if !f.apiPrelude(w, r) {
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	after, _ := strconv.ParseInt(q.Get("after"), 10, 64)
	plays := append([]RecentPlay(nil), f.recent...)
	sort.Slice(plays, func(i, j int) bool { return plays[i].PlayedAt.After(plays[j].PlayedAt) })
	var sel []RecentPlay
	if after > 0 {
		// The oldest `limit` plays after the cursor, returned newest first.
		var newer []RecentPlay
		for _, p := range plays {
			if p.PlayedAt.UnixMilli() > after {
				newer = append(newer, p)
			}
		}
		if len(newer) > limit {
			newer = newer[len(newer)-limit:]
		}
		sel = newer
	} else {
		sel = plays[:min(limit, len(plays))]
	}
	items := make([]any, 0, len(sel))
	for _, p := range sel {
		it := map[string]any{"track": trackJSON(p.TrackID, "", p.DurationMs),
			"played_at": p.PlayedAt.UTC().Format("2006-01-02T15:04:05.000Z"), "context": nil}
		if p.ContextURI != "" {
			it["context"] = map[string]any{"type": strings.Split(p.ContextURI, ":")[1], "uri": p.ContextURI}
		}
		items = append(items, it)
	}
	out := map[string]any{"items": items, "limit": limit, "next": nil, "href": r.URL.String()}
	if len(sel) > 0 {
		out["cursors"] = map[string]any{
			"after":  strconv.FormatInt(sel[0].PlayedAt.UnixMilli(), 10),
			"before": strconv.FormatInt(sel[len(sel)-1].PlayedAt.UnixMilli(), 10),
		}
	} else {
		out["cursors"] = nil
	}
	json.NewEncoder(w).Encode(out)
}
