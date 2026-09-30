package skills

import "testing"

func TestFrozen(t *testing.T) {
	cases := []struct {
		issuable, min int
		want          bool
	}{
		{0, 5, true}, {4, 5, true}, {5, 5, false}, {9, 5, false}, {1, 1, false}, {0, 1, true},
		// A floor of zero means "do not apply the policy", but an empty pool is
		// still frozen: a run cannot be built out of no tasks, and without this
		// the insert fails on task_slugs NOT NULL and the owner gets a 500.
		{0, 0, true},
		{1, 0, false},
	}
	for _, c := range cases {
		if got := Frozen(c.issuable, c.min); got != c.want {
			t.Errorf("Frozen(%d, %d) = %v, want %v", c.issuable, c.min, got, c.want)
		}
	}
}

func TestPolicyDefaults(t *testing.T) {
	if MinPool != 5 {
		t.Errorf("MinPool = %d, want 5", MinPool)
	}
	if MaxExposures != 40 {
		t.Errorf("MaxExposures = %d, want 40", MaxExposures)
	}
}
