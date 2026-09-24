package main

import (
	"context"
	"net/http"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/proofs"
)

// ownerAgent is the agent block of GET /me: the agents overview plus the
// latest proof. It is put together here because agents cannot import proofs
// (proofs already depends on agents for the stage facts).
type ownerAgent struct {
	*agents.Overview
	LastProof *proofs.Proof `json:"last_proof"`
}

// meAgent feeds identity.RegisterMeRoute. It returns an untyped nil when the
// user has no agent, so "agent" encodes as JSON null.
func meAgent(as *agents.Service, ps *proofs.Service) func(context.Context, string) (any, error) {
	return func(ctx context.Context, userID string) (any, error) {
		o, err := as.Overview(ctx, userID)
		if err != nil || o == nil {
			return nil, err
		}
		last, err := ps.Latest(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		return ownerAgent{Overview: o, LastProof: last}, nil
	}
}

// connectorStatus serves GET /connector/status, what `arena status` prints.
// Unlike the heartbeat it does not touch presence: asking never makes the
// agent look online.
func connectorStatus(as *agents.Service, ps *proofs.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := identity.MustFromContext(r.Context()).AgentID
		o, err := as.OverviewByID(r.Context(), agentID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		last, err := ps.Latest(r.Context(), agentID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, map[string]any{
			"agent":      map[string]any{"id": o.ID, "name": o.Name, "stage": o.Stage},
			"last_proof": last,
		})
	}
}
