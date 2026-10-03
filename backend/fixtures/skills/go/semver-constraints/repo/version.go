package semver

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrSyntax is returned (wrapped) for malformed versions and constraints.
var ErrSyntax = errors.New("semver: syntax error")

// Version is a parsed semantic version: MAJOR.MINOR.PATCH[-PRE][+BUILD].
type Version struct {
	Major, Minor, Patch uint64
	Pre                 []string // dot-separated pre-release identifiers
	Build               string   // build metadata, without the "+"
}

// Parse parses a version such as "1.2.3", "v1.2.3-beta.1" or "1.2.3+build.5".
func Parse(s string) (Version, error) {
	orig := s
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	var v Version
	if i := strings.IndexByte(s, '+'); i >= 0 {
		v.Build = s[i+1:]
		s = s[:i]
		if err := checkIdentifiers(v.Build, false); err != nil {
			return Version{}, fmt.Errorf("%w: %q: build metadata: %v", ErrSyntax, orig, err)
		}
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre := s[i+1:]
		s = s[:i]
		if err := checkIdentifiers(pre, true); err != nil {
			return Version{}, fmt.Errorf("%w: %q: pre-release: %v", ErrSyntax, orig, err)
		}
		v.Pre = strings.Split(pre, ".")
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("%w: %q: want MAJOR.MINOR.PATCH", ErrSyntax, orig)
	}
	var nums [3]uint64
	for i, p := range parts {
		n, err := parseNumber(p)
		if err != nil {
			return Version{}, fmt.Errorf("%w: %q: %v", ErrSyntax, orig, err)
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	return v, nil
}

func parseNumber(p string) (uint64, error) {
	if p == "" {
		return 0, errors.New("empty number")
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("bad number %q", p)
		}
	}
	return strconv.ParseUint(p, 10, 64)
}

// checkIdentifiers validates dot-separated identifiers. Numeric pre-release
// identifiers must not have leading zeros; build identifiers may.
func checkIdentifiers(s string, pre bool) error {
	if s == "" {
		return errors.New("empty")
	}
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return errors.New("empty identifier")
		}
		numeric := true
		for _, r := range id {
			switch {
			case r >= '0' && r <= '9':
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
				numeric = false
			default:
				return fmt.Errorf("bad character %q", r)
			}
		}
		if pre && numeric && len(id) > 1 && id[0] == '0' {
			return fmt.Errorf("leading zero in %q", id)
		}
	}
	return nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// Compare returns -1, 0 or 1 by semantic-version precedence.
func (v Version) Compare(o Version) int {
	if c := cmpUint(v.Major, o.Major); c != 0 {
		return c
	}
	if c := cmpUint(v.Minor, o.Minor); c != 0 {
		return c
	}
	if c := cmpUint(v.Patch, o.Patch); c != 0 {
		return c
	}
	if c := comparePre(v.Pre, o.Pre); c != 0 {
		return c
	}
	return strings.Compare(v.Build, o.Build)
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func isNumeric(id string) bool {
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return id != ""
}

func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1 // a release is higher than any of its pre-releases
	case len(b) == 0:
		return -1
	}
	return strings.Compare(strings.Join(a, "."), strings.Join(b, "."))
}
