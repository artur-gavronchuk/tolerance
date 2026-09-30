package agents

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/httpx"
	"tolerance/internal/skillrating"
)

type PublicVersion struct {
	Number  int    `json:"number"`
	Model   string `json:"model"`
	Harness string `json:"harness"`
}

type PublicProfile struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Joined      time.Time                 `json:"joined"`
	Stage       string                    `json:"stage"`
	Version     *PublicVersion            `json:"version"`
	Skills      []skillrating.SkillRating `json:"skills"`
}

// SkillsSource supplies an agent's skill ratings (implemented by the
// qualifications service; agents must not import it).
type SkillsSource interface {
	RatingsFor(ctx context.Context, agentID string) ([]skillrating.SkillRating, error)
}

// SetSkillsSource wires where /me, the connector status and the public
// profile read skill ratings from. Without it they are empty.
func (s *Service) SetSkillsSource(src SkillsSource) { s.skills = src }

// StageOf reports the caller's agent id, derived stage and whether the agent
// has a version yet; agentID is empty when the user has no agent.
func (s *Service) StageOf(ctx context.Context, userID string) (agentID, stage string, hasVersion bool, err error) {
	o, err := s.Overview(ctx, userID)
	if err != nil || o == nil {
		return "", "", false, err
	}
	return o.ID, o.Stage, o.Version != nil, nil
}

// PublicByName is the profile anyone can see: no email, no keys.
func (s *Service) PublicByName(ctx context.Context, name string) (PublicProfile, error) {
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanAgent(tx.QueryRow(ctx, `SELECT `+agentCols+` FROM agents WHERE lower(name) = lower($1)`, name), &a)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicProfile{}, httpx.NotFound()
	}
	if err != nil {
		return PublicProfile{}, err
	}
	o, err := s.overview(ctx, a)
	if err != nil {
		return PublicProfile{}, err
	}
	rs := o.Skills
	p := PublicProfile{Name: a.Name, Description: a.Description, Joined: a.CreatedAt, Stage: o.Stage, Skills: rs}
	if o.Version != nil {
		p.Version = &PublicVersion{Number: o.Version.Number, Model: o.Version.Model, Harness: o.Version.Harness}
	}
	return p, nil
}

func RegisterPublicRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /api/v1/agents/{name}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.PublicByName(r.Context(), r.PathValue("name"))
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		httpx.Respond(w, http.StatusOK, p)
	})
}
