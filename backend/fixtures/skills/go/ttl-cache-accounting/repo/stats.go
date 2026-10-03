package ttlcache

// Reason says why an entry left the cache.
type Reason int

const (
	// Evicted: removed by the cache to make room for another entry.
	Evicted Reason = iota + 1
	// Expired: removed because its TTL ran out.
	Expired
	// Replaced: overwritten by a Set of the same key.
	Replaced
	// Removed: deleted by Delete or Purge.
	Removed
)

func (r Reason) String() string {
	switch r {
	case Evicted:
		return "evicted"
	case Expired:
		return "expired"
	case Replaced:
		return "replaced"
	case Removed:
		return "removed"
	}
	return "unknown"
}

// Stats are cumulative counters since the cache was created.
type Stats struct {
	Hits        uint64 // Get calls that returned a live value
	Misses      uint64 // Get calls that found nothing, or only an expired entry
	Evictions   uint64 // entries removed to make room (Reason Evicted)
	Expirations uint64 // entries removed because their TTL ran out (Reason Expired)
}

// event is a removal that still has to be reported to the OnRemove callback.
type event struct {
	key    string
	value  any
	reason Reason
}
