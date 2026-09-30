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

func TestRemoveIsInverseOfApply(t *testing.T) {
	prior := 1700
	for _, base := range []State{
		{Uncertainty: MaxUncert},
		// The only state with a prior the system can actually be in is the one
		// NewVersion produces, where Rating already equals the prior.
		{Rating: prior, Uncertainty: MaxUncert, Prior: &prior},
	} {
		for _, scores := range [][]float64{{0.5}, {0.5, 1}, {0, 0.25, 0.75}} {
			got := base
			for _, sc := range scores {
				got = Apply(got, sc)
			}
			got = Remove(got, Target(scores[len(scores)-1]))
			want := base
			for _, sc := range scores[:len(scores)-1] {
				want = Apply(want, sc)
			}
			if got.Runs != want.Runs || got.SumTargets != want.SumTargets ||
				got.Rating != want.Rating || got.Uncertainty != want.Uncertainty {
				t.Errorf("Remove after %v = %+v, want %+v", scores, got, want)
			}
		}
	}
}

func TestRemoveLastRunFallsBackToPrior(t *testing.T) {
	prior := 1900
	s := Apply(State{Uncertainty: MaxUncert, Prior: &prior}, 0.1)
	got := Remove(s, Target(0.1))
	if got.Runs != 0 || got.SumTargets != 0 || got.Rating != prior || got.Uncertainty != MaxUncert {
		t.Fatalf("Remove of the only run = %+v, want runs 0, sum 0, rating %d, uncertainty %d", got, prior, MaxUncert)
	}
}

func TestRemoveOnEmptyStateIsNoop(t *testing.T) {
	s := State{Rating: 1500, Uncertainty: MaxUncert}
	if got := Remove(s, 1800); got != s {
		t.Fatalf("Remove on zero runs changed the state: %+v", got)
	}
}

func TestUnratedAfterVoidingTheOnlyRunOfAFirstVersion(t *testing.T) {
	s := Apply(State{Uncertainty: MaxUncert}, 0.5)
	if Unrated(s) {
		t.Fatal("a scored run is evidence; the state is not unrated")
	}
	got := Remove(s, Target(0.5))
	if !Unrated(got) {
		t.Fatalf("after voiding the only run of a first version the agent is unrated again: %+v", got)
	}
	if got.Rating != 0 {
		t.Errorf("rating = %d, want 0: no run supports a number any more", got.Rating)
	}
	prior := 1900
	if Unrated(Remove(Apply(State{Rating: prior, Uncertainty: MaxUncert, Prior: &prior}, 0.5), Target(0.5))) {
		t.Error("a state that still carries a prior version's rating is not unrated")
	}
}
