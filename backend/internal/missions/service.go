package missions

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/campaigns"
	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

func canManage(role string) bool { return role == "owner" || role == "organizer" }

type Service struct {
	pool      *db.Pool
	campaigns *campaigns.Service
}

func NewService(pool *db.Pool, campaignService *campaigns.Service) *Service {
	return &Service{pool: pool, campaigns: campaignService}
}

type RequirementInput struct {
	StableKey string `json:"stable_key"`
	Gate      bool   `json:"gate"`
	Weight    int    `json:"weight"`
	Category  string `json:"category"`
	Text      string `json:"text"`
}

type CreateInput struct {
	Stage        string             `json:"stage"`
	Title        string             `json:"title"`
	Brief        string             `json:"brief"`
	Deadline     time.Time          `json:"deadline"`
	Requirements []RequirementInput `json:"requirements"`
}

var validStages = map[string]bool{"qualification": true, "build": true, "adapt": true, "handoff": true}

func (s *Service) Create(ctx context.Context, actor identity.Actor, campaignID string, input CreateInput) (Mission, error) {
	if !canManage(actor.Role) {
		return Mission{}, httpx.Forbidden("Миссию может создать владелец или организатор.")
	}
	// Get already enforces tenant isolation (404 for a campaign in another
	// organization) and gives us the campaign's own concurrency-checked
	// state, without this module touching the campaigns table directly.
	campaign, err := s.campaigns.Get(ctx, actor, campaignID)
	if err != nil {
		return Mission{}, err
	}
	if campaign.State == campaigns.StateCancelled || campaign.State == campaigns.StateCompleted {
		return Mission{}, httpx.StateConflict("Кампания завершена или отменена.")
	}
	if !validStages[input.Stage] {
		return Mission{}, httpx.WithField(422, "invalid_body", "Этап должен быть qualification, build, adapt или handoff.", "stage", "invalid")
	}
	if !httpx.ValidText(input.Title, 1, 200) || !httpx.ValidText(input.Brief, 1, 10000) {
		return Mission{}, httpx.WithField(422, "invalid_contract", "Нужны название и бриф.", "title", "required")
	}
	if !input.Deadline.After(time.Now()) {
		return Mission{}, httpx.WithField(422, "invalid_contract", "Дедлайн должен быть в будущем.", "deadline", "past")
	}
	if err := validateRequirements(input.Requirements); err != nil {
		return Mission{}, err
	}

	var m Mission
	err = s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		var ordinal int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(ordinal), 0) + 1 FROM missions WHERE campaign_id = $1`, campaignID).Scan(&ordinal); err != nil {
			return err
		}
		m = Mission{ID: idgen.New("mission"), OrganizationID: actor.OrganizationID, CampaignID: campaignID,
			Stage: input.Stage, Ordinal: ordinal, State: StateDraft, Version: 1}
		if _, err := tx.Exec(ctx, `
			INSERT INTO missions (id, organization_id, campaign_id, stage, ordinal, state, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			m.ID, m.OrganizationID, m.CampaignID, m.Stage, m.Ordinal, m.State, m.Version); err != nil {
			return err
		}

		contractBody, err := json.Marshal(contract{Title: input.Title, Brief: input.Brief, Deadline: input.Deadline.UTC()})
		if err != nil {
			return err
		}
		version := &MissionVersion{ID: idgen.New("mv"), OrganizationID: actor.OrganizationID, MissionID: m.ID,
			Number: 1, Contract: contractBody, Policies: json.RawMessage(`{}`), Version: 1,
			SubmissionDeadline: ptrTime(input.Deadline.UTC())}
		if _, err := tx.Exec(ctx, `
			INSERT INTO mission_versions (id, organization_id, mission_id, number, contract, policies, submission_deadline, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			version.ID, version.OrganizationID, version.MissionID, version.Number, version.Contract, version.Policies,
			version.SubmissionDeadline, version.Version); err != nil {
			return err
		}
		for _, ri := range input.Requirements {
			req := Requirement{ID: idgen.New("requirement"), MissionVersionID: version.ID, StableKey: ri.StableKey,
				Revision: 1, Gate: ri.Gate, Weight: ri.Weight, Category: ri.Category, Text: ri.Text}
			if _, err := tx.Exec(ctx, `
				INSERT INTO requirements (id, organization_id, mission_version_id, stable_key, revision, gate, weight, category, text)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				req.ID, actor.OrganizationID, req.MissionVersionID, req.StableKey, req.Revision, req.Gate, req.Weight, req.Category, req.Text); err != nil {
				return err
			}
			version.Requirements = append(version.Requirements, req)
		}
		m.CurrentVersion = version

		return audit.Record(ctx, tx, audit.Event{OrganizationID: actor.OrganizationID, ActorID: actor.UserID,
			Action: "mission.created", AggregateKind: "mission", AggregateID: m.ID, RequestID: httpx.RequestID(ctx)})
	})
	if err != nil {
		return Mission{}, err
	}
	return m, nil
}

func validateRequirements(inputs []RequirementInput) error {
	if len(inputs) == 0 {
		return httpx.WithField(422, "invalid_contract", "Нужно хотя бы одно требование.", "requirements", "required")
	}
	seen := map[string]bool{}
	for _, r := range inputs {
		if !httpx.ValidText(r.StableKey, 1, 64) {
			return httpx.WithField(422, "invalid_contract", "У требования должен быть stable_key.", "requirements[].stable_key", "required")
		}
		if seen[r.StableKey] {
			return httpx.WithField(422, "invalid_contract", "Повторяющийся stable_key: "+r.StableKey, "requirements[].stable_key", "duplicate")
		}
		seen[r.StableKey] = true
		if !httpx.ValidText(r.Category, 1, 64) || !httpx.ValidText(r.Text, 1, 2000) {
			return httpx.WithField(422, "invalid_contract", "У требования должны быть категория и текст.", "requirements[]", "required")
		}
	}
	return nil
}

func (s *Service) Get(ctx context.Context, actor identity.Actor, id string) (Mission, error) {
	var m Mission
	err := s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanMission(tx.QueryRow(ctx, missionColumns+` FROM missions WHERE id = $1`, id), &m); err != nil {
			return err
		}
		version, err := loadLatestVersion(ctx, tx, m.ID)
		if err != nil {
			return err
		}
		m.CurrentVersion = version
		return nil
	})
	if err == pgx.ErrNoRows {
		return Mission{}, httpx.NotFound()
	}
	if err != nil {
		return Mission{}, err
	}
	return m, nil
}

// ConfirmCalibration is slice 1's stand-in for scenario-based calibration
// (see docs/plans/slice-1-contract-and-access.md): an organizer's explicit
// confirmation gates calibrating → open until slice 2 wires up a real
// evaluator to run the reference submission and intentional breaks against.
func (s *Service) ConfirmCalibration(ctx context.Context, actor identity.Actor, missionID string) (Mission, error) {
	if !canManage(actor.Role) {
		return Mission{}, httpx.Forbidden("Калибровку подтверждает организатор.")
	}
	var m Mission
	err := s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanMission(tx.QueryRow(ctx, missionColumns+` FROM missions WHERE id = $1 FOR UPDATE`, missionID), &m); err != nil {
			return err
		}
		if m.State != StateDraft {
			return httpx.StateConflict("Подтвердить калибровку можно только для черновика.")
		}
		version, err := loadLatestVersion(ctx, tx, m.ID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE mission_versions SET calibration_confirmed_at = $1, calibration_confirmed_by = $2 WHERE id = $3`,
			now, actor.UserID, version.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE missions SET state = $1, version = version + 1 WHERE id = $2`, StateCalibrating, m.ID); err != nil {
			return err
		}
		m.State, m.Version = StateCalibrating, m.Version+1
		version.CalibrationConfirmedAt, version.CalibrationConfirmedBy = &now, &actor.UserID
		m.CurrentVersion = version
		return audit.Record(ctx, tx, audit.Event{OrganizationID: actor.OrganizationID, ActorID: actor.UserID,
			Action: "mission.calibration_confirmed", AggregateKind: "mission", AggregateID: m.ID, RequestID: httpx.RequestID(ctx)})
	})
	if err == pgx.ErrNoRows {
		return Mission{}, httpx.NotFound()
	}
	if err != nil {
		return Mission{}, err
	}
	return m, nil
}

// Open freezes the mission's current version: from this instant its
// contract and requirements are immutable (enforced independently by the
// database trigger from migration 00004), and its contract_digest lets
// anyone holding the same contract verify they match, byte for byte.
func (s *Service) Open(ctx context.Context, actor identity.Actor, missionID string, expectedVersion int) (Mission, error) {
	if !canManage(actor.Role) {
		return Mission{}, httpx.Forbidden("Миссию открывает владелец или организатор.")
	}
	var m Mission
	err := s.pool.Tx(ctx, actor.OrganizationID, actor.UserID, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanMission(tx.QueryRow(ctx, missionColumns+` FROM missions WHERE id = $1 FOR UPDATE`, missionID), &m); err != nil {
			return err
		}
		if m.State != StateCalibrating {
			return httpx.StateConflict("Открыть можно только миссию с подтверждённой калибровкой.")
		}
		version, err := loadLatestVersion(ctx, tx, m.ID)
		if err != nil {
			return err
		}
		if version.CalibrationConfirmedAt == nil {
			return httpx.StateConflict("Калибровка ещё не подтверждена.")
		}

		var c contract
		if err := json.Unmarshal(version.Contract, &c); err != nil {
			return err
		}
		reqDigests := make([]requirementDigest, len(version.Requirements))
		for i, r := range version.Requirements {
			reqDigests[i] = requirementDigest{StableKey: r.StableKey, Revision: r.Revision, Gate: r.Gate,
				Weight: r.Weight, Category: r.Category, Text: r.Text}
		}
		sort.Slice(reqDigests, func(i, j int) bool { return reqDigests[i].StableKey < reqDigests[j].StableKey })
		payload, err := json.Marshal(digestPayload{MissionID: m.ID, Number: version.Number, Contract: c,
			Requirements: reqDigests, EvaluatorVersion: evaluatorVersion})
		if err != nil {
			return err
		}
		digest := idgen.Digest(payload)
		now := time.Now().UTC()

		if _, err := tx.Exec(ctx, `UPDATE mission_versions SET contract_digest = $1, published_at = $2 WHERE id = $3`,
			digest, now, version.ID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE missions SET state = $1, active_version_id = $2, version = version + 1
			WHERE id = $3 AND version = $4`,
			StateOpen, version.ID, m.ID, expectedVersion)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.StateConflict("Миссия изменилась, обновите данные и повторите.")
		}
		m.State, m.ActiveVersionID, m.Version = StateOpen, &version.ID, expectedVersion+1
		version.ContractDigest, version.PublishedAt = &digest, &now
		m.CurrentVersion = version
		return audit.Record(ctx, tx, audit.Event{OrganizationID: actor.OrganizationID, ActorID: actor.UserID,
			Action: "mission.opened", AggregateKind: "mission", AggregateID: m.ID, RequestID: httpx.RequestID(ctx)})
	})
	if err == pgx.ErrNoRows {
		return Mission{}, httpx.NotFound()
	}
	if err != nil {
		return Mission{}, err
	}
	return m, nil
}

const missionColumns = `SELECT id, organization_id, campaign_id, stage, ordinal, state, active_version_id, version`

func scanMission(row pgx.Row, m *Mission) error {
	return row.Scan(&m.ID, &m.OrganizationID, &m.CampaignID, &m.Stage, &m.Ordinal, &m.State, &m.ActiveVersionID, &m.Version)
}

const versionColumns = `SELECT id, organization_id, mission_id, number, parent_version_id, contract, contract_digest,
	calibration_confirmed_at, calibration_confirmed_by, published_at, submission_deadline, appeal_deadline, policies, version`

func loadLatestVersion(ctx context.Context, tx pgx.Tx, missionID string) (*MissionVersion, error) {
	var v MissionVersion
	err := tx.QueryRow(ctx, versionColumns+` FROM mission_versions WHERE mission_id = $1 ORDER BY number DESC LIMIT 1`, missionID).
		Scan(&v.ID, &v.OrganizationID, &v.MissionID, &v.Number, &v.ParentVersionID, &v.Contract, &v.ContractDigest,
			&v.CalibrationConfirmedAt, &v.CalibrationConfirmedBy, &v.PublishedAt, &v.SubmissionDeadline, &v.AppealDeadline,
			&v.Policies, &v.Version)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, mission_version_id, stable_key, revision, gate, weight, category, text
		FROM requirements WHERE mission_version_id = $1 ORDER BY stable_key`, v.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r Requirement
		if err := rows.Scan(&r.ID, &r.MissionVersionID, &r.StableKey, &r.Revision, &r.Gate, &r.Weight, &r.Category, &r.Text); err != nil {
			return nil, err
		}
		v.Requirements = append(v.Requirements, r)
	}
	return &v, rows.Err()
}

func ptrTime(t time.Time) *time.Time { return &t }
