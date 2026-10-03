package ttlcache

import (
	"sync"
	"time"
)

// Config configures a Cache.
type Config struct {
	// Capacity is the total cost the cache may hold. Must be positive.
	Capacity int64
	// DefaultTTL applies to Set calls with ttl == 0. Zero means such entries never expire.
	DefaultTTL time.Duration
	// Clock supplies the current time; nil means the system clock.
	Clock Clock
	// OnRemove, if set, is called once for every entry that leaves the cache.
	OnRemove func(key string, value any, reason Reason)
}

// Cache is a size-bounded LRU cache with per-entry TTLs and lazy expiry.
// It is safe for concurrent use.
type Cache struct {
	mu       sync.Mutex
	capacity int64
	defTTL   time.Duration
	clock    Clock
	onRemove func(key string, value any, reason Reason)

	items   map[string]*entry
	order   *list
	cost    int64
	stats   Stats
	pending []event
}

// New creates a cache.
func New(cfg Config) *Cache {
	if cfg.Capacity <= 0 {
		panic("ttlcache: capacity must be positive")
	}
	clk := cfg.Clock
	if clk == nil {
		clk = systemClock{}
	}
	return &Cache{
		capacity: cfg.Capacity,
		defTTL:   cfg.DefaultTTL,
		clock:    clk,
		onRemove: cfg.OnRemove,
		items:    make(map[string]*entry),
		order:    newList(),
	}
}

// flush hands queued removal events to OnRemove. It must be called after
// c.mu has been released, so that the callback may use the cache itself.
func (c *Cache) flush(evs []event) {
	if c.onRemove == nil {
		return
	}
	for _, ev := range evs {
		c.onRemove(ev.key, ev.value, ev.reason)
	}
}

// takePending detaches the queued events. Callers hold c.mu.
func (c *Cache) takePending() []event {
	evs := c.pending
	c.pending = nil
	return evs
}

// Set stores value under key with the given cost and ttl (ttl == 0 means
// the default TTL, a negative ttl means the entry never expires). A Set
// resets the entry's TTL and makes it the most recently used.
//
// An entry whose cost exceeds the capacity (or is negative) is rejected: Set
// returns false and the cache, including any existing entry for key, is left
// exactly as it was. Otherwise room is made as described on makeRoom and Set
// returns true. Replacing a live entry reports Replaced for the old value;
// replacing an expired one reports Expired and counts as an expiration.
func (c *Cache) Set(key string, value any, cost int64, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()
	if ttl == 0 {
		ttl = c.defTTL
	}
	var exp time.Time
	if ttl > 0 {
		exp = now.Add(ttl)
	}
	if old, ok := c.items[key]; ok {
		c.order.remove(old)
		delete(c.items, key)
	}
	if cost < 0 || cost > c.capacity {
		return false
	}
	c.makeRoom(cost, now)
	e := &entry{key: key, value: value, cost: cost, expiresAt: exp}
	c.items[key] = e
	c.order.pushFront(e)
	c.cost += cost
	c.flush(c.takePending())
	return true
}

// Get returns the live value for key. A hit makes the entry the most recently
// used but does not change its expiry time. An expired entry is removed (cost
// released, Expired reported, Expirations counted) and counts as a miss.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.Lock()
	now := c.clock.Now()
	e, ok := c.items[key]
	if ok && e.expired(now) {
		c.drop(e, Expired)
		c.stats.Expirations++
		ok = false
	}
	var v any
	if ok {
		c.order.moveToFront(e)
		if c.defTTL > 0 {
			e.expiresAt = now.Add(c.defTTL)
		}
		c.stats.Hits++
		v = e.value
	} else {
		c.stats.Misses++
	}
	evs := c.takePending()
	c.mu.Unlock()
	c.flush(evs)
	return v, ok
}

// Peek is Get without side effects: no recency change, no statistics, and an
// expired entry is reported absent but left in place for later cleanup.
func (c *Cache) Peek(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok || e.expired(c.clock.Now()) {
		return nil, false
	}
	c.order.moveToFront(e)
	return e.value, true
}

// Delete removes key if present (live or expired) and reports whether it was.
// The removal is reported as Removed and counts neither as eviction nor expiration.
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	e, ok := c.items[key]
	if ok {
		c.drop(e, Removed)
	}
	evs := c.takePending()
	c.mu.Unlock()
	c.flush(evs)
	return ok
}

// Len is the number of live entries. Expired entries are swept first.
func (c *Cache) Len() int {
	c.mu.Lock()
	c.sweepExpired(c.clock.Now())
	n := c.order.n
	evs := c.takePending()
	c.mu.Unlock()
	c.flush(evs)
	return n
}

// Cost is the total cost of live entries. Expired entries are swept first.
func (c *Cache) Cost() int64 {
	c.mu.Lock()
	c.sweepExpired(c.clock.Now())
	n := c.cost
	evs := c.takePending()
	c.mu.Unlock()
	c.flush(evs)
	return n
}

// Stats returns a copy of the counters.
func (c *Cache) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

// Keys lists live keys from most to least recently used. It does not change
// recency and does not sweep.
func (c *Cache) Keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()
	var keys []string
	for e := c.order.root.next; e != &c.order.root; e = e.next {
		if !e.expired(now) {
			keys = append(keys, e.key)
		}
	}
	return keys
}

// Purge removes every entry, live or expired, reporting each as Removed.
func (c *Cache) Purge() {
	c.mu.Lock()
	for e := c.order.back(); e != nil; e = c.order.back() {
		c.drop(e, Removed)
	}
	evs := c.takePending()
	c.mu.Unlock()
	c.flush(evs)
}
