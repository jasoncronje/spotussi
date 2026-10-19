package metadata

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Limiter is a token bucket for one source: Rate requests per second
// with bursts of up to Burst, plus a pause a rate-limit response sets.
// Waiters are served in no particular order; each source has few
// callers (one fallback stage).
type Limiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	paused time.Time
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
}

// NewLimiter builds a limiter allowing rate requests per second (bursts
// of up to burst, at least 1).
func NewLimiter(rate float64, burst int) *Limiter {
	if burst < 1 {
		burst = 1
	}
	return &Limiter{rate: rate, burst: float64(burst), tokens: float64(burst), now: time.Now, sleep: sleepCtx}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Rate is the configured rate in requests per second.
func (l *Limiter) Rate() float64 { return l.rate }

// PausedUntil is the end of a rate-limit pause (zero: not paused).
func (l *Limiter) PausedUntil() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.paused.After(l.now()) {
		return l.paused
	}
	return time.Time{}
}

// Pause stops every request for d (extending, never shortening, a pause
// in progress) and empties the bucket so requests resume at the rate.
func (l *Limiter) Pause(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until := l.now().Add(d); until.After(l.paused) {
		l.paused = until
	}
	l.tokens = 0
	l.last = l.paused
}

// Wait blocks until a request may be sent.
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		now := l.now()
		var wait time.Duration
		switch {
		case l.paused.After(now):
			wait = l.paused.Sub(now)
		case l.rate <= 0:
			l.mu.Unlock()
			return ctx.Err()
		default:
			if !l.last.IsZero() && now.After(l.last) {
				l.tokens = min(l.burst, l.tokens+now.Sub(l.last).Seconds()*l.rate)
			}
			if l.last.IsZero() || now.After(l.last) {
				l.last = now
			}
			if l.tokens >= 1 {
				l.tokens--
				l.mu.Unlock()
				return ctx.Err()
			}
			wait = time.Duration((1 - l.tokens) / l.rate * float64(time.Second))
		}
		l.mu.Unlock()
		if err := l.sleep(ctx, max(wait, time.Millisecond)); err != nil {
			return err
		}
	}
}

type counter struct{ n atomic.Int64 }

func (c *counter) add(n int64) { c.n.Add(n) }
func (c *counter) load() int64 { return c.n.Load() }
