package agents

import (
	"context"
	"time"
)

const (
	StageRegistered  = "registered"
	StageOffline     = "offline"
	StageConnected   = "connected"
	StageChecking    = "checking"
	StageOperational = "operational"
	StageCheckFailed = "check_failed"

	presenceTTL = 2 * time.Minute
)

// ProofFacts is what the stage needs to know about an agent's proofs. The
// proofs module implements ProofFactsSource; agents does not import it.
type ProofFacts struct {
	HasPassed          bool
	HasOpen            bool
	LastFinishedStatus string // "" | passed | failed | infra_error | expired
}

type ProofFactsSource interface {
	ProofFacts(ctx context.Context, agentID string) (ProofFacts, error)
}

// NoProofFacts is the source used before the proofs module is wired.
type NoProofFacts struct{}

func (NoProofFacts) ProofFacts(context.Context, string) (ProofFacts, error) { return ProofFacts{}, nil }

// ComputeStage is the single definition of an agent's stage (spec §3.3).
// A proof in flight keeps the agent in "checking" even if the connector
// went quiet: the proof will expire on its own and the stage will follow.
func ComputeStage(hasKey bool, lastSeen *time.Time, now time.Time, f ProofFacts) string {
	if f.HasOpen {
		return StageChecking
	}
	if !hasKey || lastSeen == nil {
		return StageRegistered
	}
	if now.Sub(*lastSeen) > presenceTTL {
		return StageOffline
	}
	if f.HasPassed {
		return StageOperational
	}
	if f.LastFinishedStatus == "failed" {
		return StageCheckFailed
	}
	return StageConnected
}
