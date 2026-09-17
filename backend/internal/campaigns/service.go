// Package campaigns implements the top-level container a mission belongs
// to: who owns it, its mode (public season vs. private trial), and its
// coarse lifecycle. It knows nothing about missions, submissions or any
// other module; those modules hold a campaign_id and look campaigns up
// through this package's Service when they need to.
package campaigns

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

type Campaign struct {
	ID                string          `json:"id"`
	OrganizationID    string          `json:"organization_id"`
	Name              string          `json:"name"`
	Slug              *string         `json:"slug,omitempty"`
	Mode              string          `json:"mode"`
	PublicationPolicy json.RawMessage `json:"publication_policy"`
	Currency          string          `json:"currency"`
	State             string          `json:"state"`
	Version           int             `json:"version"`
	CreatedAt         time.Time       `json:"created_at"`
}

const (
	StateDraft      = "draft"
	StateActive     = "active"
	StateFinalizing = "finalizing"
	StateCompleted  = "completed"
	StateCancelled  = "cancelled"
)

var validModes = map[string]bool{"public_season": true, "private_trial": true}

// canManage reports whether role may create campaigns and drive their
// lifecycle. Every other active member may only read.
func canManage(role string) bool { return role == "owner" || role == "organizer" }

type Service struct {
	pool *db.Pool
}

func NewService(pool *db.Pool) *Service { return &Service{pool: pool} }

type CreateInput struct {
	Name     string `json:"name"`
	Mode     string `json:"mode"`
	Currency string `json:"currency"`
}

func (s *Service) Create(ctx context.Context, actor identity.Actor, input CreateInput) (Campaign, error) {
	if !canManage(actor.Role) {
		return Campaign{}, httpx.Forbidden("Кампанию может создать владелец или организатор.")
	}
	if !httpx.ValidText(input.Name, 1, 200) {
		return Campaign{}, httpx.WithField(422, "invalid_body", "Укажите название до 200 байт.", "name", "required")
	}
	if !validModes[input.Mode] {
		return Campaign{}, httpx.WithField(422, "invalid_body", "Режим должен быть public_season или private_trial.", "mode", "invalid")
	}
	if !httpx.ValidText(input.Currency, 3, 3) {
		return Campaign{}, httpx.WithField(422, "invalid_body", "Укажите валюту в формате ISO 4217.", "currency", "invalid")
	}

	var c Campaign
	err := s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		c = Campaign{ID: idgen.New("campaign"), OrganizationID: actor.OrganizationID, Name: input.Name,
			Mode: input.Mode, Currency: input.Currency, State: StateDraft, Version: 1, CreatedAt: time.Now().UTC()}
		if _, err := tx.Exec(ctx, `
			INSERT INTO campaigns (id, organization_id, name, mode, currency, state, version, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			c.ID, c.OrganizationID, c.Name, c.Mode, c.Currency, c.State, c.Version, c.CreatedAt); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{OrganizationID: actor.OrganizationID, ActorID: actor.UserID,
			Action: "campaign.created", AggregateKind: "campaign", AggregateID: c.ID, RequestID: httpx.RequestID(ctx)})
	})
	if err != nil {
		return Campaign{}, err
	}
	return c, nil
}

func (s *Service) Get(ctx context.Context, actor identity.Actor, id string) (Campaign, error) {
	var c Campaign
	err := s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		return scanCampaign(tx.QueryRow(ctx, campaignColumns+` FROM campaigns WHERE id = $1`, id), &c)
	})
	if err == pgx.ErrNoRows {
		return Campaign{}, httpx.NotFound()
	}
	if err != nil {
		return Campaign{}, err
	}
	return c, nil
}

func (s *Service) List(ctx context.Context, actor identity.Actor) ([]Campaign, error) {
	var out []Campaign
	err := s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, campaignColumns+` FROM campaigns ORDER BY created_at`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Campaign
			if err := scanCampaign(rows, &c); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if out == nil {
		out = []Campaign{}
	}
	return out, err
}

// transition moves a campaign from one of `from` into `to`, enforcing
// optimistic concurrency on expectedVersion: a stale caller gets
// state_conflict rather than silently overwriting a decision made after it
// last read the campaign.
func (s *Service) transition(ctx context.Context, actor identity.Actor, id string, expectedVersion int, from []string, to, action, reason string) (Campaign, error) {
	if !canManage(actor.Role) {
		return Campaign{}, httpx.Forbidden("Действие доступно владельцу или организатору.")
	}
	var c Campaign
	err := s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanCampaign(tx.QueryRow(ctx, campaignColumns+` FROM campaigns WHERE id = $1 FOR UPDATE`, id), &c); err != nil {
			return err
		}
		if !contains(from, c.State) {
			return httpx.StateConflict("Кампания находится в состоянии " + c.State + ".")
		}
		before := c.Version
		tag, err := tx.Exec(ctx, `UPDATE campaigns SET state = $1, version = version + 1 WHERE id = $2 AND version = $3`,
			to, id, expectedVersion)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.StateConflict("Кампания изменилась, обновите данные и повторите.")
		}
		c.State, c.Version = to, expectedVersion+1
		after := c.Version
		return audit.Record(ctx, tx, audit.Event{OrganizationID: actor.OrganizationID, ActorID: actor.UserID,
			Action: action, AggregateKind: "campaign", AggregateID: id, Reason: reason,
			BeforeVersion: &before, AfterVersion: &after, RequestID: httpx.RequestID(ctx)})
	})
	if err == pgx.ErrNoRows {
		return Campaign{}, httpx.NotFound()
	}
	if err != nil {
		return Campaign{}, err
	}
	return c, nil
}

func (s *Service) Activate(ctx context.Context, actor identity.Actor, id string, expectedVersion int) (Campaign, error) {
	return s.transition(ctx, actor, id, expectedVersion, []string{StateDraft}, StateActive, "campaign.activated", "")
}

func (s *Service) Complete(ctx context.Context, actor identity.Actor, id string, expectedVersion int) (Campaign, error) {
	return s.transition(ctx, actor, id, expectedVersion, []string{StateActive, StateFinalizing}, StateCompleted, "campaign.completed", "")
}

func (s *Service) Cancel(ctx context.Context, actor identity.Actor, id string, expectedVersion int, reason string) (Campaign, error) {
	if !httpx.ValidText(reason, 1, 2000) {
		return Campaign{}, httpx.WithField(422, "invalid_body", "Укажите причину отмены.", "reason", "required")
	}
	return s.transition(ctx, actor, id, expectedVersion, []string{StateDraft, StateActive, StateFinalizing}, StateCancelled, "campaign.cancelled", reason)
}

const campaignColumns = `SELECT id, organization_id, name, slug, mode, publication_policy, currency, state, version, created_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCampaign(row rowScanner, c *Campaign) error {
	return row.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Slug, &c.Mode, &c.PublicationPolicy, &c.Currency, &c.State, &c.Version, &c.CreatedAt)
}

func contains(values []string, v string) bool {
	for _, value := range values {
		if value == v {
			return true
		}
	}
	return false
}
