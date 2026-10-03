package semver

import (
	"errors"
	"reflect"
	"testing"
)

func mp(t *testing.T, s string) Version {
	t.Helper()
	v, err := Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func TestHiddenPrereleasePrecedence(t *testing.T) {
	chain := []string{
		"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta",
		"1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0",
	}
	for i := range chain {
		for j := range chain {
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := mp(t, chain[i]).Compare(mp(t, chain[j])); got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", chain[i], chain[j], got, want)
			}
		}
	}
	// numeric identifiers beyond uint64 still compare as numbers
	if mp(t, "1.0.0-99999999999999999999999").Compare(mp(t, "1.0.0-100000000000000000000000")) >= 0 {
		t.Error("long numeric identifiers compared as text")
	}
	// hyphens inside identifiers are ordinary alphanumerics
	if mp(t, "1.0.0-a-b").Compare(mp(t, "1.0.0-a")) <= 0 {
		t.Error("a-b should be above a")
	}
	// build metadata does not take part in precedence
	if got := mp(t, "1.0.0+a").Compare(mp(t, "1.0.0+b")); got != 0 {
		t.Errorf("build metadata affected precedence: %d", got)
	}
	if got := mp(t, "1.0.0-rc.1+x").Compare(mp(t, "1.0.0-rc.1+y")); got != 0 {
		t.Errorf("build metadata affected pre-release precedence: %d", got)
	}
}

func TestHiddenParseIsStrict(t *testing.T) {
	bad := []string{"01.2.3", "1.02.3", "1.2.03", "1.2.3-01", "1.2.3-alpha.01", "1.2", "1.2.3.4",
		"1.2.3-", "1.2.3-a..b", "1.2.3+", "1.2.3+a..b", "1.2.x", "", "1.2.3-é", "-1.2.3",
		"99999999999999999999.0.0"}
	for _, s := range bad {
		if _, err := Parse(s); !errors.Is(err, ErrSyntax) {
			t.Errorf("Parse(%q): want ErrSyntax, got %v", s, err)
		}
	}
	good := []string{"0.0.0", "1.2.3-0", "v1.2.3+001", "1.2.3-x-y.z", "1.2.3-0a", "V2.0.0", "1.2.3-alpha.0.1+b-1.2"}
	for _, s := range good {
		if _, err := Parse(s); err != nil {
			t.Errorf("Parse(%q): %v", s, err)
		}
	}
}

func check(t *testing.T, constraint string, in, out []string) {
	t.Helper()
	c, err := ParseConstraint(constraint)
	if err != nil {
		t.Errorf("ParseConstraint(%q): %v", constraint, err)
		return
	}
	for _, s := range in {
		if !c.Check(mp(t, s)) {
			t.Errorf("%q should accept %s", constraint, s)
		}
	}
	for _, s := range out {
		if c.Check(mp(t, s)) {
			t.Errorf("%q should reject %s", constraint, s)
		}
	}
}

func TestHiddenCaretAndTilde(t *testing.T) {
	check(t, "^1.2.3", []string{"1.2.3", "1.2.4", "1.9.0", "1.99.99"}, []string{"1.2.2", "2.0.0", "0.9.9"})
	check(t, "^0.2.3", []string{"0.2.3", "0.2.9"}, []string{"0.2.2", "0.3.0", "1.0.0"})
	check(t, "^0.0.3", []string{"0.0.3"}, []string{"0.0.2", "0.0.4", "0.1.0"})
	check(t, "^0.0", []string{"0.0.0", "0.0.9"}, []string{"0.1.0"})
	check(t, "^0", []string{"0.0.0", "0.9.9"}, []string{"1.0.0"})
	check(t, "^1.2", []string{"1.2.0", "1.9.9"}, []string{"1.1.9", "2.0.0"})
	check(t, "^0.x", []string{"0.0.1", "0.8.0"}, []string{"1.0.0"})
	check(t, "~1.2.3", []string{"1.2.3", "1.2.99"}, []string{"1.2.2", "1.3.0"})
	check(t, "~1.2", []string{"1.2.0", "1.2.7"}, []string{"1.1.9", "1.3.0"})
	check(t, "~1", []string{"1.0.0", "1.9.9"}, []string{"0.9.9", "2.0.0"})
	check(t, "~0.0.1", []string{"0.0.1", "0.0.9"}, []string{"0.1.0"})
	check(t, "^ 1.2.3", []string{"1.5.0"}, []string{"2.0.0"})
}

func TestHiddenPartialsAndWildcards(t *testing.T) {
	check(t, "<=1.2", []string{"1.2.0", "1.2.9", "0.1.0"}, []string{"1.3.0"})
	check(t, "<=1", []string{"1.9.9"}, []string{"2.0.0"})
	check(t, "<=1.2.3", []string{"1.2.3"}, []string{"1.2.4"})
	check(t, ">1.2", []string{"1.3.0", "2.0.0"}, []string{"1.2.9", "1.2.0"})
	check(t, ">1", []string{"2.0.0"}, []string{"1.9.9"})
	check(t, ">=1.2", []string{"1.2.0"}, []string{"1.1.9"})
	check(t, "<1.2", []string{"1.1.9"}, []string{"1.2.0"})
	check(t, "=1.2", []string{"1.2.0", "1.2.5"}, []string{"1.3.0", "1.1.0"})
	check(t, "1.2", []string{"1.2.5"}, []string{"1.3.0"})
	check(t, "1.x", []string{"1.0.0", "1.9.9"}, []string{"2.0.0", "0.9.9"})
	check(t, "1.2.*", []string{"1.2.0", "1.2.8"}, []string{"1.3.0"})
	check(t, "*", []string{"0.0.0", "7.8.9"}, nil)
	check(t, "x", []string{"3.0.0"}, nil)
	check(t, "=1.2.3", []string{"1.2.3", "1.2.3+build.9"}, []string{"1.2.4"})
	check(t, "!=1.2.3", []string{"1.2.4", "1.2.2"}, []string{"1.2.3", "1.2.3+b"})
	check(t, ">= 1.2.3, < 2.0.0", []string{"1.2.3", "1.9.0"}, []string{"1.2.2", "2.0.0"})
	check(t, ">=1.0.0 <1.2.0 || >=2.0.0 <2.1.0", []string{"1.1.0", "2.0.5"}, []string{"1.2.0", "1.9.0", "2.1.0"})
	check(t, "<1.0.0 || ^3.0.0", []string{"0.5.0", "3.4.0"}, []string{"1.0.0", "2.0.0", "4.0.0"})

	for _, s := range []string{"", "||", "1.0.0 ||", ">=", "!=1.2", ">*", "~", "1.x.3", "1.2.3.4", "foo", ">=1.2.3 foo", "1.x-beta", "^01.2.3"} {
		if _, err := ParseConstraint(s); !errors.Is(err, ErrSyntax) {
			t.Errorf("ParseConstraint(%q): want ErrSyntax, got %v", s, err)
		}
	}
}

func TestHiddenPrereleaseRule(t *testing.T) {
	// A pre-release only satisfies a comparator set that names a pre-release of the same x.y.z.
	check(t, ">=1.2.3", []string{"1.2.3", "2.0.0"}, []string{"1.2.4-alpha", "2.0.0-rc.1", "3.0.0-beta"})
	check(t, "^1.2.3", []string{"1.4.0"}, []string{"1.4.0-beta", "1.2.4-alpha", "2.0.0-alpha"})
	check(t, ">=1.2.3-beta.2 <2.0.0", []string{"1.2.3-beta.2", "1.2.3-beta.11", "1.2.3-rc.1", "1.2.3", "1.5.0"},
		[]string{"1.2.3-beta.1", "1.2.3-alpha", "1.3.0-alpha", "2.0.0-alpha"})
	check(t, "1.2.3-rc.1", []string{"1.2.3-rc.1"}, []string{"1.2.3-rc.2", "1.2.3"})
	check(t, "~1.2.3-beta.1", []string{"1.2.3-beta.5", "1.2.9"}, []string{"1.2.4-beta.1", "1.3.0"})
	// the rule applies per set: the second set has no pre-release comparator
	check(t, ">=1.0.0-beta <1.0.0 || >=1.0.0 <2.0.0", []string{"1.0.0-beta.3", "1.0.0", "1.5.0"}, []string{"1.5.0-beta", "2.0.0-beta"})
	check(t, "*", nil, []string{"1.0.0-alpha"})
	check(t, "!=1.0.0", []string{"1.0.1"}, []string{"1.0.0-alpha"})
}

func TestHiddenHighestAndMatching(t *testing.T) {
	vs := []string{"1.2.0", "1.10.0", "not-a-version", "1.9.0+z", "1.9.0+a", "2.0.0", "1.10.0-rc.1", "1.9", "v1.10.0+meta"}
	got, err := Highest(vs, "^1.0.0")
	if err != nil || got != "1.10.0" {
		t.Fatalf("Highest = %q, %v; want 1.10.0 (first of the ties)", got, err)
	}
	m, err := Matching(vs, "^1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.2.0", "1.9.0+z", "1.9.0+a", "1.10.0", "v1.10.0+meta"}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("Matching = %v, want %v", m, want)
	}
	got, err = Highest([]string{"2.0.0-rc.1", "1.0.0"}, "<2.0.0")
	if err != nil || got != "1.0.0" {
		t.Fatalf("Highest = %q, %v", got, err)
	}
	got, err = Highest([]string{"2.0.0-rc.1", "2.0.0-rc.10", "2.0.0-rc.2"}, ">=2.0.0-rc.1")
	if err != nil || got != "2.0.0-rc.10" {
		t.Fatalf("Highest = %q, %v", got, err)
	}
	if _, err := Highest([]string{"1.0.0"}, ">=2"); err != ErrNoMatch {
		t.Fatalf("want ErrNoMatch, got %v", err)
	}
	if _, err := Highest([]string{"1.0.0"}, "nonsense"); !errors.Is(err, ErrSyntax) {
		t.Fatalf("want ErrSyntax, got %v", err)
	}
	if _, err := Highest(nil, "*"); err != ErrNoMatch {
		t.Fatalf("empty list: %v", err)
	}
}
