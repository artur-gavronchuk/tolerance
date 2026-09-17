// Package budget holds spending limits. Slice 1 only needs a limit to exist
// and be readable; reservations and usage entries arrive in slice 2
// (submissions, which are what actually spend against a limit) and slice 4
// (managed execution).
package budget

import (
	"context"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type Budget struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	ScopeKind      string `json:"scope_kind"`
	ScopeID        string `json:"scope_id"`
	Currency       string `json:"currency"`
	LimitMinor     int64  `json:"limit_minor"`
	Version        int    `json:"version"`
}

var validScopeKinds = map[string]bool{"campaign": true, "entry": true, "organizer_reserve": true}

func canManage(role string) bool { return role == "owner" || role == "organizer" }

type Service struct {
	pool *db.Pool
}

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

type SetInput struct {
	ScopeKind  string `json:"scope_kind"`
	ScopeID    string `json:"scope_id"`
	Currency   string `json:"currency"`
	LimitMinor int64  `json:"limit_minor"`
}

// Set creates or updates the limit for a scope. There is deliberately no
// history of prior limits in slice 1 — audit_events already records every
// change with its actor and reason once reservations exist to make a limit
// change consequential.
func (s *Service) Set(ctx context.Context, actor identity.Actor, input SetInput) (Budget, error) {
	if !canManage(actor.Role) {
		return Budget{}, httpx.Forbidden("Лимит устанавливает владелец или организатор.")
	}
	if !validScopeKinds[input.ScopeKind] {
		return Budget{}, httpx.WithField(422, "invalid_body", "scope_kind должен быть campaign, entry или organizer_reserve.", "scope_kind", "invalid")
	}
	if !httpx.ValidText(input.ScopeID, 1, 200) {
		return Budget{}, httpx.WithField(422, "invalid_body", "Укажите scope_id.", "scope_id", "required")
	}
	if !httpx.ValidText(input.Currency, 3, 3) {
		return Budget{}, httpx.WithField(422, "invalid_body", "Укажите валюту в формате ISO 4217.", "currency", "invalid")
	}
	if input.LimitMinor < 0 {
		return Budget{}, httpx.WithField(422, "invalid_body", "Лимит не может быть отрицательным.", "limit_minor", "negative")
	}

	var b Budget
	err := s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO budgets (id, organization_id, scope_kind, scope_id, currency, limit_minor, version)
			VALUES ($1, $2, $3, $4, $5, $6, 1)
			ON CONFLICT (organization_id, scope_kind, scope_id) DO UPDATE SET
				currency = EXCLUDED.currency, limit_minor = EXCLUDED.limit_minor, version = budgets.version + 1
			RETURNING id, organization_id, scope_kind, scope_id, currency, limit_minor, version`,
			idgen.New("budget"), actor.OrganizationID, input.ScopeKind, input.ScopeID, input.Currency, input.LimitMinor)
		if err := row.Scan(&b.ID, &b.OrganizationID, &b.ScopeKind, &b.ScopeID, &b.Currency, &b.LimitMinor, &b.Version); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{OrganizationID: actor.OrganizationID, ActorID: actor.UserID,
			Action: "budget.set", AggregateKind: "budget", AggregateID: b.ID, RequestID: httpx.RequestID(ctx)})
	})
	if err != nil {
		return Budget{}, err
	}
	return b, nil
}

func (s *Service) Get(ctx context.Context, actor identity.Actor, scopeKind, scopeID string) (Budget, error) {
	var b Budget
	err := s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, organization_id, scope_kind, scope_id, currency, limit_minor, version
			FROM budgets WHERE scope_kind = $1 AND scope_id = $2`, scopeKind, scopeID).
			Scan(&b.ID, &b.OrganizationID, &b.ScopeKind, &b.ScopeID, &b.Currency, &b.LimitMinor, &b.Version)
	})
	if err == pgx.ErrNoRows {
		return Budget{}, httpx.NotFound()
	}
	if err != nil {
		return Budget{}, err
	}
	return b, nil
}
