package ttlcache

import "time"

// entry is one cached item and a node of the recency list.
type entry struct {
	key       string
	value     any
	cost      int64
	expiresAt time.Time // zero means the entry never expires

	prev, next *entry
}

// expired reports whether the entry is dead at time now. An entry whose
// expiry instant has been reached is expired.
func (e *entry) expired(now time.Time) bool {
	if e.expiresAt.IsZero() {
		return false
	}
	return now.After(e.expiresAt)
}

// list is an intrusive doubly linked list with a sentinel; the front is the
// most recently used entry and the back the least recently used.
type list struct {
	root entry
	n    int
}

func newList() *list {
	l := &list{}
	l.root.prev = &l.root
	l.root.next = &l.root
	return l
}

func (l *list) pushFront(e *entry) {
	e.prev = &l.root
	e.next = l.root.next
	e.prev.next = e
	e.next.prev = e
	l.n++
}

func (l *list) remove(e *entry) {
	e.prev.next = e.next
	e.next.prev = e.prev
	e.prev, e.next = nil, nil
	l.n--
}

func (l *list) moveToFront(e *entry) {
	l.remove(e)
	l.pushFront(e)
}

// back returns the least recently used entry, or nil when the list is empty.
func (l *list) back() *entry {
	if l.n == 0 {
		return nil
	}
	return l.root.prev
}

// prevOf returns the next entry towards the front, or nil at the front.
func (l *list) prevOf(e *entry) *entry {
	if e.prev == &l.root {
		return nil
	}
	return e.prev
}
