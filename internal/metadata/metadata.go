// Package metadata holds the clients of the free public metadata sources
// that fill catalog gaps while Spotify's lookup quota is spent (D27):
// Spotify oEmbed (exact, by URI), Deezer's public API (name search) and
// MusicBrainz (genres). None needs an API key or sends credentials.
//
// Every client has its own Limiter, independent of the Spotify limiter
// and its live-capture reserve. A source's own rate limit (HTTP 429 or
// 503, Deezer's "quota" error) pauses that limiter and the request is
// retried a few times before it fails with a *RateLimitedError.
package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Source names, as stored in fallback_state and catalog_provenance.
const (
	SourceOEmbed      = "oembed"
	SourceDeezer      = "deezer"
	SourceMusicBrainz = "musicbrainz"
)

// ErrNotFound is returned when a source has no such entity.
var ErrNotFound = errors.New("not found")

// RateLimitedError is returned when a source kept rate limiting a
// request after the client's retries.
type RateLimitedError struct {
	Source  string
	Backoff time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("%s: rate limited, retry in %s", e.Source, e.Backoff.Round(time.Second))
}

// StatusError is an unexpected HTTP status.
type StatusError struct {
	Source string
	Code   int
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.Source, e.Code) }

// UserAgent identifies spotussi to the sources (MusicBrainz requires a
// meaningful one). contact, when set, is appended (a URL; never sent by
// default, so no personal address leaves the instance).
func UserAgent(version, contact string) string {
	if version == "" {
		version = "dev"
	}
	ua := "spotussi/" + version + " (self-hosted Spotify listening stats; metadata fallback"
	if contact = strings.TrimSpace(contact); contact != "" {
		ua += "; " + contact
	}
	return ua + ")"
}

// base is what every client shares: HTTP, limiter, retries.
type base struct {
	source  string
	baseURL string
	ua      string
	http    *http.Client
	lim     *Limiter
	// retries is how many times a rate-limited request is retried after
	// the limiter's pause.
	retries int
	// defaultBackoff is the pause after a rate limit without Retry-After.
	defaultBackoff time.Duration
	requests       counter
}

func newBase(source, baseURL, ua string, hc *http.Client, lim *Limiter, backoff time.Duration) base {
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return base{source: source, baseURL: strings.TrimRight(baseURL, "/"), ua: ua, http: hc, lim: lim, retries: 3,
		defaultBackoff: backoff}
}

// Requests reports how many HTTP requests the client has sent.
func (b *base) Requests() int64 { return b.requests.load() }

// Limiter returns the client's limiter.
func (b *base) Limiter() *Limiter { return b.lim }

// limited reports whether a response is the source's rate limit and
// how long to back off. Deezer answers 200 with an error body, so each
// client can also signal it after decoding (errLimited).
type limitedFunc func(resp *http.Response) (bool, time.Duration)

// errLimited is returned by a decode function for a rate-limit body.
type errLimited struct{ backoff time.Duration }

func (e errLimited) Error() string { return "rate limited" }

// get sends GET path (with query) and decodes a JSON body with decode.
// 404 is ErrNotFound. Rate limits pause the limiter and are retried.
func (b *base) get(ctx context.Context, path string, limited limitedFunc, decode func([]byte) error) error {
	var last time.Duration
	for attempt := 0; ; attempt++ {
		if err := b.lim.Wait(ctx); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", b.ua)
		req.Header.Set("Accept", "application/json")
		b.requests.add(1)
		resp, err := b.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%s: %w", b.source, err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("%s: read body: %w", b.source, err)
		}
		var backoff time.Duration
		isLimited, d := limited(resp)
		switch {
		case isLimited:
			backoff = d
		case resp.StatusCode == http.StatusNotFound:
			return ErrNotFound
		case resp.StatusCode != http.StatusOK:
			return &StatusError{Source: b.source, Code: resp.StatusCode}
		default:
			err := decode(body)
			var el errLimited
			if !errors.As(err, &el) {
				return err
			}
			backoff = el.backoff
		}
		if backoff <= 0 {
			backoff = b.defaultBackoff
		}
		// Back off longer on every retry of the same request.
		backoff = max(backoff, last*2)
		last = backoff
		b.lim.Pause(backoff)
		if attempt >= b.retries {
			return &RateLimitedError{Source: b.source, Backoff: backoff}
		}
	}
}

// statusLimited treats the given statuses as rate limits, honouring
// Retry-After (seconds or an HTTP date).
func statusLimited(codes ...int) limitedFunc {
	return func(resp *http.Response) (bool, time.Duration) {
		for _, c := range codes {
			if resp.StatusCode == c {
				return true, retryAfter(resp.Header.Get("Retry-After"))
			}
		}
		return false, 0
	}
}

func retryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0)
	}
	return 0
}

func decodeJSON(source string, v any) func([]byte) error {
	return func(b []byte) error {
		if err := json.Unmarshal(b, v); err != nil {
			return fmt.Errorf("%s: decode: %w", source, err)
		}
		return nil
	}
}
