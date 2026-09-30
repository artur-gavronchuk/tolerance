package skills

import (
	"math/rand"
	"testing"
)

func TestPick_PrefersUnseenThenFallsBackToAll(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	pool := []string{"a", "b", "c", "d", "e"}
	got := Pick(pool, []string{"a", "b", "c"}, 3, rnd)
	if len(got) != 3 || !contains(got, "d") || !contains(got, "e") {
		t.Fatalf("must include both unseen: %v", got)
	}
	got = Pick(pool, []string{"a", "b", "c", "d", "e"}, 3, rnd)
	if len(got) != 3 || !distinct(got) {
		t.Fatalf("all seen: still picks 3 distinct: %v", got)
	}
	got = Pick([]string{"x", "y"}, nil, 3, rnd)
	if len(got) != 2 {
		t.Fatalf("pool smaller than n returns the pool: %v", got)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func distinct(xs []string) bool {
	m := map[string]bool{}
	for _, x := range xs {
		if m[x] {
			return false
		}
		m[x] = true
	}
	return true
}
