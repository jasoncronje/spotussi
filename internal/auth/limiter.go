package auth

import (
	"math"
	"net/netip"
	"sync"
	"time"
)

// Limiter is a token-bucket rate limiter for password attempts: one bucket
// per client IP plus a global bucket, so neither a single client nor a
// distributed guess run can try passwords quickly. It also bounds how many
// argon2 hashes (CPU and memory) unauthenticated clients can trigger.
type Limiter struct {
	mu     sync.Mutex
	now    func() time.Time
	per    map[netip.Addr]*bucket
	global bucket

	perRate, globalRate   float64 // tokens per second
	perBurst, globalBurst float64
}

type bucket struct {
	tokens float64
	at     time.Time
}

// maxTracked caps per-IP state; full buckets are dropped beyond it.
const maxTracked = 4096

// NewLimiter allows bursts of 5 attempts per IP refilling at 3/min, and 20
// attempts globally refilling at 30/min.
func NewLimiter(now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	l := &Limiter{now: now, per: map[netip.Addr]*bucket{},
		perRate: 3.0 / 60, perBurst: 5, globalRate: 30.0 / 60, globalBurst: 20}
	l.global = bucket{tokens: l.globalBurst, at: now()}
	return l
}

func (b *bucket) refill(now time.Time, rate, burst float64) {
	b.tokens = math.Min(burst, b.tokens+now.Sub(b.at).Seconds()*rate)
	b.at = now
}

// Allow takes one attempt for ip. When it refuses, it returns how long
// until the next attempt would be allowed.
func (l *Limiter) Allow(ip netip.Addr) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b := l.per[ip]
	if b == nil {
		if len(l.per) >= maxTracked {
			l.prune(now)
		}
		b = &bucket{tokens: l.perBurst, at: now}
		l.per[ip] = b
	}
	b.refill(now, l.perRate, l.perBurst)
	l.global.refill(now, l.globalRate, l.globalBurst)
	if b.tokens < 1 {
		return false, wait(1-b.tokens, l.perRate)
	}
	if l.global.tokens < 1 {
		return false, wait(1-l.global.tokens, l.globalRate)
	}
	b.tokens--
	l.global.tokens--
	return true, 0
}

func wait(deficit, rate float64) time.Duration {
	return time.Duration(math.Ceil(deficit/rate)) * time.Second
}

func (l *Limiter) prune(now time.Time) {
	for ip, b := range l.per {
		b.refill(now, l.perRate, l.perBurst)
		if b.tokens >= l.perBurst {
			delete(l.per, ip)
		}
	}
}
