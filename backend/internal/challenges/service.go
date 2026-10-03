package challenges

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/proofs"
	"tolerance/internal/skillrating"
)

type Service struct {
	pool   *db.Pool
	proofs *proofs.Service
	logger *slog.Logger
}

func NewService(pool *db.Pool, ps *proofs.Service) *Service {
	return &Service{pool: pool, proofs: ps}
}

const challengeCols = `id, slug, title, summary, skill_task_slug, min_tier, opens_at, closes_at, status, prizes, publish_tests, payout_note, created_by, created_at`

func scanChallenge(row interface{ Scan(...any) error }, c *Challenge) error {
	if err := row.Scan(&c.ID, &c.Slug, &c.Title, &c.Summary, &c.SkillTaskSlug, &c.MinTier, &c.OpensAt, &c.ClosesAt,
		&c.Status, &c.Prizes, &c.PublishTests, &c.PayoutNote, &c.CreatedBy, &c.CreatedAt); err != nil {
		return err
	}
	c.OpensAt, c.ClosesAt, c.CreatedAt = c.OpensAt.UTC(), c.ClosesAt.UTC(), c.CreatedAt.UTC()
	return nil
}

// Create registers a challenge in draft. It refuses a prize challenge on a skill
// whose verdict is computed inside the agent's own process: places are worth what
// the verdict is worth, and there is no point paying for one a diff can forge.
func (s *Service) Create(ctx context.Context, actorID string, in NewInput) (Challenge, error) {
	in.Slug, in.Title = strings.TrimSpace(in.Slug), strings.TrimSpace(in.Title)
	switch {
	case in.Slug == "" || strings.ContainsAny(in.Slug, " /?#"):
		return Challenge{}, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed", "slug must be a non-empty url-safe string", "slug", "invalid")
	case in.Title == "":
		return Challenge{}, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed", "title is required", "title", "required")
	case !in.ClosesAt.After(in.OpensAt):
		return Challenge{}, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed", "closes_at must be after opens_at", "closes_at", "invalid")
	}
	if in.MinTier == "" {
		in.MinTier = "verified"
	}
	if _, ok := tierRank[in.MinTier]; !ok {
		return Challenge{}, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed", "unknown min_tier", "min_tier", "invalid")
	}
	publishTests := true
	if in.PublishTests != nil {
		publishTests = *in.PublishTests
	}

	var c Challenge
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var language string
		err := tx.QueryRow(ctx, `SELECT s.language FROM skill_tasks t JOIN skills s ON s.slug = t.skill_slug
			WHERE t.slug = $1`, in.SkillTaskSlug).Scan(&language)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		if err := checkPrizeLanguage(in.Prizes, language); err != nil {
			return err
		}
		// Claiming the task is part of creating the challenge: a task two agents
		// meet in a competition must not also be handed out for qualification.
		// Nothing un-reserves it, because the challenge burns it either way.
		if _, err := tx.Exec(ctx, `UPDATE skill_tasks SET challenge_only = true WHERE slug = $1`, in.SkillTaskSlug); err != nil {
			return err
		}
		if err := scanChallenge(tx.QueryRow(ctx, `INSERT INTO challenges
			(id, slug, title, summary, skill_task_slug, min_tier, opens_at, closes_at, prizes, publish_tests, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING `+challengeCols,
			idgen.New("chal"), in.Slug, in.Title, in.Summary, in.SkillTaskSlug, in.MinTier,
			in.OpensAt.UTC(), in.ClosesAt.UTC(), in.Prizes, publishTests, actorID), &c); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: actorID, ActorKind: identity.KindUser, Action: "admin.challenge_created",
			AggregateKind: "challenge", AggregateID: c.ID, Payload: map[string]any{"slug": c.Slug}, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Challenge{}, httpx.New(http.StatusConflict, "slug_taken", "A challenge with this slug already exists")
	}
	return c, err
}

// checkPrizeLanguage refuses prize money on a skill whose verdict is still
// computed inside the agent's own process. It is a function rather than two
// copies because the rule has to hold on every path that can set prizes — the
// spec puts it in the design doc precisely so it cannot be quietly stepped around.
func checkPrizeLanguage(prizes, language string) error {
	if strings.TrimSpace(prizes) == "" || !inProcessVerdictLanguages[language] {
		return nil
	}
	return httpx.WithField(http.StatusUnprocessableEntity, "prizes_not_allowed_for_language",
		"A prize challenge needs a verdict the entrant's own diff cannot reach. For "+language+
			" the test harness still runs in the agent's process, so run this one without prizes.",
		"prizes", "unsupported_language")
}

// Enter records one agent's single attempt and queues its proof. The entry key
// is (challenge, agent), so the one-attempt rule is the schema's, not a check
// this code could forget.
func (s *Service) Enter(ctx context.Context, userID, slug string, consentPublish bool) (Entry, error) {
	var e Entry
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var agentID, versionID string
		var banned *time.Time
		var current *string
		err := tx.QueryRow(ctx, `SELECT id, banned_at, current_version_id FROM agents WHERE owner_user_id = $1 FOR UPDATE`, userID).
			Scan(&agentID, &banned, &current)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.New(http.StatusNotFound, "no_agent", "Create an agent first")
		}
		if err != nil {
			return err
		}
		if banned != nil {
			return httpx.New(http.StatusForbidden, "agent_banned", "This agent is banned from the arena")
		}
		if current == nil {
			return httpx.New(http.StatusConflict, "no_version", "The connector has not reported the agent version yet; update it and run `arena connect`")
		}
		versionID = *current

		var c Challenge
		if err := scanChallenge(tx.QueryRow(ctx, `SELECT `+challengeCols+` FROM challenges WHERE slug = $1 FOR UPDATE`, slug), &c); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.NotFound()
			}
			return err
		}
		// Status and clock both have to agree. An administrator may open a
		// challenge early, and until opens_at the public pages label it "Opens
		// <date>" — taking an entry then hands that agent extra hours against a
		// fixed deadline on a one-attempt competition.
		if c.Status != StatusOpen || c.OpensAt.After(time.Now().UTC()) {
			return httpx.New(http.StatusConflict, "challenge_not_open", "This challenge is not open for entries")
		}

		// Same bar as a qualification run: an agent that has never proved it can
		// work on its own does not get to burn a slot and pad the entrant count.
		var passed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM proofs WHERE agent_id = $1 AND kind = 'proof' AND status = 'passed')`, agentID).Scan(&passed); err != nil {
			return err
		}
		if !passed {
			return httpx.New(http.StatusConflict, "agent_not_operational", "Pass the basic proof first")
		}

		// Same rule as a qualification run: an offline connector would let the
		// task expire unclaimed and burn the agent's single attempt.
		var online bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM agent_presence WHERE agent_id = $1 AND last_seen_at > now() - make_interval(secs => $2))`,
			agentID, agents.PresenceTTL.Seconds()).Scan(&online); err != nil {
			return err
		}
		if !online {
			return httpx.New(http.StatusConflict, "agent_offline", "The connector is not online; run `arena connect` first")
		}

		if err := s.checkTier(ctx, tx, agentID, versionID, c); err != nil {
			return err
		}

		p, err := s.proofs.CreateChallengeProof(ctx, tx, agentID, c.ID, c.SkillTaskSlug)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO challenge_entries (challenge_id, agent_id, version_id, proof_id, consent_publish)
			VALUES ($1, $2, $3, $4, $5)`, c.ID, agentID, versionID, p.ID, consentPublish); err != nil {
			return err
		}
		e = Entry{ChallengeSlug: c.Slug, Title: c.Title, AgentID: agentID, VersionID: versionID,
			ProofID: p.ID, ConsentPublish: consentPublish, CreatedAt: p.CreatedAt}
		return audit.Record(ctx, tx, audit.Event{ActorID: userID, ActorKind: identity.KindUser, Action: "challenge.entered",
			AggregateKind: "challenge", AggregateID: c.ID,
			Payload: map[string]any{"agent_id": agentID, "proof_id": p.ID}, RequestID: httpx.RequestID(ctx)})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if strings.Contains(pgErr.ConstraintName, "challenge_entries_pkey") {
			return Entry{}, httpx.New(http.StatusConflict, "already_entered", "A challenge gives each agent one attempt")
		}
		if strings.Contains(pgErr.ConstraintName, "proofs_one_open_idx") {
			return Entry{}, httpx.New(http.StatusConflict, "proof_in_progress", "This agent already has a proof in progress; wait for it to finish")
		}
	}
	return e, err
}

// checkTier enforces the entry bar against the agent's *current* version: a
// rating earned by a configuration the owner has since replaced must not open the
// door for the new one.
func (s *Service) checkTier(ctx context.Context, tx pgx.Tx, agentID, versionID string, c Challenge) error {
	need := tierRank[c.MinTier]
	if need == 0 {
		return nil
	}
	var rating, uncertainty int
	err := tx.QueryRow(ctx, `SELECT r.rating, r.uncertainty FROM skill_ratings r
		JOIN skill_tasks t ON t.skill_slug = r.skill_slug
		WHERE r.agent_id = $1 AND t.slug = $2 AND r.version_id = $3`, agentID, c.SkillTaskSlug, versionID).
		Scan(&rating, &uncertainty)
	if errors.Is(err, pgx.ErrNoRows) {
		return tierTooLow(c.MinTier)
	}
	if err != nil {
		return err
	}
	if tierRank[skillrating.Tier(skillrating.Access(rating, uncertainty))] < need {
		return tierTooLow(c.MinTier)
	}
	return nil
}

func tierTooLow(min string) error {
	return httpx.New(http.StatusForbidden, "tier_too_low",
		"This challenge is open to agents rated "+min+" or better on its skill, on their current version")
}

// MyEntries lists the caller's own entries, newest first.
func (s *Service) MyEntries(ctx context.Context, userID string) ([]Entry, error) {
	out := []Entry{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT c.slug, c.title, e.agent_id, e.version_id, e.proof_id, e.consent_publish,
				e.score::float8, e.diff_lines, e.submitted_at, e.rank, e.created_at
			FROM challenge_entries e JOIN challenges c ON c.id = e.challenge_id
			JOIN agents a ON a.id = e.agent_id
			WHERE a.owner_user_id = $1 ORDER BY e.created_at DESC`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e Entry
			if err := rows.Scan(&e.ChallengeSlug, &e.Title, &e.AgentID, &e.VersionID, &e.ProofID, &e.ConsentPublish,
				&e.Score, &e.DiffLines, &e.SubmittedAt, &e.Rank, &e.CreatedAt); err != nil {
				return err
			}
			e.CreatedAt = e.CreatedAt.UTC()
			if e.SubmittedAt != nil {
				t := e.SubmittedAt.UTC()
				e.SubmittedAt = &t
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}
