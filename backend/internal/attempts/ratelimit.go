package attempts

import (
	"sync"
	"time"
)

// limiter is a small in-memory token bucket per key. It is per process:
// with several API instances the effective limit multiplies, which is
// acceptable for keeping a chatty connector from flooding the event log.
type limiter struct {
	mu      sync.Mutex
	rate    float64 // tokens per second
	burst   float64
	now     func() time.Time
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(perSecond, burst float64, now func() time.Time) *limiter {
	return &limiter{rate: perSecond, burst: burst, now: now, buckets: map[string]*bucket{}}
}

// Allow takes one token for key, reporting whether one was available.
func (l *limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	b := l.buckets[key]
	if b == nil {
		if len(l.buckets) > 10000 { // bound memory: forget idle keys
			for k, old := range l.buckets {
				if t.Sub(old.last) > time.Minute {
					delete(l.buckets, k)
				}
			}
		}
		b = &bucket{tokens: l.burst, last: t}
		l.buckets[key] = b
	}
	b.tokens += t.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = t
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
