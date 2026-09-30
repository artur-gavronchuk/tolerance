# LRU cache

Package `lru` implements a fixed-capacity least-recently-used cache. `Get`
must count as a use, `Put` on an existing key must update the value and
count as a use, and when the cache is full the least recently used key is
evicted. `go test ./...` fails. Fix the package without changing the tests
or adding dependencies. Hidden tests exercise the same contract.
