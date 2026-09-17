// Package audit records what happened, who did it, and why, in the same
// transaction as the change itself. A command that changes state and fails
// to write its audit event should fail entirely, which is exactly what
// happens when Record is called before the enclosing transaction commits.
package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/idgen"
)

type Event struct {
	OrganizationID string
	ActorID        string
	ActorKind      string // "user" | "service"; defaults to "user"
	Action         string
	AggregateKind  string
	AggregateID    string
	BeforeVersion  *int
	AfterVersion   *int
	Reason         string
	Payload        any
	RequestID      string
}

func Record(ctx context.Context, tx pgx.Tx, e Event) error {
	kind := e.ActorKind
	if kind == "" {
		kind = "user"
	}
	payload := e.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events
			(id, organization_id, actor_id, actor_kind, action, aggregate_kind, aggregate_id,
			 before_version, after_version, reason, payload, request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''), $11, NULLIF($12, ''))`,
		idgen.New("audit"), e.OrganizationID, e.ActorID, kind, e.Action, e.AggregateKind, e.AggregateID,
		e.BeforeVersion, e.AfterVersion, e.Reason, body, e.RequestID)
	return err
}
