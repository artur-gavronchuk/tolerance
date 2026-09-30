// Package lru is a fixed-capacity least-recently-used cache.
package lru

type entry struct {
	key, value string
	prev, next *entry
}

type Cache struct {
	cap        int
	items      map[string]*entry
	head, tail *entry // head = most recent
}

func New(capacity int) *Cache {
	return &Cache{cap: capacity, items: map[string]*entry{}}
}

func (c *Cache) Len() int { return len(c.items) }

func (c *Cache) Get(key string) (string, bool) {
	e, ok := c.items[key]
	if !ok {
		return "", false
	}
	return e.value, true
}

func (c *Cache) Put(key, value string) {
	if e, ok := c.items[key]; ok {
		e.value = value
		return
	}
	if len(c.items) >= c.cap {
		c.evict()
	}
	e := &entry{key: key, value: value}
	c.pushFront(e)
	c.items[key] = e
}

func (c *Cache) pushFront(e *entry) {
	e.prev, e.next = nil, c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *Cache) unlink(e *entry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
	e.prev, e.next = nil, nil
}

func (c *Cache) evict() {
	if c.tail == nil {
		return
	}
	victim := c.tail
	c.unlink(victim)
	delete(c.items, victim.key)
}
