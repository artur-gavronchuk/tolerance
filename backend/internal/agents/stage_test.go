package agents

import (
	"testing"
	"time"
)

func TestComputeStage(t *testing.T) {
	now := time.Unix(10_000, 0)
	fresh := now.Add(-30 * time.Second)
	stale := now.Add(-3 * time.Minute)
	cases := []struct {
		name   string
		hasKey bool
		seen   *time.Time
		facts  ProofFacts
		want   string
	}{
		{"no key", false, nil, ProofFacts{}, StageRegistered},
		{"key, never seen", true, nil, ProofFacts{}, StageRegistered},
		{"stale presence", true, &stale, ProofFacts{}, StageOffline},
		{"fresh, nothing yet", true, &fresh, ProofFacts{}, StageConnected},
		{"open proof", true, &fresh, ProofFacts{HasOpen: true}, StageChecking},
		{"open proof while offline still checking", true, &stale, ProofFacts{HasOpen: true}, StageChecking},
		{"passed once", true, &fresh, ProofFacts{HasPassed: true, LastFinishedStatus: "failed"}, StageOperational},
		{"passed but offline", true, &stale, ProofFacts{HasPassed: true}, StageOffline},
		{"last failed, never passed", true, &fresh, ProofFacts{LastFinishedStatus: "failed"}, StageCheckFailed},
		{"last infra_error, never passed", true, &fresh, ProofFacts{LastFinishedStatus: "infra_error"}, StageConnected},
	}
	for _, c := range cases {
		if got := ComputeStage(c.hasKey, c.seen, now, c.facts); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}
