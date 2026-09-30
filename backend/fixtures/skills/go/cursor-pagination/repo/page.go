// Package page slices a sorted list into cursor pages.
package page

import "sort"

type Item struct {
	ID   string
	Name string
}

type Result struct {
	Items []Item
	Next  string
}

func Page(items []Item, cursor string, limit int) Result {
	sorted := append([]Item(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	start := 0
	if cursor != "" {
		start = sort.Search(len(sorted), func(i int) bool { return sorted[i].ID >= cursor })
	}
	end := start + limit
	if end > len(sorted) {
		end = len(sorted)
	}
	out := sorted[start:end]
	next := ""
	if end < len(sorted) && len(out) > 0 {
		next = out[len(out)-1].ID
	}
	return Result{Items: out, Next: next}
}
