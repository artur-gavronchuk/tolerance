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
	Challenges  []ChallengePlace          `json:"challenges"`
}

// ChallengePlace is one finished competition this agent has a place in. It lives
// here, not in internal/challenges, because that package imports this one.
type ChallengePlace struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Rank  int    `json:"rank"`
	Of    int    `json:"of"`
}

// ChallengePlacesSource supplies an agent's finished challenge places
// (implemented by the challenges service).
type ChallengePlacesSource interface {
	PlacesFor(ctx context.Context, agentID string) ([]ChallengePlace, error)
}

// SetChallengePlacesSource wires where the public profile reads challenge places
// from. Without it the profile lists none.
func (s *Service) SetChallengePlacesSource(src ChallengePlacesSource) { s.places = src }

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

// PublicByName is the profile anyone can see: no email, no keys. An agent whose
// owner opted out of the public arena, or one the platform banned, has no
// profile at all — 404 rather than 403, because whether it exists is part of
// what was hidden. Its rating keeps being computed either way.
func (s *Service) PublicByName(ctx context.Context, name string) (PublicProfile, error) {
	var a Agent
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return scanAgent(tx.QueryRow(ctx, `SELECT `+agentCols+` FROM agents
			WHERE lower(name) = lower($1) AND public AND banned_at IS NULL`, name), &a)
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
	p := PublicProfile{Name: a.Name, Description: a.Description, Joined: a.CreatedAt, Stage: o.Stage, Skills: rs,
		Challenges: []ChallengePlace{}}
	if s.places != nil {
		places, err := s.places.PlacesFor(ctx, a.ID)
		if err != nil {
			return PublicProfile{}, err
		}
		if places != nil {
			p.Challenges = places
		}
	}
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
