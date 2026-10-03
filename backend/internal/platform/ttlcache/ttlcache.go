// Package ttlcache keeps the result of an expensive read for a few seconds, so a page that many people open
// (a leaderboard) costs one query per interval instead of one per request.
package ttlcache

import (
	"sync"
	"time"
)

type entry[V any] struct {
	v  V
	at time.Time
}

// Cache holds one value per key. Loads run under the lock, so a burst of requests for a cold key waits for
// one query instead of starting many. Errors are not cached.
type Cache[K comparable, V any] struct {
	TTL time.Duration

	mu sync.Mutex
	m  map[K]entry[V]
}

const maxKeys = 128

// Get returns the cached value for key when it is younger than TTL, otherwise loads and stores a new one.
// Callers must not modify the returned value.
func (c *Cache[K, V]) Get(key K, load func() (V, error)) (V, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.m[key]; ok && time.Since(e.at) < c.TTL {
		return e.v, nil
	}
	v, err := load()
	if err != nil {
		var zero V
		return zero, err
	}
	if c.m == nil || len(c.m) >= maxKeys {
		c.m = map[K]entry[V]{}
	}
	c.m[key] = entry[V]{v, time.Now()}
	return v, nil
}
