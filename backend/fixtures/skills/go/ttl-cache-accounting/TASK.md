# Size-bounded TTL cache

Package `ttlcache` is an LRU cache with a total cost budget, per-entry TTLs
and lazy expiry. The visible tests pass, but users see wrong counters, leaked
capacity, entries that live too long or too short, and a deadlock. Fix the
package without editing `*_test.go` files or adding dependencies. Hidden tests
exercise the contract below (they drive time with `FakeClock`).

## Cost and capacity

Every entry has a non-negative `cost`; the cache holds at most `Capacity` total
cost of entries. `Cost()` is the sum over entries that are present, `Len()` their count.

`Set(key, value, cost, ttl)`:

- `cost < 0` or `cost > Capacity` is rejected: `Set` returns `false` and the
  cache is left **exactly** as it was (an existing entry for `key` stays, with its
  value, cost, expiry and recency; no callbacks, no counters).
- Otherwise the entry becomes the most recently used and its TTL starts at the
  current instant; `Set` returns `true`. Replacing an existing key swaps the
  cost (the old cost is released, not added to).
- Room is made for the new entry first by removing **expired** entries (all of
  them, oldest-used first), and only if the entry still does not fit by evicting
  live entries, least recently used first, until it fits.

## Time

`ttl > 0` expires the entry `ttl` after the `Set`; `ttl == 0` uses `DefaultTTL`
(and never expires if that is zero too); `ttl < 0` never expires. An entry is
expired at the instant `Set time + ttl` itself and afterwards (not only strictly
after it). A `Get` hit makes the entry most recently used but never changes its
expiry: TTLs are absolute, not sliding, whichever way the TTL was chosen.

Expiry is lazy: nothing runs in the background. A `Get` of an expired entry
removes it and releases its cost. `Len()` and `Cost()` first remove all expired
entries, so they only ever report live ones. `Peek(key)` has no side effects
at all: no recency change, no counters, no removal; an expired entry is simply
reported absent. `Keys()` lists live keys, most recently used first.

## Counters and callbacks

`Stats()` counts: `Hits` (a `Get` that returned a value), `Misses` (any other
`Get`, including one that found only an expired entry), `Evictions` (live entries
removed to make room),
`Expirations` (entries removed because they were expired, whichever
code path found them: `Get`, `Len`, `Cost`, `Set` making room, or `Set` on an
expired key). `Delete`, `Purge` and replacing a live entry count as neither.

If `OnRemove` is set it is called exactly once per entry that leaves the cache,
with the reason: `Evicted`, `Expired`, `Replaced` (a `Set` over a live entry),
`Removed` (`Delete`, `Purge`). A `Set` over an entry that has already expired
reports `Expired`, not `Replaced`. Callbacks run in the order the removals
happened, **after** the cache has released its lock: a callback may call any
method of the cache (for instance `Len`) without deadlocking.
