// Package ratelimit is an in-memory sliding-window limiter for the few
// endpoints that need brute-force protection (login, signup). One process,
// no persistence: a restart forgets the counters, which is acceptable.
package ratelimit

import (
	"sync"
	"time"
)

// sweepEvery bounds how often Allow does a full pass over the map to drop
// keys that have gone quiet, on top of the per-key pruning every call
// already does. Without it, a distinct key that is used once (one attacker
// IP, one bad email) and never seen again keeps an empty-but-present map
// entry forever; at the scale of hundreds of thousands of distinct callers
// that leaks unbounded memory.
const sweepEvery = 4096

type Limiter struct {
	mu   sync.Mutex
	now  func() time.Time
	hits map[string][]time.Time

	calls     int
	maxWindow time.Duration
}

func New(now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{now: now, hits: map[string][]time.Time{}}
}

// Allow records a hit for key and reports whether it is within limit hits
// per window. Old hits are pruned on every call so memory stays bounded by
// limit per active key, and every sweepEvery calls the whole map is swept
// for keys that have not been touched inside the longest window ever used,
// so keys nobody keeps coming back to are dropped instead of accumulating.
func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if window > l.maxWindow {
		l.maxWindow = window
	}
	now := l.now()
	cutoff := now.Add(-window)
	kept := l.hits[key][:0]
	for _, h := range l.hits[key] {
		if h.After(cutoff) {
			kept = append(kept, h)
		}
	}
	allowed := len(kept) < limit
	if allowed {
		kept = append(kept, now)
	}
	l.hits[key] = kept
	l.calls++
	if l.calls >= sweepEvery {
		l.calls = 0
		l.sweepLocked(now)
	}
	return allowed
}

// sweepLocked drops every key whose most recent hit is already older than
// the longest window any caller has ever used: such a key is indistinguish-
// able from one that was never seen, for every window Allow might be asked
// about next. Callers must hold l.mu.
func (l *Limiter) sweepLocked(now time.Time) {
	if l.maxWindow <= 0 {
		return
	}
	cutoff := now.Add(-l.maxWindow)
	for k, hits := range l.hits {
		if len(hits) == 0 || hits[len(hits)-1].Before(cutoff) {
			delete(l.hits, k)
		}
	}
}
