package skillrating

import "testing"

func TestTargetAndUncertainty(t *testing.T) {
	if Target(0) != 1000 || Target(1) != 2400 || Target(0.5) != 1700 {
		t.Fatalf("target: %d %d %d", Target(0), Target(1), Target(0.5))
	}
	for n, want := range map[int]int{1: 350, 2: 247, 3: 202, 4: 175, 9: 117, 34: 60, 100: 60} {
		if got := Uncertainty(n); got != want {
			t.Fatalf("Uncertainty(%d) = %d, want %d", n, got, want)
		}
	}
	if Uncertainty(0) != 350 {
		t.Fatalf("zero runs must be max uncertainty")
	}
}

func TestApply_AveragesTargetsOnOneVersion(t *testing.T) {
	s := Apply(State{}, 1.0)
	if s.Rating != 2400 || s.Uncertainty != 350 || s.Runs != 1 || s.SumTargets != 2400 {
		t.Fatalf("first run: %+v", s)
	}
	s = Apply(s, 0.5)
	if s.Rating != 2050 || s.Uncertainty != 247 || s.Runs != 2 {
		t.Fatalf("second run: %+v", s)
	}
	s = Apply(s, 0.0)
	if s.Rating != 1700 || s.Runs != 3 {
		t.Fatalf("third run: %+v", s)
	}
}

func TestNewVersion_KeepsPriorAndResetsConfidence(t *testing.T) {
	s := Apply(Apply(State{}, 1.0), 1.0) // 2400, n=2
	v := NewVersion(s)
	if v.Runs != 0 || v.SumTargets != 0 || v.Prior == nil || *v.Prior != 2400 || v.Rating != 2400 || v.Uncertainty != 350 {
		t.Fatalf("new version: %+v", v)
	}
	first := Apply(v, 0.0) // target 1000, blended with prior 2400
	if first.Rating != 1700 || first.Runs != 1 || first.Uncertainty != 350 {
		t.Fatalf("first run on new version: %+v", first)
	}
	second := Apply(first, 0.0)
	if second.Rating != 1000 || second.Runs != 2 {
		t.Fatalf("prior must stop influencing after the first run: %+v", second)
	}
}

func TestAccessAndTier(t *testing.T) {
	cases := []struct {
		r, u int
		tier string
		ok   bool
	}{
		{2400, 350, "strong", true}, {1842, 350, "none", false}, {1850, 350, "verified", true},
		{2160, 60, "elite", true}, {2159, 60, "strong", true}, {1500, 0, "verified", true}, {1499, 0, "none", false},
	}
	for _, c := range cases {
		a := Access(c.r, c.u)
		if Tier(a) != c.tier || Verified(c.r, c.u) != c.ok {
			t.Fatalf("%d-%d: access %d tier %s verified %v", c.r, c.u, a, Tier(a), Verified(c.r, c.u))
		}
	}
}
