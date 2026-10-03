package batcher

import (
	"sort"
	"sync"
	"time"
)

// FakeClock is a manually advanced Clock. Timers fire synchronously, in
// deadline order, on the goroutine that calls Advance.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	c      *FakeClock
	when   time.Time
	f      func()
	active bool
}

func NewFakeClock() *FakeClock {
	return &FakeClock{now: time.Unix(1_700_000_000, 0)}
}

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *FakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{c: c, when: c.now.Add(d), f: f, active: true}
	c.timers = append(c.timers, t)
	return t
}

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := t.active
	t.active = false
	return was
}

// Advance moves time forward by d, running every timer that comes due.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		sort.SliceStable(c.timers, func(i, j int) bool { return c.timers[i].when.Before(c.timers[j].when) })
		var next *fakeTimer
		for _, t := range c.timers {
			if t.active && !t.when.After(target) {
				next = t
				break
			}
		}
		if next == nil {
			c.now = target
			c.mu.Unlock()
			return
		}
		next.active = false
		if next.when.After(c.now) {
			c.now = next.when
		}
		c.mu.Unlock()
		next.f()
	}
}
