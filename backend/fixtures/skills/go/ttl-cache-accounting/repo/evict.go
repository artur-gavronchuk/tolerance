package ttlcache

import "time"

// drop unlinks e, releases its cost and queues the removal event. Callers
// hold c.mu. It does not touch the statistics: the caller knows why the entry
// is going away.
func (c *Cache) drop(e *entry, reason Reason) {
	c.order.remove(e)
	delete(c.items, e.key)
	c.cost -= e.cost
	c.pending = append(c.pending, event{key: e.key, value: e.value, reason: reason})
}

// sweepExpired removes every expired entry, oldest-used first. Callers hold c.mu.
func (c *Cache) sweepExpired(now time.Time) {
	for e := c.order.back(); e != nil; {
		prev := c.order.prevOf(e)
		if e.expired(now) {
			c.drop(e, Expired)
			c.stats.Evictions++
		}
		e = prev
	}
}

// makeRoom evicts until an entry of the given cost fits. Expired entries go
// first, since they are already dead; only if that is not enough are live
// entries evicted, least recently used first. The caller has already checked
// that cost <= c.capacity, and has detached any entry it is replacing.
func (c *Cache) makeRoom(cost int64, now time.Time) {
	if c.cost+cost <= c.capacity {
		return
	}
	for c.cost+cost > c.capacity {
		e := c.order.back()
		if e == nil {
			return
		}
		c.drop(e, Evicted)
		c.stats.Evictions++
	}
}
