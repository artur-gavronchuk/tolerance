package challenges_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/challenges"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/proofs"
	"tolerance/internal/skillrating"
)

type fx struct {
	d       *dbtest.DB
	svc     *challenges.Service
	adminID string
	userID  string
	agentID string
	slug    string
	task    string
}

func (f *fx) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	err := f.d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
	if err != nil {
		t.Fatalf("seed (%s): %v", sql, err)
	}
}

// addSkillTask seeds a task in the named skill's language. challengeOnly keeps it
// out of the qualification pool.
func (f *fx) addSkillTask(t *testing.T, skill, slug string, challengeOnly bool) string {
	t.Helper()
	lang := skill
	f.exec(t, `INSERT INTO skills (slug, title, language, image, run_cmd)
		VALUES ($1, $1, $2, 'arena-skill-' || $1 || ':1', 'go test -json ./...') ON CONFLICT DO NOTHING`, skill, lang)
	f.exec(t, `INSERT INTO skill_tasks (slug, skill_slug, title, difficulty, agent_timeout_s, sandbox_timeout_s,
		hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256, challenge_only)
		VALUES ($1, $2, $1, 2, 600, 120, 4, '# task', '\x00', '\x00', 'sha', $3)`, slug, skill, challengeOnly)
	return slug
}

// addAgent creates an owner with an online agent that has a current version.
func (f *fx) addAgent(t *testing.T, name string) (userID, agentID string) {
	t.Helper()
	userID, agentID = idgen.New("user"), idgen.New("agent")
	f.exec(t, `INSERT INTO users (id, email) VALUES ($1, $1 || '@example.com')`, userID)
	f.exec(t, `INSERT INTO agents (id, owner_user_id, name) VALUES ($1, $2, $3)`, agentID, userID, name)
	ver := idgen.New("ver")
	f.exec(t, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
		VALUES ($1, $2, 1, 'claude-opus-5', 'claude-code 2.1', $1)`, ver, agentID)
	f.exec(t, `UPDATE agents SET current_version_id = $2 WHERE id = $1`, agentID, ver)
	f.exec(t, `INSERT INTO agent_presence (agent_id, last_seen_at, connector_version, hostname)
		VALUES ($1, now(), '0.2', 'h') ON CONFLICT (agent_id) DO UPDATE SET last_seen_at = now()`, agentID)
	return userID, agentID
}

// rate gives the agent a rating on the skill, earned on its current version.
func (f *fx) rate(t *testing.T, agentID, skill string, rating, uncertainty int) {
	t.Helper()
	f.exec(t, `INSERT INTO skill_ratings (agent_id, skill_slug, version_id, rating, uncertainty, runs, sum_targets)
		SELECT $1, $2, current_version_id, $3, $4, 1, $3 FROM agents WHERE id = $1
		ON CONFLICT (agent_id, skill_slug) DO UPDATE SET rating = $3, uncertainty = $4,
			version_id = (SELECT current_version_id FROM agents WHERE id = $1)`, agentID, skill, rating, uncertainty)
}

// newVersion is the owner switching configuration: the stored rating now names a
// version the agent no longer runs.
func (f *fx) newVersion(t *testing.T, agentID string) {
	t.Helper()
	ver := idgen.New("ver")
	f.exec(t, `INSERT INTO agent_versions (id, agent_id, number, model, harness, config_digest)
		VALUES ($1, $2, 2, 'claude-sonnet-5-5', 'claude-code 2.1', $1)`, ver, agentID)
	f.exec(t, `UPDATE agents SET current_version_id = $2 WHERE id = $1`, agentID, ver)
}

func setup(t *testing.T, minTier, status string) *fx {
	t.Helper()
	d := dbtest.New(t)
	f := &fx{d: d, svc: challenges.NewService(d.AppPool, proofs.NewService(d.AppPool))}
	f.adminID = idgen.New("user")
	f.exec(t, `INSERT INTO users (id, email, role) VALUES ($1, 'admin@example.com', 'admin')`, f.adminID)
	f.userID, f.agentID = f.addAgent(t, "entrant")
	f.task = f.addSkillTask(t, "go", "cup-task", true)
	f.slug = "autumn-cup"
	f.exec(t, `INSERT INTO challenges (id, slug, title, skill_task_slug, min_tier, opens_at, closes_at, status, created_by)
		VALUES ($1, $2, 'Autumn cup', $3, $4, now() - interval '1 hour', now() + interval '1 hour', $5, $6)`,
		idgen.New("chal"), f.slug, f.task, minTier, status, f.adminID)
	return f
}

// freeSlot ends the agent's open proof so it can start another one.
func (f *fx) freeSlot(t *testing.T, agentID string) {
	t.Helper()
	f.exec(t, `UPDATE proofs SET status = 'passed', finished_at = now() WHERE agent_id = $1 AND finished_at IS NULL`, agentID)
}

func (f *fx) rating(t *testing.T, agentID, skill string) (rating, uncertainty, runs int) {
	t.Helper()
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT rating, uncertainty, runs FROM skill_ratings WHERE agent_id = $1 AND skill_slug = $2`,
			agentID, skill).Scan(&rating, &uncertainty, &runs)
	})
	if err != nil {
		t.Fatal(err)
	}
	return
}

func problem(t *testing.T, err error, status int, code string) {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected a problem, got %v", err)
	}
	if p.Status != status || p.Code != code {
		t.Fatalf("got %d %s, want %d %s", p.Status, p.Code, status, code)
	}
}

func TestEnterRequiresTheMinimumTier(t *testing.T) {
	f := setup(t, "verified", "open")
	ctx := context.Background()
	f.rate(t, f.agentID, "go", 1600, 350) // access 1250, below verified
	problem(t, mustErr(f.svc.Enter(ctx, f.userID, f.slug, true)), 403, "tier_too_low")

	f.rate(t, f.agentID, "go", 2000, 100) // access 1900
	if _, err := f.svc.Enter(ctx, f.userID, f.slug, true); err != nil {
		t.Fatalf("enter: %v", err)
	}
}

func TestEnterWithoutAnyRatingIsRefused(t *testing.T) {
	f := setup(t, "verified", "open")
	problem(t, mustErr(f.svc.Enter(context.Background(), f.userID, f.slug, true)), 403, "tier_too_low")
}

func TestEnterUsesTheTierOfTheCurrentVersion(t *testing.T) {
	f := setup(t, "verified", "open")
	f.rate(t, f.agentID, "go", 2000, 100)
	f.newVersion(t, f.agentID)
	// A rating earned by a configuration the owner has since replaced must not
	// open the door for the new one.
	problem(t, mustErr(f.svc.Enter(context.Background(), f.userID, f.slug, true)), 403, "tier_too_low")
}

func TestEnterTwiceIsRefused(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	if _, err := f.svc.Enter(ctx, f.userID, f.slug, true); err != nil {
		t.Fatalf("enter: %v", err)
	}
	f.freeSlot(t, f.agentID)
	problem(t, mustErr(f.svc.Enter(ctx, f.userID, f.slug, true)), 409, "already_entered")
}

func TestEnterWhileAnotherProofIsOpenIsRefused(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	if _, err := f.svc.Enter(ctx, f.userID, f.slug, true); err != nil {
		t.Fatalf("enter: %v", err)
	}
	other := f.addSkillTask(t, "go", "second-cup-task", true)
	f.exec(t, `INSERT INTO challenges (id, slug, title, skill_task_slug, min_tier, opens_at, closes_at, status, created_by)
		VALUES ($1, 'winter-cup', 'Winter cup', $2, 'none', now() - interval '1 hour', now() + interval '1 hour', 'open', $3)`,
		idgen.New("chal"), other, f.adminID)
	// The agent still owes a diff for the first challenge; one open proof at a time.
	problem(t, mustErr(f.svc.Enter(ctx, f.userID, "winter-cup", true)), 409, "proof_in_progress")
}

func TestBannedAgentCannotEnter(t *testing.T) {
	f := setup(t, "none", "open")
	f.exec(t, `UPDATE agents SET banned_at = now(), banned_reason = 'test' WHERE id = $1`, f.agentID)
	problem(t, mustErr(f.svc.Enter(context.Background(), f.userID, f.slug, true)), 403, "agent_banned")
}

func TestOfflineAgentCannotEnter(t *testing.T) {
	f := setup(t, "none", "open")
	f.exec(t, `UPDATE agent_presence SET last_seen_at = now() - interval '10 minutes' WHERE agent_id = $1`, f.agentID)
	problem(t, mustErr(f.svc.Enter(context.Background(), f.userID, f.slug, true)), 409, "agent_offline")
}

func TestTwoOwnersMayBothEnter(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	// The entry key is (challenge, agent), not (challenge, user): two agents can
	// both be in one challenge. One agent per owner is a slice 4 limit enforced by
	// a unique index on agents, so the two agents here belong to two owners.
	second, _ := f.addAgent(t, "rival")
	if _, err := f.svc.Enter(ctx, f.userID, f.slug, true); err != nil {
		t.Fatalf("first entrant: %v", err)
	}
	if _, err := f.svc.Enter(ctx, second, f.slug, true); err != nil {
		t.Fatalf("second entrant: %v", err)
	}
}

func TestEnterOutsideTheOpenWindow(t *testing.T) {
	for _, status := range []string{"draft", "closed", "published"} {
		f := setup(t, "none", status)
		problem(t, mustErr(f.svc.Enter(context.Background(), f.userID, f.slug, true)), 409, "challenge_not_open")
	}
}

func TestChallengeDoesNotChangeTheSkillRating(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	f.rate(t, f.agentID, "go", 2000, 100)
	beforeR, beforeU, beforeRuns := f.rating(t, f.agentID, "go")
	if _, err := f.svc.Enter(ctx, f.userID, f.slug, true); err != nil {
		t.Fatalf("enter: %v", err)
	}
	f.freeSlot(t, f.agentID)
	// One task that gets published afterwards must not weigh as much as a run over
	// a rotated pool; otherwise a well-studied task pumps the rating. Places and
	// the public page are the whole reward.
	r, u, runs := f.rating(t, f.agentID, "go")
	if r != beforeR || u != beforeU || runs != beforeRuns {
		t.Fatalf("rating moved on a challenge: %d/%d/%d -> %d/%d/%d", beforeR, beforeU, beforeRuns, r, u, runs)
	}
}

func TestEnterQueuesAChallengeProofOverTheHiddenTask(t *testing.T) {
	f := setup(t, "none", "open")
	entry, err := f.svc.Enter(context.Background(), f.userID, f.slug, true)
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	var kind, taskSlug, status string
	err = f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT kind, skill_task_slug, status FROM proofs WHERE id = $1`, entry.ProofID).
			Scan(&kind, &taskSlug, &status)
	})
	if err != nil {
		t.Fatal(err)
	}
	if kind != proofs.KindChallenge || taskSlug != f.task || status != proofs.StatusQueued {
		t.Fatalf("proof = %s/%s/%s, want challenge/%s/queued", kind, taskSlug, status, f.task)
	}
}

func TestCreateRefusesPrizesWhereTheVerdictIsInProcess(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	py := f.addSkillTask(t, "python", "python-cup-task", true)
	base := challenges.NewInput{Title: "Python cup", SkillTaskSlug: py,
		OpensAt: time.Now().Add(time.Hour), ClosesAt: time.Now().Add(48 * time.Hour)}

	withPrizes := base
	withPrizes.Slug, withPrizes.Prizes = "python-cup", "$500 for first place"
	problem(t, mustErr2(f.svc.Create(ctx, f.adminID, withPrizes)), 422, "prizes_not_allowed_for_language")

	// The same challenge without prizes is fine, and Go takes prizes.
	free := base
	free.Slug = "python-open"
	if _, err := f.svc.Create(ctx, f.adminID, free); err != nil {
		t.Fatalf("prize-free python challenge: %v", err)
	}
	goCup := base
	goCup.Slug, goCup.SkillTaskSlug, goCup.Prizes = "go-cup", f.task, "$500"
	if _, err := f.svc.Create(ctx, f.adminID, goCup); err != nil {
		t.Fatalf("go challenge with prizes: %v", err)
	}
}

func TestCreateValidatesTheWindowAndTheTask(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	bad := challenges.NewInput{Slug: "bad", Title: "Bad", SkillTaskSlug: f.task,
		OpensAt: time.Now().Add(2 * time.Hour), ClosesAt: time.Now().Add(time.Hour)}
	problem(t, mustErr2(f.svc.Create(ctx, f.adminID, bad)), 422, "validation_failed")

	missing := challenges.NewInput{Slug: "missing", Title: "Missing", SkillTaskSlug: "no-such-task",
		OpensAt: time.Now(), ClosesAt: time.Now().Add(time.Hour)}
	problem(t, mustErr2(f.svc.Create(ctx, f.adminID, missing)), 404, "not_found")

	dup := challenges.NewInput{Slug: f.slug, Title: "Duplicate", SkillTaskSlug: f.task,
		OpensAt: time.Now(), ClosesAt: time.Now().Add(time.Hour)}
	problem(t, mustErr2(f.svc.Create(ctx, f.adminID, dup)), 409, "slug_taken")
}

// mustErr and mustErr2 discard the value of a (T, error) call so a guard can be
// asserted in one line.
func mustErr(_ challenges.Entry, err error) error      { return err }
func mustErr2(_ challenges.Challenge, err error) error { return err }

// skillrating is imported for the tier thresholds the tests reason about.
var _ = skillrating.TierVerified
