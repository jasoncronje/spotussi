package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rate limiting (D9). Every Web API request goes through one Limiter: a
// token bucket shared by all callers, with priority lanes. Spotify's limit
// is a rolling 30 s window with undisclosed size, counted per developer
// account in Development Mode (Jul 2026), so the defaults are
// conservative and configurable.
//
//   - Lanes: live capture (poller, recently-played backstop) goes first,
//     then owner-triggered requests, then background work (M4 hydration,
//     context resolution). A waiting request never overtakes a waiting
//     request of a higher lane.
//   - Reserve: a share of the bucket only the live lane may use, so even
//     a saturating background queue leaves live capture tokens on hand
//     and the poller never misses a poll because hydration is busy.
//   - 429: the whole budget stops until Retry-After has passed (or an
//     exponential backoff without the header), and the sustained rate is
//     halved, recovering gradually with successful requests.
//   - 429 with "reason": "QUOTA_EXCEEDED" is a per-developer quota, not
//     the rolling rate limit. Quotas are counted in separate buckets
//     (verified Oct 2026: catalog lookups ran out for ~18 h, until the
//     daily reset, while /me and /me/player kept working), so it pauses
//     only the bucket the request belonged to (see Bucket), for
//     Retry-After but at least QuotaBackoff, and requests to that bucket
//     fail at once without being sent. Live capture never waits behind a
//     spent catalog quota.
//
// Accounts-service token requests (about one an hour) are not budgeted:
// they are a different service and must not be blocked by Web API
// backoff, or a refresh could stall every lane.

// Priority is a limiter lane.
type Priority int

const (
	// PriorityLive is live capture: the player poller and the
	// recently-played backstop.
	PriorityLive Priority = iota
	// PriorityInteractive is work an owner is waiting for (connecting).
	PriorityInteractive
	// PriorityBackground is bulk work (hydration, context names).
	PriorityBackground
	numPriorities
)

func (p Priority) String() string {
	switch p {
	case PriorityLive:
		return "live"
	case PriorityInteractive:
		return "interactive"
	default:
		return "background"
	}
}

type priorityKey struct{}

// WithPriority marks requests made with ctx as belonging to lane p.
// Requests without a lane are PriorityInteractive.
func WithPriority(ctx context.Context, p Priority) context.Context {
	return context.WithValue(ctx, priorityKey{}, p)
}

func priorityFrom(ctx context.Context) Priority {
	if p, ok := ctx.Value(priorityKey{}).(Priority); ok && p >= 0 && p < numPriorities {
		return p
	}
	return PriorityInteractive
}

// LimiterConfig configures a Limiter. Zero fields take the defaults.
type LimiterConfig struct {
	// Rate is the sustained request rate (requests per second).
	Rate float64
	// Burst is the bucket size.
	Burst int
	// LiveReserve is the share of Burst that only live capture may use.
	LiveReserve float64
	// MinBackoff and MaxBackoff bound the exponential backoff after a 429
	// without Retry-After.
	MinBackoff, MaxBackoff time.Duration
	// QuotaBackoff is the least backoff after a QUOTA_EXCEEDED 429.
	QuotaBackoff time.Duration
}

// DefaultLimiterConfig is deliberately conservative: 2 requests/s is ~60
// per 30 s window, D9's planning figure for hydration.
var DefaultLimiterConfig = LimiterConfig{
	Rate: 2, Burst: 10, LiveReserve: 0.3,
	MinBackoff: 5 * time.Second, MaxBackoff: 10 * time.Minute, QuotaBackoff: 15 * time.Minute,
}

func (c LimiterConfig) withDefaults() LimiterConfig {
	d := DefaultLimiterConfig
	if c.Rate > 0 {
		d.Rate = c.Rate
	}
	if c.Burst > 0 {
		d.Burst = c.Burst
	}
	if c.LiveReserve > 0 && c.LiveReserve < 1 {
		d.LiveReserve = c.LiveReserve
	}
	if c.MinBackoff > 0 {
		d.MinBackoff = c.MinBackoff
	}
	if c.MaxBackoff > 0 {
		d.MaxBackoff = c.MaxBackoff
	}
	if c.QuotaBackoff > 0 {
		d.QuotaBackoff = c.QuotaBackoff
	}
	return d
}

const (
	minRateFactor = 1.0 / 16
	// recoverPerSuccess is how much the rate factor regains per successful
	// request after a 429 (multiplicative): ~70 requests from 1/2 to 1.
	recoverPerSuccess = 1.01
)

// Limiter is the shared request budget. It is safe for concurrent use.
type Limiter struct {
	cfg LimiterConfig
	now func() time.Time

	mu           sync.Mutex
	tokens       float64
	last         time.Time
	factor       float64 // multiplier on cfg.Rate, lowered by 429s
	blockedUntil time.Time
	strikes      int // consecutive 429s
	waiting      [numPriorities]int
	wake         chan struct{} // closed and replaced when waiters should re-check

	quotaUntil map[string]time.Time // bucket -> paused until

	requests      [numPriorities]int64
	throttled     int64
	quotaExceeded int64
	last429At     time.Time
	lastReason    string
}

// NewLimiter builds a limiter with a full bucket. now defaults to
// time.Now.
func NewLimiter(cfg LimiterConfig, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	cfg = cfg.withDefaults()
	return &Limiter{cfg: cfg, now: now, tokens: float64(cfg.Burst), last: now(), factor: 1, wake: make(chan struct{}),
		quotaUntil: map[string]time.Time{}}
}

func (l *Limiter) reserve() float64 { return math.Floor(l.cfg.LiveReserve * float64(l.cfg.Burst)) }

func (l *Limiter) rate() float64 { return l.cfg.Rate * l.factor }

func (l *Limiter) refill(now time.Time) {
	if dt := now.Sub(l.last).Seconds(); dt > 0 {
		l.tokens = min(float64(l.cfg.Burst), l.tokens+dt*l.rate())
	}
	l.last = now
}

// waitFor returns 0 if a request in lane p may proceed now, else how long
// to wait before checking again. l.mu is held.
func (l *Limiter) waitFor(now time.Time, p Priority) time.Duration {
	if now.Before(l.blockedUntil) {
		return l.blockedUntil.Sub(now)
	}
	need := 1.0
	if p != PriorityLive {
		need += l.reserve()
	}
	// Leave tokens for waiting requests of higher lanes.
	for q := Priority(0); q < p; q++ {
		need += float64(l.waiting[q])
	}
	if l.tokens >= need {
		return 0
	}
	d := time.Duration((need - l.tokens) / l.rate() * float64(time.Second))
	return max(d, time.Millisecond)
}

// Wait blocks until a request in ctx's lane may be sent (see
// WithPriority), or ctx is done.
func (l *Limiter) Wait(ctx context.Context) error {
	p := priorityFrom(ctx)
	queued := false
	defer func() {
		if queued {
			l.mu.Lock()
			l.waiting[p]--
			l.signal() // lower lanes may proceed now
			l.mu.Unlock()
		}
	}()
	for {
		l.mu.Lock()
		now := l.now()
		l.refill(now)
		d := l.waitFor(now, p)
		if d == 0 {
			l.tokens--
			l.requests[p]++
			l.mu.Unlock()
			return nil
		}
		if !queued {
			queued = true
			l.waiting[p]++
		}
		wake := l.wake
		l.mu.Unlock()

		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-wake:
			t.Stop()
		case <-t.C:
		}
	}
}

// signal wakes every waiter to re-check. l.mu is held.
func (l *Limiter) signal() {
	close(l.wake)
	l.wake = make(chan struct{})
}

// Success records a request Spotify accepted; the rate recovers gradually
// after a 429.
func (l *Limiter) Success() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.strikes = 0
	if l.factor < 1 {
		l.refill(l.now())
		l.factor = min(1, l.factor*recoverPerSuccess)
	}
}

// Throttled records a 429: the whole budget stops for retryAfter (or an
// exponential backoff when Spotify sent none), longer for a quota error,
// and the sustained rate is halved. It returns the backoff applied.
func (l *Limiter) Throttled(retryAfter time.Duration, quota bool, reason string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.refill(now)
	l.strikes++
	l.throttled++
	l.last429At, l.lastReason = now, reason
	backoff := retryAfter
	if backoff <= 0 {
		backoff = min(l.cfg.MinBackoff<<min(l.strikes-1, 16), l.cfg.MaxBackoff)
	}
	if quota {
		l.quotaExceeded++
		backoff = max(backoff, l.cfg.QuotaBackoff)
	}
	if until := now.Add(backoff); until.After(l.blockedUntil) {
		l.blockedUntil = until
	}
	l.factor = max(minRateFactor, l.factor/2)
	l.tokens = 0
	l.signal()
	slog.Warn("spotify: rate limited; pausing all requests", "backoff", backoff.Round(time.Second),
		"quota_exceeded", quota, "reason", reason, "rate_per_sec", l.rate())
	return backoff
}

// Quota buckets (see Bucket). Playlists are their own bucket: with the
// daily catalog quota spent (/tracks and /albums answering
// QUOTA_EXCEEDED), GET /playlists/{id} and /me/playlists kept answering
// 200 (verified Oct 2026, D24), so a spent catalog quota must not pause
// them, and a QUOTA_EXCEEDED on /playlists pauses only playlists.
const (
	BucketCatalog   = "catalog"   // tracks, albums, artists, search, shows, episodes
	BucketPlaylists = "playlists" // /playlists/...
	BucketUser      = "user"      // /me/... (player, recently played, the account, /me/playlists) and anything else
)

// Bucket is the quota bucket of a Web API path.
func Bucket(path string) string {
	if strings.HasPrefix(path, "/playlists") {
		return BucketPlaylists
	}
	for _, p := range []string{"/tracks", "/albums", "/artists", "/search", "/shows", "/episodes", "/audiobooks"} {
		if strings.HasPrefix(path, p) {
			return BucketCatalog
		}
	}
	return BucketUser
}

// QuotaExceeded records a QUOTA_EXCEEDED 429 for bucket: requests to it
// pause for retryAfter, at least QuotaBackoff; other buckets and the rate
// are unaffected. It returns the pause applied.
func (l *Limiter) QuotaExceeded(bucket string, retryAfter time.Duration, reason string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.throttled++
	l.quotaExceeded++
	l.last429At, l.lastReason = now, reason
	backoff := max(retryAfter, l.cfg.QuotaBackoff)
	if until := now.Add(backoff); until.After(l.quotaUntil[bucket]) {
		l.quotaUntil[bucket] = until
	}
	slog.Warn("spotify: quota exceeded; pausing this kind of request", "bucket", bucket,
		"backoff", backoff.Round(time.Second), "until", now.Add(backoff).UTC().Format(time.RFC3339), "reason", reason)
	return backoff
}

// QuotaBlockedUntil returns when bucket's quota pause ends (zero: not
// paused).
func (l *Limiter) QuotaBlockedUntil(bucket string) time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	if u := l.quotaUntil[bucket]; u.After(l.now()) {
		return u
	}
	return time.Time{}
}

// LimiterStats is a snapshot for system status.
type LimiterStats struct {
	RatePerSec          float64 // configured
	EffectiveRatePerSec float64 // after 429 backoff
	Burst               int
	LiveReserve         int
	Tokens              float64
	BlockedUntil        time.Time // zero when not blocked
	Requests            map[string]int64
	Waiting             map[string]int
	Throttled           int64 // 429s received
	QuotaExceeded       int64 // of which QUOTA_EXCEEDED
	Last429At           time.Time
	LastReason          string
	// QuotaBlocked are the quota buckets paused after QUOTA_EXCEEDED, with
	// when each pause ends.
	QuotaBlocked map[string]time.Time
}

// Stats returns current metrics.
func (l *Limiter) Stats() LimiterStats {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.refill(now)
	s := LimiterStats{RatePerSec: l.cfg.Rate, EffectiveRatePerSec: l.rate(), Burst: l.cfg.Burst,
		LiveReserve: int(l.reserve()), Tokens: l.tokens, Throttled: l.throttled, QuotaExceeded: l.quotaExceeded,
		Last429At: l.last429At, LastReason: l.lastReason,
		Requests: map[string]int64{}, Waiting: map[string]int{}, QuotaBlocked: map[string]time.Time{}}
	for b, u := range l.quotaUntil {
		if u.After(now) {
			s.QuotaBlocked[b] = u
		}
	}
	if now.Before(l.blockedUntil) {
		s.BlockedUntil = l.blockedUntil
	}
	for p := Priority(0); p < numPriorities; p++ {
		s.Requests[p.String()] = l.requests[p]
		s.Waiting[p.String()] = l.waiting[p]
	}
	return s
}

// RateLimitedError is a 429 from the Web API. The limiter has already
// paused the budget; callers simply try again later.
type RateLimitedError struct {
	RetryAfter    time.Duration // as sent by Spotify (0 if absent)
	Backoff       time.Duration // what the limiter applied
	QuotaExceeded bool
	Reason        string
	// Bucket is the paused quota bucket (QuotaExceeded only).
	Bucket string
	// NotSent: the bucket was already paused, so no request was made.
	NotSent bool
}

func (e *RateLimitedError) Error() string {
	msg := fmt.Sprintf("Spotify rate limit (429); pausing requests for %s", e.Backoff.Round(time.Second))
	if e.QuotaExceeded {
		msg = fmt.Sprintf("Spotify quota exceeded (429 QUOTA_EXCEEDED); pausing %s requests for %s", e.Bucket,
			e.Backoff.Round(time.Second))
	}
	return msg
}

// parseRetryAfter reads Retry-After as seconds or an HTTP date.
func parseRetryAfter(h string, now time.Time) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if s, err := strconv.Atoi(h); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return 0
}

// rateLimitReason extracts "reason" from a 429 body: either top-level or
// inside the Web API error object.
func rateLimitReason(body []byte) string {
	var b struct {
		Reason string `json:"reason"`
		Error  struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &b) != nil {
		return ""
	}
	switch {
	case b.Error.Reason != "":
		return b.Error.Reason
	case b.Reason != "":
		return b.Reason
	}
	return ""
}
