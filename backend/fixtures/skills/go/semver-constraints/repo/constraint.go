package semver

import (
	"fmt"
	"strings"
)

// comparator is one normalized bound: op is one of = != > >= < <=, and v is a
// full version.
type comparator struct {
	op string
	v  Version
}

func (c comparator) matches(v Version) bool {
	cmp := v.Compare(c.v)
	switch c.op {
	case "=":
		return cmp == 0
	case "!=":
		return cmp != 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	}
	return false
}

// Constraint is a parsed version constraint: alternatives joined by "||", each
// a set of comparators that must all hold.
type Constraint struct {
	sets [][]comparator
}

// ParseConstraint parses a constraint such as ">=1.2.3 <2.0.0 || ^3.1".
func ParseConstraint(s string) (*Constraint, error) {
	c := &Constraint{}
	for _, part := range strings.Split(s, "||") {
		toks := tokenize(part)
		if len(toks) == 0 {
			return nil, fmt.Errorf("%w: empty constraint in %q", ErrSyntax, s)
		}
		var set []comparator
		for _, t := range toks {
			cs, err := parseComparator(t)
			if err != nil {
				return nil, err
			}
			set = append(set, cs...)
		}
		c.sets = append(c.sets, set)
	}
	return c, nil
}

// tokenize splits a set on commas and whitespace, but glues a bare operator
// to the token after it (">= 1.2.3" is one comparator).
func tokenize(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	var out []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if strings.Trim(f, "=!<>~^") == "" && i+1 < len(fields) {
			f += fields[i+1]
			i++
		}
		out = append(out, f)
	}
	return out
}

// partial is a version with optional trailing parts: -1 means "not given"
// (missing, or x, X or *).
type partial struct {
	major, minor, patch int64
	pre                 []string
	v                   Version // the filled-in version (missing parts are 0)
}

func parsePartial(s string) (partial, error) {
	p := partial{major: -1, minor: -1, patch: -1}
	core := strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	rest := ""
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core, rest = core[:i], core[i:]
	}
	parts := strings.Split(core, ".")
	if len(parts) > 3 || core == "" {
		return p, fmt.Errorf("%w: bad version %q", ErrSyntax, s)
	}
	wild := false
	got := [3]*int64{&p.major, &p.minor, &p.patch}
	var nums [3]string
	for i, part := range parts {
		if part == "x" || part == "X" || part == "*" {
			wild = true
			continue
		}
		if wild {
			return p, fmt.Errorf("%w: %q: number after wildcard", ErrSyntax, s)
		}
		n, err := parseNumber(part)
		if err != nil {
			return p, fmt.Errorf("%w: %q: %v", ErrSyntax, s, err)
		}
		*got[i] = int64(n)
		nums[i] = part
	}
	if rest != "" {
		if p.patch < 0 {
			return p, fmt.Errorf("%w: %q: pre-release needs a full version", ErrSyntax, s)
		}
		full, err := Parse(core + rest)
		if err != nil {
			return p, err
		}
		p.pre = full.Pre
		p.v = full
		return p, nil
	}
	fill := func(n int64) uint64 {
		if n < 0 {
			return 0
		}
		return uint64(n)
	}
	p.v = Version{Major: fill(p.major), Minor: fill(p.minor), Patch: fill(p.patch)}
	return p, nil
}

func (p partial) full() bool { return p.patch >= 0 }

// bump returns the version that is one step above p at the given level:
// 0 = major, 1 = minor, 2 = patch.
func (p partial) bump(level int) Version {
	switch level {
	case 0:
		return Version{Major: p.v.Major + 1}
	case 1:
		return Version{Major: p.v.Major, Minor: p.v.Minor + 1}
	}
	return Version{Major: p.v.Major, Minor: p.v.Minor, Patch: p.v.Patch + 1}
}

// level is the index of the last given part: 0 for "1", 1 for "1.2", 2 for "1.2.3".
func (p partial) level() int {
	switch {
	case p.patch >= 0:
		return 2
	case p.minor >= 0:
		return 1
	case p.major >= 0:
		return 0
	}
	return -1
}

func parseComparator(tok string) ([]comparator, error) {
	op := ""
	for _, o := range []string{">=", "<=", "!=", ">", "<", "=", "~", "^"} {
		if strings.HasPrefix(tok, o) {
			op = o
			break
		}
	}
	p, err := parsePartial(tok[len(op):])
	if err != nil {
		return nil, err
	}
	lvl := p.level()
	lo := func(o string) comparator { return comparator{o, p.v} }
	hi := func(o string, v Version) comparator { return comparator{o, v} }

	switch op {
	case "", "=":
		switch {
		case lvl < 0:
			return []comparator{lo(">=")}, nil // "*": any release
		case p.full():
			return []comparator{lo("=")}, nil
		}
		return []comparator{lo(">="), hi("<", p.bump(lvl))}, nil
	case "!=":
		if !p.full() {
			return nil, fmt.Errorf("%w: != needs a full version, got %q", ErrSyntax, tok)
		}
		return []comparator{lo("!=")}, nil
	case ">":
		if lvl < 0 {
			return nil, fmt.Errorf("%w: %q: operator needs a version", ErrSyntax, tok)
		}
		if p.full() {
			return []comparator{lo(">")}, nil
		}
		return []comparator{hi(">=", p.bump(lvl))}, nil
	case ">=":
		if lvl < 0 {
			return nil, fmt.Errorf("%w: %q: operator needs a version", ErrSyntax, tok)
		}
		return []comparator{lo(">=")}, nil
	case "<":
		if lvl < 0 {
			return nil, fmt.Errorf("%w: %q: operator needs a version", ErrSyntax, tok)
		}
		return []comparator{lo("<")}, nil
	case "<=":
		if lvl < 0 {
			return nil, fmt.Errorf("%w: %q: operator needs a version", ErrSyntax, tok)
		}
		return []comparator{lo("<=")}, nil
	case "~":
		if lvl < 0 {
			return nil, fmt.Errorf("%w: %q: operator needs a version", ErrSyntax, tok)
		}
		// ~1.2.3 and ~1.2 allow patch changes; ~1 allows minor changes.
		up := 1
		return []comparator{lo(">="), hi("<", p.bump(up))}, nil
	case "^":
		if lvl < 0 {
			return nil, fmt.Errorf("%w: %q: operator needs a version", ErrSyntax, tok)
		}
		// ^ allows changes that do not touch the major version.
		up := 0
		return []comparator{lo(">="), hi("<", p.bump(up))}, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrSyntax, tok)
}

// Check reports whether v satisfies the constraint.
//
// A version with a pre-release is only considered by a set of comparators in
// which at least one comparator carries a pre-release on the same
// MAJOR.MINOR.PATCH.
func (c *Constraint) Check(v Version) bool {
	for _, set := range c.sets {
		if setMatches(set, v) {
			return true
		}
	}
	return false
}

func setMatches(set []comparator, v Version) bool {
	for _, cmp := range set {
		if !cmp.matches(v) {
			return false
		}
	}
	return true
}
