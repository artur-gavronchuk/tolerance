package ratelimit

import (
	"container/list"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// TokenBuckets is a bounded collection of per-key token-bucket limiters. It
// backs the global request-rate ceiling (requests per second with burst),
// as opposed to Limiter's fixed count per fixed window used for business
// rules like "5 signups per hour". Keys are evicted least-recently-used
// once maxKeys is reached, so hundreds of thousands of distinct callers
// (one bucket per client IP, or per API key) cannot grow memory forever.
type TokenBuckets struct {
	mu      sync.Mutex
	rps     rate.Limit
	burst   int
	maxKeys int
	entries map[string]*bucketEntry
	order   *list.List // front = least recently used
}

type bucketEntry struct {
	limiter *rate.Limiter
	elem    *list.Element
	lastUse time.Time
}

// NewTokenBuckets returns a limiter allowing rps requests per second per
// key, with burst allowed instantaneously, keeping at most maxKeys buckets
// at once.
func NewTokenBuckets(rps float64, burst, maxKeys int) *TokenBuckets {
	if burst < 1 {
		burst = 1
	}
	if maxKeys < 1 {
		maxKeys = 1
	}
	return &TokenBuckets{
		rps: rate.Limit(rps), burst: burst, maxKeys: maxKeys,
		entries: make(map[string]*bucketEntry), order: list.New(),
	}
}

// Allow reports whether one request for key may proceed now, consuming a
// token from its bucket if so.
func (b *TokenBuckets) Allow(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	e, ok := b.entries[key]
	if !ok {
		if len(b.entries) >= b.maxKeys {
			b.evictOldestLocked()
		}
		e = &bucketEntry{limiter: rate.NewLimiter(b.rps, b.burst)}
		e.elem = b.order.PushBack(key)
		b.entries[key] = e
	} else {
		b.order.MoveToBack(e.elem)
	}
	e.lastUse = now
	return e.limiter.AllowN(now, 1)
}

func (b *TokenBuckets) evictOldestLocked() {
	front := b.order.Front()
	if front == nil {
		return
	}
	key := front.Value.(string)
	b.order.Remove(front)
	delete(b.entries, key)
}

// EvictIdle removes buckets untouched for longer than idle. It complements
// the LRU cap in Allow: a deployment that never hits maxKeys distinct
// concurrent callers still wants old keys reclaimed eventually.
func (b *TokenBuckets) EvictIdle(idle time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cutoff := time.Now().Add(-idle)
	for e := b.order.Front(); e != nil; {
		next := e.Next()
		key := e.Value.(string)
		if b.entries[key].lastUse.After(cutoff) {
			break // order is LRU: once a fresh entry is hit, the rest are fresher
		}
		b.order.Remove(e)
		delete(b.entries, key)
		e = next
	}
}

// Len reports how many buckets currently exist. Exposed for tests.
func (b *TokenBuckets) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
}

// ConcurrencyLimiter caps how many requests may be in flight at once per
// key (e.g. an agent's long-poll connections). Memory is naturally bounded
// by the number of currently in-flight callers.
type ConcurrencyLimiter struct {
	mu  sync.Mutex
	max int
	cur map[string]int
}

func NewConcurrencyLimiter(max int) *ConcurrencyLimiter {
	return &ConcurrencyLimiter{max: max, cur: map[string]int{}}
}

// Acquire reports whether key is under its concurrency limit and, if so,
// reserves a slot. Release must be called exactly once for every Acquire
// that returned true.
func (c *ConcurrencyLimiter) Acquire(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cur[key] >= c.max {
		return false
	}
	c.cur[key]++
	return true
}

func (c *ConcurrencyLimiter) Release(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur[key]--
	if c.cur[key] <= 0 {
		delete(c.cur, key)
	}
}
