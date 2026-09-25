package proofs_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/proofs"
)

type fixture struct {
	d      *dbtest.DB
	users  *identity.Service
	agents *agents.Service
	proofs *proofs.Service
	userID string
	agent  string
}

func setup(t *testing.T) fixture {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	tasks, err := proofs.LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := proofs.SyncCatalog(ctx, d.AdminPool, tasks); err != nil {
		t.Fatal(err)
	}
	ps := proofs.NewService(d.AppPool)
	as := agents.NewService(d.AppPool, ps)
	us := identity.NewService(d.AppPool, nil)
	u, _, err := us.Signup(ctx, "o@example.com", "longenough1")
	if err != nil {
		t.Fatal(err)
	}
	a, err := as.Create(ctx, u.ID, agents.CreateInput{Name: "fixer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := as.CreateKey(ctx, u.ID, "k"); err != nil {
		t.Fatal(err)
	}
	return fixture{d: d, users: us, agents: as, proofs: ps, userID: u.ID, agent: a.ID}
}

func problem(t *testing.T, err error, status int, code string) {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Status != status || p.Code != code {
		t.Fatalf("expected %d %s, got %v", status, code, err)
	}
}

func TestCreateProof_RequiresOnlineAgentAndOneAtATime(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	_, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	problem(t, err, 409, "agent_offline")

	if err := f.agents.Heartbeat(ctx, f.agent, "0.1", "h"); err != nil {
		t.Fatal(err)
	}
	p, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	if err != nil || p.Status != proofs.StatusQueued {
		t.Fatalf("create: %v %+v", err, p)
	}
	_, err = f.proofs.Create(ctx, f.userID, "go-fix-retry")
	problem(t, err, 409, "proof_in_progress")

	_, err = f.proofs.Create(ctx, f.userID, "nope")
	problem(t, err, 404, "not_found")

	facts, err := f.proofs.ProofFacts(ctx, f.agent)
	if err != nil || !facts.HasOpen || facts.HasPassed {
		t.Fatalf("facts: %v %+v", err, facts)
	}
	o, _ := f.agents.Overview(ctx, f.userID)
	if o.Stage != agents.StageChecking {
		t.Fatalf("stage: %s", o.Stage)
	}

	got, err := f.proofs.Get(ctx, f.userID, p.ID)
	if err != nil || got.ID != p.ID {
		t.Fatalf("get: %v", err)
	}
	list, _ := f.proofs.List(ctx, f.userID)
	if len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
}

func TestCreateProof_DailyLimit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	for i := 0; i < 10; i++ {
		p, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		// finish it so the next one can start
		err = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'failed', finished_at = now() WHERE id = $1`, p.ID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	problem(t, err, 429, "daily_limit")
	o, _ := f.agents.Overview(ctx, f.userID)
	if o.Stage != agents.StageCheckFailed {
		t.Fatalf("stage after failures: %s", o.Stage)
	}
}

func TestRetry_OnlyForInfraErrorOrExpired(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	p, _ := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	_, err := f.proofs.Retry(ctx, f.userID, p.ID)
	problem(t, err, 409, "state_conflict")
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'infra_error', finished_at = now(), failure_reason = 'x' WHERE id = $1`, p.ID)
		return err
	})
	r, err := f.proofs.Retry(ctx, f.userID, p.ID)
	if err != nil || r.Status != proofs.StatusQueued || r.FailureReason != "" || r.FinishedAt != nil {
		t.Fatalf("retry: %v %+v", err, r)
	}
}

func TestTasksHidesGameBotTasks(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO proof_tasks (slug, title, language, kind, image, run_cmd, agent_timeout_s, sandbox_timeout_s,
			visible_tests, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256)
			VALUES ('tanks-bot', 'Tanks', 'python', 'game_bot', 'x', 'true', 60, 60, 0, 0, 'md', ''::bytea, ''::bytea, 'sha')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	tasks, err := f.proofs.Tasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.Slug == "tanks-bot" {
			t.Fatalf("Tasks must not list game_bot tasks: %+v", tasks)
		}
	}

	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	_, err = f.proofs.Create(ctx, f.userID, "tanks-bot")
	problem(t, err, 404, "not_found")
}

func TestCreateWithRepoOverridesRepo(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO proof_tasks (slug, title, language, kind, image, run_cmd, agent_timeout_s, sandbox_timeout_s,
			visible_tests, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256)
			VALUES ('tanks-bot', 'Tanks', 'python', 'game_bot', 'x', 'true', 60, 60, 0, 0, 'md', 'task-repo'::bytea, ''::bytea, 'task-sha')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")

	// CreateWithRepo against a kind = 'proof' task is not found.
	_, err = f.proofs.CreateWithRepo(ctx, f.userID, "go-fix-retry", []byte("own-repo"))
	problem(t, err, 404, "not_found")

	p, err := f.proofs.CreateWithRepo(ctx, f.userID, "tanks-bot", []byte("own-repo"))
	if err != nil {
		t.Fatalf("create with repo: %v", err)
	}
	if p.Kind != proofs.KindGameBot {
		t.Fatalf("expected kind game_bot, got %q", p.Kind)
	}

	claimed, task, err := f.proofs.Claim(ctx, f.agent)
	if err != nil || claimed == nil || task == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if string(task.RepoTar) != "own-repo" {
		t.Fatalf("expected the proof's own repo, got %q", task.RepoTar)
	}
	sum := sha256.Sum256([]byte("own-repo"))
	wantSHA := hex.EncodeToString(sum[:])
	if task.RepoSHA256 != wantSHA {
		t.Fatalf("expected the proof's own repo sha256, got %q want %q", task.RepoSHA256, wantSHA)
	}

	tar, err := f.proofs.RepoTar(ctx, f.agent, claimed.ID)
	if err != nil || string(tar) != "own-repo" {
		t.Fatalf("RepoTar: %v %q", err, tar)
	}
}

func TestProofFactsIgnoreGameBot(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO proof_tasks (slug, title, language, kind, image, run_cmd, agent_timeout_s, sandbox_timeout_s,
			visible_tests, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256)
			VALUES ('tanks-bot', 'Tanks', 'python', 'game_bot', 'x', 'true', 60, 60, 0, 0, 'md', 'r'::bytea, ''::bytea, 'sha')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	p, err := f.proofs.CreateWithRepo(ctx, f.userID, "tanks-bot", []byte("r"))
	if err != nil {
		t.Fatal(err)
	}
	err = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'passed', finished_at = now() WHERE id = $1`, p.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	facts, err := f.proofs.ProofFacts(ctx, f.agent)
	if err != nil {
		t.Fatal(err)
	}
	if facts.HasPassed || facts.HasOpen || facts.LastFinishedStatus != "" {
		t.Fatalf("a passed game_bot proof must not affect agent stage facts: %+v", facts)
	}
	o, _ := f.agents.Overview(ctx, f.userID)
	if o.Stage != agents.StageConnected {
		t.Fatalf("stage must stay connected, not move on a game_bot proof: %s", o.Stage)
	}
}

func TestListAndLatestHideGameBotProofs(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO proof_tasks (slug, title, language, kind, image, run_cmd, agent_timeout_s, sandbox_timeout_s,
			visible_tests, hidden_tests, task_md, repo_tar, hidden_tar, repo_sha256)
			VALUES ('tanks-bot', 'Tanks', 'python', 'game_bot', 'x', 'true', 60, 60, 0, 0, 'md', 'r'::bytea, ''::bytea, 'sha')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")

	gameProof, err := f.proofs.CreateWithRepo(ctx, f.userID, "tanks-bot", []byte("r"))
	if err != nil {
		t.Fatal(err)
	}
	// Finish it so a plain proof can be created afterwards without hitting "one open proof".
	err = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE proofs SET status = 'passed', finished_at = now() WHERE id = $1`, gameProof.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := f.proofs.List(ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.ID == gameProof.ID {
			t.Fatalf("List must not return game_bot proofs: %+v", list)
		}
	}

	latest, err := f.proofs.Latest(ctx, f.agent)
	if err != nil {
		t.Fatal(err)
	}
	if latest != nil {
		t.Fatalf("Latest must not return a game_bot proof when there is no kind=proof one: %+v", latest)
	}

	plainProof, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	if err != nil {
		t.Fatal(err)
	}
	latest, err = f.proofs.Latest(ctx, f.agent)
	if err != nil || latest == nil || latest.ID != plainProof.ID {
		t.Fatalf("Latest must return the newest kind=proof proof: %v %+v", err, latest)
	}
}

func TestLatest_NilThenNewestWithoutDiff(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if p, err := f.proofs.Latest(ctx, f.agent); err != nil || p != nil {
		t.Fatalf("an agent without proofs has no latest one: %v %+v", err, p)
	}
	_ = f.agents.Heartbeat(ctx, f.agent, "0.1", "h")
	created, err := f.proofs.Create(ctx, f.userID, "go-fix-retry")
	if err != nil {
		t.Fatal(err)
	}
	claimed, _, err := f.proofs.Claim(ctx, f.agent)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if err := f.proofs.SubmitResult(ctx, f.agent, claimed.ID, proofs.ResultInput{Diff: "--- a/x\n+++ b/x\n", LogTail: "log"}); err != nil {
		t.Fatal(err)
	}
	p, err := f.proofs.Latest(ctx, f.agent)
	if err != nil || p == nil || p.ID != created.ID || p.Status != proofs.StatusDiffSubmitted || p.Diff != "" || p.AgentLogTail != "" {
		t.Fatalf("latest must be the newest proof without diff and log: %v %+v", err, p)
	}
}
