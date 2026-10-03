package challenges_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/challenges"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/proofs"
	"tolerance/internal/skillrating"
	"tolerance/internal/skills"
)

type fx struct {
	d       *dbtest.DB
	svc     *challenges.Service
	proofs  *proofs.Service
	hidden  map[string][]string // skill task slug -> hidden test names
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

// reserve marks a catalog task as challenge-only, which is how a challenge's task
// is kept out of the qualification pool.
func (f *fx) reserve(t *testing.T, slug string) string {
	t.Helper()
	f.exec(t, `UPDATE skill_tasks SET challenge_only = true WHERE slug = $1`, slug)
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
	f.operational(t, agentID)
	return userID, agentID
}

// operational gives the agent the passed basic proof that entering a challenge
// requires. basicTask is seeded once per fixture.
func (f *fx) operational(t *testing.T, agentID string) {
	t.Helper()
	f.exec(t, `INSERT INTO proofs (id, agent_id, kind, task_slug, status, finished_at)
		VALUES ($1, $2, 'proof', $3, 'passed', now())`, idgen.New("proof"), agentID, basicTask)
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

// Slugs from this repository's practice catalog (backend/fixtures/skills).
const (
	goTask     = "go-lru-cache-eviction"
	goTask2    = "go-cursor-pagination"
	pythonTask = "py-interval-merge"
	basicTask  = "basic-proof"
)

func setup(t *testing.T, minTier, status string) *fx {
	t.Helper()
	d := dbtest.New(t)
	ps := proofs.NewService(d.AppPool)
	f := &fx{d: d, svc: challenges.NewService(d.AppPool, ps), proofs: ps}
	// The real catalog, so hidden test names are real: a challenge is scored by
	// counting hidden tests by name, exactly as a qualification run is.
	sk, tasks, err := skills.LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if err := skills.SyncCatalog(context.Background(), d.AdminPool, sk, tasks); err != nil {
		t.Fatal(err)
	}
	if f.hidden, err = skills.HiddenNamesByTask(sk, tasks); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO proof_tasks (slug, title, language, image, run_cmd, agent_timeout_s, sandbox_timeout_s,
		visible_tests, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256)
		VALUES ($1, $1, 'go', 'arena-proof-go:1', 'go test -json ./...', 600, 120, 1, 1, '# basic', '\x00', '\x00', 'sha')`, basicTask)
	f.adminID = idgen.New("user")
	f.exec(t, `INSERT INTO users (id, email, role) VALUES ($1, 'admin@example.com', 'admin')`, f.adminID)
	f.userID, f.agentID = f.addAgent(t, "entrant")
	f.task = f.reserve(t, goTask)
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
	other := f.reserve(t, goTask2)
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
	py := f.reserve(t, pythonTask)
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

// enterAgent creates a fresh owner with an online agent and enters it.
func (f *fx) enterAgent(t *testing.T, name string) string {
	t.Helper()
	return f.enterAgentWithConsent(t, name, true)
}

func (f *fx) enterAgentWithConsent(t *testing.T, name string, consent bool) string {
	t.Helper()
	userID, agentID := f.addAgent(t, name)
	if _, err := f.svc.Enter(context.Background(), userID, f.slug, consent); err != nil {
		t.Fatalf("enter as %s: %v", name, err)
	}
	return agentID
}

func (f *fx) openProofID(t *testing.T, agentID string) string {
	t.Helper()
	var id string
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM proofs WHERE agent_id = $1 AND kind = 'challenge' ORDER BY created_at DESC LIMIT 1`, agentID).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fx) proofStatus(t *testing.T, agentID string) string {
	t.Helper()
	var st string
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status FROM proofs WHERE id = $1`, f.openProofID(t, agentID)).Scan(&st)
	})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// score plays the sandbox for this agent's entry: frac of the task's hidden tests
// pass, the diff has diffLines changed lines, and the finish hook runs.
func (f *fx) score(t *testing.T, agentID string, frac float64, diffLines int) {
	t.Helper()
	proofID := f.writeVerdict(t, agentID, frac, diffLines)
	if err := f.svc.OnProofFinished(context.Background(), proofID); err != nil {
		t.Fatalf("finish hook: %v", err)
	}
}

// scoreWithoutHook writes the proof's verdict but deliberately does not run
// OnProofFinished, standing in for a finish hook that was dropped.
func (f *fx) scoreWithoutHook(t *testing.T, agentID string, frac float64, diffLines int) {
	t.Helper()
	f.writeVerdict(t, agentID, frac, diffLines)
}

// writeVerdict puts a sandbox result and a diff on the entry's proof and returns
// its id, without notifying anyone.
func (f *fx) writeVerdict(t *testing.T, agentID string, frac float64, diffLines int) string {
	t.Helper()
	proofID := f.openProofID(t, agentID)
	names := f.hidden[f.task]
	if len(names) == 0 {
		t.Fatalf("no hidden test names for %s", f.task)
	}
	n := int(float64(len(names))*frac + 0.5)
	tests := make([]proofs.TestResult, 0, len(names)+1)
	// A passing test outside the hidden set must never add to the score.
	tests = append(tests, proofs.TestResult{Name: "visible_passes", Passed: true})
	for i, name := range names {
		tests = append(tests, proofs.TestResult{Name: name, Passed: i < n})
	}
	sr, err := json.Marshal(proofs.SandboxResult{Tests: tests})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n")
	for i := 0; i < diffLines; i++ {
		fmt.Fprintf(&b, "+line %d\n", i)
	}
	status := "passed"
	if n < len(names) {
		status = "failed"
	}
	f.exec(t, `UPDATE proofs SET status = $2, finished_at = now(), diff = $3, sandbox_result = $4 WHERE id = $1`,
		proofID, status, b.String(), sr)
	return proofID
}

func (f *fx) status(t *testing.T) string {
	t.Helper()
	var st string
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status FROM challenges WHERE slug = $1`, f.slug).Scan(&st)
	})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func (f *fx) setWindow(t *testing.T, opensIn, closesIn time.Duration) {
	t.Helper()
	f.exec(t, `UPDATE challenges SET opens_at = now() + make_interval(secs => $2), closes_at = now() + make_interval(secs => $3) WHERE slug = $1`,
		f.slug, opensIn.Seconds(), closesIn.Seconds())
}

func TestCloseRanksEntriesAndKeepsTheirNames(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	a := f.enterAgent(t, "winner")
	b := f.enterAgent(t, "runnerup")
	f.score(t, a, 1, 12)
	f.score(t, b, 1, 80)

	// The winner goes private after submitting.
	f.exec(t, `UPDATE agents SET public = false WHERE id = $1`, a)

	if err := f.svc.Close(ctx, f.adminID, f.slug); err != nil {
		t.Fatalf("close: %v", err)
	}
	view, err := f.svc.Public(ctx, f.slug)
	if err != nil {
		t.Fatalf("public: %v", err)
	}
	if len(view.Standings) != 2 {
		t.Fatalf("standings = %d entries, want 2: opting out of the arena tables does not erase a place in a finished competition", len(view.Standings))
	}
	if view.Standings[0].AgentName != "winner" || view.Standings[0].Rank != 1 {
		t.Fatalf("first place = %s/%d, want winner/1", view.Standings[0].AgentName, view.Standings[0].Rank)
	}
	if view.Standings[1].Rank != 2 {
		t.Errorf("second place rank = %d, want 2", view.Standings[1].Rank)
	}
}

func TestCloseScoresAnUnfinishedEntryAsZero(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	done := f.enterAgent(t, "done")
	stuck := f.enterAgent(t, "stillrunning")
	f.exec(t, `UPDATE proofs SET status = 'running_agent', claimed_at = now()
		WHERE agent_id = $1 AND kind = 'challenge'`, stuck)
	f.score(t, done, 0.25, 5)

	if err := f.svc.Close(ctx, f.adminID, f.slug); err != nil {
		t.Fatalf("close: %v", err)
	}
	view, err := f.svc.Public(ctx, f.slug)
	if err != nil {
		t.Fatal(err)
	}
	last := view.Standings[len(view.Standings)-1]
	if last.AgentName != "stillrunning" || last.Rank != 2 || last.Score != 0 {
		t.Fatalf("unfinished entry = %+v, want stillrunning ranked last with score 0", last)
	}
	// Its proof must not be left open forever either: the deadline passed, so the
	// attempt did not happen and the agent's slot is free again.
	if st := f.proofStatus(t, stuck); st != "expired" {
		t.Fatalf("proof of an unfinished entry = %q, want expired", st)
	}
}

func TestTickOpensAndClosesOnTime(t *testing.T) {
	f := setup(t, "none", "draft")
	ctx := context.Background()
	f.setWindow(t, -time.Hour, time.Hour)
	if err := f.svc.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if st := f.status(t); st != "open" {
		t.Fatalf("status = %q, want open", st)
	}
	f.setWindow(t, -2*time.Hour, -time.Hour)
	if err := f.svc.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if st := f.status(t); st != "closed" {
		t.Fatalf("status = %q, want closed", st)
	}
	// A tick with nothing to do is not an error.
	if err := f.svc.Tick(ctx); err != nil {
		t.Fatalf("idle tick: %v", err)
	}
}

func TestPublicHidesTheTaskUntilPublished(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	a := f.enterAgent(t, "solo")
	f.score(t, a, 1, 9)

	open, err := f.svc.Public(ctx, f.slug)
	if err != nil {
		t.Fatal(err)
	}
	if open.TaskMD != "" || len(open.HiddenTests) != 0 || len(open.Standings) != 0 {
		t.Fatal("an open challenge must not leak the task, the test names or the standings")
	}
	if open.Entrants != 1 {
		t.Errorf("entrants = %d, want 1", open.Entrants)
	}

	if err := f.svc.Close(ctx, f.adminID, f.slug); err != nil {
		t.Fatal(err)
	}
	closed, err := f.svc.Public(ctx, f.slug)
	if err != nil {
		t.Fatal(err)
	}
	if closed.TaskMD != "" || len(closed.HiddenTests) != 0 {
		t.Error("a closed but unpublished challenge still hides the task and the test names")
	}
	if len(closed.Standings) != 1 {
		t.Fatal("a closed challenge shows its standings")
	}
	if closed.Standings[0].Diff != "" {
		t.Error("a closed challenge does not publish diffs yet")
	}

	if err := f.svc.Publish(ctx, f.adminID, f.slug); err != nil {
		t.Fatal(err)
	}
	pub, err := f.svc.Public(ctx, f.slug)
	if err != nil {
		t.Fatal(err)
	}
	if pub.TaskMD == "" || len(pub.HiddenTests) == 0 {
		t.Errorf("a published challenge shows the task and the hidden test names: %+v", pub)
	}
	if pub.Standings[0].Diff == "" {
		t.Error("a consenting entrant's diff is published")
	}
}

func TestPublishWithholdsANonConsentingDiff(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	a := f.enterAgentWithConsent(t, "shy", false)
	f.score(t, a, 1, 9)
	if err := f.svc.Close(ctx, f.adminID, f.slug); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Publish(ctx, f.adminID, f.slug); err != nil {
		t.Fatal(err)
	}
	view, err := f.svc.Public(ctx, f.slug)
	if err != nil {
		t.Fatal(err)
	}
	if view.Standings[0].Diff != "" {
		t.Fatal("a diff stays private without the entrant's consent")
	}
	if view.TaskMD == "" {
		t.Error("the task itself is published regardless of any entrant's consent")
	}
}

func TestCloseAndPublishGuardTheirOrder(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	// Publishing before closing is refused; closing twice is refused.
	problem(t, f.svc.Publish(ctx, f.adminID, f.slug), 409, "challenge_not_closed")
	if err := f.svc.Close(ctx, f.adminID, f.slug); err != nil {
		t.Fatal(err)
	}
	problem(t, f.svc.Close(ctx, f.adminID, f.slug), 409, "challenge_not_open")
	if err := f.svc.Publish(ctx, f.adminID, f.slug); err != nil {
		t.Fatal(err)
	}
	problem(t, f.svc.Publish(ctx, f.adminID, f.slug), 409, "challenge_not_closed")
}

func TestPublicRefusesADraft(t *testing.T) {
	f := setup(t, "none", "draft")
	_, err := f.svc.Public(context.Background(), f.slug)
	problem(t, err, 404, "not_found")
}

func TestListGroupsOpenUpcomingAndPast(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	upcoming := challenges.NewInput{Slug: "spring-cup", Title: "Spring cup", SkillTaskSlug: f.reserve(t, goTask2),
		OpensAt: time.Now().Add(24 * time.Hour), ClosesAt: time.Now().Add(48 * time.Hour)}
	if _, err := f.svc.Create(ctx, f.adminID, upcoming); err != nil {
		t.Fatalf("create: %v", err)
	}
	// A draft is nobody's business until it opens.
	got, err := f.svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Open) != 1 || got.Open[0].Slug != f.slug {
		t.Fatalf("open = %+v, want just %s", got.Open, f.slug)
	}
	if len(got.Upcoming) != 0 || len(got.Past) != 0 {
		t.Fatalf("a draft must not be listed: upcoming = %+v past = %+v", got.Upcoming, got.Past)
	}
	if err := f.svc.Open(ctx, f.adminID, "spring-cup"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if got, err = f.svc.List(ctx); err != nil {
		t.Fatal(err)
	}
	if len(got.Upcoming) != 1 || got.Upcoming[0].Slug != "spring-cup" {
		t.Fatalf("upcoming = %+v, want spring-cup (open but not yet started)", got.Upcoming)
	}
	if err := f.svc.Close(ctx, f.adminID, f.slug); err != nil {
		t.Fatal(err)
	}
	if got, err = f.svc.List(ctx); err != nil {
		t.Fatal(err)
	}
	if len(got.Past) != 1 || got.Past[0].Slug != f.slug {
		t.Fatalf("past = %+v, want %s", got.Past, f.slug)
	}
}

func TestCreateTakesTheTaskOutOfTheQualificationPool(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	// A task straight from the catalog, still in the pool.
	var reserved bool
	read := func() bool {
		t.Helper()
		if err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT challenge_only FROM skill_tasks WHERE slug = $1`, goTask2).Scan(&reserved)
		}); err != nil {
			t.Fatal(err)
		}
		return reserved
	}
	if read() {
		t.Fatal("the catalog task must start in the qualification pool")
	}
	if _, err := f.svc.Create(ctx, f.adminID, challenges.NewInput{Slug: "spring-cup", Title: "Spring cup",
		SkillTaskSlug: goTask2, OpensAt: time.Now(), ClosesAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !read() {
		t.Fatal("creating a challenge must claim its task: agents meeting on it in a competition must not also be qualified on it")
	}
}

func TestEnterBeforeTheWindowOpens(t *testing.T) {
	f := setup(t, "none", "open")
	// An admin may open a challenge early; until opens_at the public pages label
	// it "Opens <date>", so accepting an entry here hands one agent extra hours
	// against a fixed deadline on a one-attempt competition.
	f.setWindow(t, time.Hour, 2*time.Hour)
	problem(t, mustErr(f.svc.Enter(context.Background(), f.userID, f.slug, true)), 409, "challenge_not_open")
}

func TestEnterRequiresABasicProof(t *testing.T) {
	f := setup(t, "none", "open")
	// This agent has never proved it can work at all. An open challenge must not
	// take its entry: it burns a slot and pads the entrant count.
	f.exec(t, `DELETE FROM proofs WHERE agent_id = $1 AND kind = 'proof'`, f.agentID)
	problem(t, mustErr(f.svc.Enter(context.Background(), f.userID, f.slug, true)), 409, "agent_not_operational")
}

func TestPatchCannotSmuggleInPrizes(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	py := f.reserve(t, pythonTask)
	c, err := f.svc.Create(ctx, f.adminID, challenges.NewInput{Slug: "python-open", Title: "Python open",
		SkillTaskSlug: py, OpensAt: time.Now(), ClosesAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	prizes := "$500 for first place"
	// Create refuses prizes on a language whose verdict runs in the agent's own
	// process. Adding them afterwards must be refused for the same reason.
	problem(t, mustErr2(f.svc.Patch(ctx, f.adminID, c.Slug, &prizes, nil)), 422, "prizes_not_allowed_for_language")

	note := "paid by bank transfer"
	if _, err := f.svc.Patch(ctx, f.adminID, c.Slug, nil, &note); err != nil {
		t.Fatalf("a payout note is always allowed: %v", err)
	}
}

func TestCloseRecoversAScoreFromAFinishedProof(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	a := f.enterAgent(t, "unhooked")
	// The proof reached a verdict but the finish hook never ran — a transient
	// failure, which proofs.Worker.notify logs and swallows. Closing must read the
	// verdict that is sitting in the row, not publish this agent as a zero.
	f.scoreWithoutHook(t, a, 1, 11)

	if err := f.svc.Close(ctx, f.adminID, f.slug); err != nil {
		t.Fatalf("close: %v", err)
	}
	view, err := f.svc.Public(ctx, f.slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Standings) != 1 || view.Standings[0].Score != 1 {
		t.Fatalf("standings = %+v, want the entry scored 1 from its own sandbox result", view.Standings)
	}
}

func TestAHookArrivingAfterCloseDoesNotDesyncScoreFromRank(t *testing.T) {
	f := setup(t, "none", "open")
	ctx := context.Background()
	winner := f.enterAgent(t, "won")
	loser := f.enterAgent(t, "lost")
	f.scoreWithoutHook(t, winner, 1, 10)
	f.score(t, loser, 0.5, 10)
	if err := f.svc.Close(ctx, f.adminID, f.slug); err != nil {
		t.Fatalf("close: %v", err)
	}
	// The hook finally fires, after the places are out. It must not rewrite a
	// score the ranking was computed from.
	if err := f.svc.OnProofFinished(ctx, f.openProofID(t, winner)); err != nil {
		t.Fatalf("late hook: %v", err)
	}
	view, err := f.svc.Public(ctx, f.slug)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range view.Standings {
		if s.Rank == 1 && s.Score < 0.99 {
			t.Fatalf("first place carries score %v: a late hook desynced score from rank (%+v)", s.Score, view.Standings)
		}
	}
}
