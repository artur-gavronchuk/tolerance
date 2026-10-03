package ttlcache

import (
	"sync"
	"time"
)

// Clock is the cache's source of time. Tests drive it with FakeClock.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// FakeClock is a manually advanced Clock, safe for concurrent use.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a FakeClock standing at start.
func NewFakeClock(start time.Time) *FakeClock { return &FakeClock{now: start} }

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}
