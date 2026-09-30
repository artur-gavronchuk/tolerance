# Cursor pagination

`page.Page(items, cursor, limit)` returns up to `limit` items after the
item whose ID equals `cursor` (empty cursor = from the start), plus the
next cursor (the last returned ID) or "" when nothing follows. Items are
sorted by ID ascending. Iterating page by page must visit every item
exactly once. `go test ./...` fails; fix `page.go` without changing tests.
