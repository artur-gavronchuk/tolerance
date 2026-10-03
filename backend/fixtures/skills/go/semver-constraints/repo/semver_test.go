package semver

import "testing"

func mustParse(t *testing.T, s string) Version {
	t.Helper()
	v, err := Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func TestParseAndString(t *testing.T) {
	v := mustParse(t, "v1.2.3-beta.1+build.5")
	if v.Major != 1 || v.Minor != 2 || v.Patch != 3 || len(v.Pre) != 2 || v.Build != "build.5" {
		t.Fatalf("bad parse: %+v", v)
	}
	if got := v.String(); got != "1.2.3-beta.1+build.5" {
		t.Fatalf("string: %s", got)
	}
}

func TestCompareBasics(t *testing.T) {
	if mustParse(t, "1.2.3").Compare(mustParse(t, "1.10.0")) >= 0 {
		t.Fatal("1.2.3 should be below 1.10.0")
	}
	if mustParse(t, "1.0.0-alpha").Compare(mustParse(t, "1.0.0")) >= 0 {
		t.Fatal("pre-release should be below the release")
	}
}

func TestSimpleConstraints(t *testing.T) {
	c, err := ParseConstraint(">=1.2.0 <2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Check(mustParse(t, "1.5.0")) || c.Check(mustParse(t, "2.0.0")) || c.Check(mustParse(t, "1.1.9")) {
		t.Fatal("range check wrong")
	}
	c, _ = ParseConstraint("^1.2.3")
	if !c.Check(mustParse(t, "1.9.9")) || c.Check(mustParse(t, "2.0.0")) {
		t.Fatal("caret wrong")
	}
}

func TestHighest(t *testing.T) {
	got, err := Highest([]string{"1.0.0", "1.4.0", "2.0.0", "1.2.0"}, "^1.0.0")
	if err != nil || got != "1.4.0" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := Highest([]string{"1.0.0"}, ">=2.0.0"); err != ErrNoMatch {
		t.Fatalf("want ErrNoMatch, got %v", err)
	}
}
