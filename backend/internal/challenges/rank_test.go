package challenges

import (
	"testing"
	"time"
)

func at(min int) time.Time { return time.Date(2026, 10, 1, 12, min, 0, 0, time.UTC) }

func TestRankOrdersByScoreThenDiffThenTime(t *testing.T) {
	got := Rank([]Result{
		{AgentName: "slow-but-right", Score: 1, DiffLines: 40, SubmittedAt: at(50)},
		{AgentName: "tidy", Score: 1, DiffLines: 12, SubmittedAt: at(55)},
		{AgentName: "early-tie", Score: 1, DiffLines: 12, SubmittedAt: at(30)},
		{AgentName: "partial", Score: 0.5, DiffLines: 3, SubmittedAt: at(10)},
		{AgentName: "zero", Score: 0, DiffLines: 0, SubmittedAt: at(5)},
	})
	want := []struct {
		name string
		rank int
	}{{"early-tie", 1}, {"tidy", 2}, {"slow-but-right", 3}, {"partial", 4}, {"zero", 5}}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].AgentName != w.name || got[i].Rank != w.rank {
			t.Fatalf("position %d = %s/%d, want %s/%d", i, got[i].AgentName, got[i].Rank, w.name, w.rank)
		}
	}
}

func TestRankSharesAPlaceOnAFullTie(t *testing.T) {
	got := Rank([]Result{
		{AgentName: "a", Score: 1, DiffLines: 10, SubmittedAt: at(20)},
		{AgentName: "b", Score: 1, DiffLines: 10, SubmittedAt: at(20)},
		{AgentName: "c", Score: 0.9, DiffLines: 5, SubmittedAt: at(1)},
	})
	// Standard competition ranking: two firsts, then third. Sharing a place is
	// what entries equal on every key get, and the next place is skipped so
	// "third" still means "two ahead of you".
	if got[0].Rank != 1 || got[1].Rank != 1 || got[2].Rank != 3 {
		t.Fatalf("ranks = %d, %d, %d, want 1, 1, 3", got[0].Rank, got[1].Rank, got[2].Rank)
	}
}

func TestRankDoesNotMutateItsInput(t *testing.T) {
	in := []Result{
		{AgentName: "second", Score: 0.5, DiffLines: 1, SubmittedAt: at(1)},
		{AgentName: "first", Score: 1, DiffLines: 1, SubmittedAt: at(2)},
	}
	_ = Rank(in)
	if in[0].AgentName != "second" || in[0].Rank != 0 {
		t.Fatalf("Rank rewrote its argument: %+v", in)
	}
}

func TestRankEmpty(t *testing.T) {
	if got := Rank(nil); len(got) != 0 {
		t.Fatalf("Rank(nil) = %v, want empty", got)
	}
}
