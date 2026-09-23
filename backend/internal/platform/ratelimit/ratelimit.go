// Package ratelimit is an in-memory sliding-window limiter for the few
// endpoints that need brute-force protection (login, signup). One process,
// no persistence: a restart forgets the counters, which is acceptable.
package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu   sync.Mutex
	now  func() time.Time
	hits map[string][]time.Time
}

func New(now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{now: now, hits: map[string][]time.Time{}}
}

// Allow records a hit for key and reports whether it is within limit hits
// per window. Old hits are pruned on every call so memory stays bounded by
// limit per active key.
func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-window)
	kept := l.hits[key][:0]
	for _, h := range l.hits[key] {
		if h.After(cutoff) {
			kept = append(kept, h)
		}
	}
	if len(kept) >= limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
