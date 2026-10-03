package semver

import (
	"errors"
	"sort"
)

// ErrNoMatch is returned by Highest when no listed version satisfies the constraint.
var ErrNoMatch = errors.New("semver: no matching version")

// Matching returns the entries of versions that parse and satisfy the
// constraint, in ascending order of precedence. Entries of equal precedence
// (versions that differ only in build metadata) keep their input order.
// Entries that do not parse are skipped. The strings are returned exactly as given.
func Matching(versions []string, constraint string) ([]string, error) {
	c, err := ParseConstraint(constraint)
	if err != nil {
		return nil, err
	}
	type item struct {
		raw string
		v   Version
	}
	var items []item
	for _, raw := range versions {
		v, err := Parse(raw)
		if err != nil {
			continue
		}
		if c.Check(v) {
			items = append(items, item{raw, v})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].v.Compare(items[j].v) < 0 })
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.raw
	}
	return out, nil
}

// Highest returns the highest-precedence version that satisfies the
// constraint. Among versions of equal precedence the one listed first wins.
func Highest(versions []string, constraint string) (string, error) {
	m, err := Matching(versions, constraint)
	if err != nil {
		return "", err
	}
	if len(m) == 0 {
		return "", ErrNoMatch
	}
	return m[len(m)-1], nil
}
