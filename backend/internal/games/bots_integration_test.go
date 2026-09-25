package games_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/games"
	"tolerance/internal/games/tanks"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/proofs"
)

type fixture struct {
	d      *dbtest.DB
	svc    *games.Service
	users  *identity.Service
	agents *agents.Service
	proofs *proofs.Service
	userID string
}

func setup(t *testing.T) fixture {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	if err := games.Sync(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	ps := proofs.NewService(d.AppPool)
	as := agents.NewService(d.AppPool, ps)
	us := identity.NewService(d.AppPool, nil)
	svc := games.NewService(d.AppPool, ps, nil, slog.Default(), games.Config{})
	u, _, err := us.Signup(ctx, "o@example.com", "longenough1")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{d: d, svc: svc, users: us, agents: as, proofs: ps, userID: u.ID}
}

// archiveWithManifestName is the python starter kit with bot.json's "name" replaced.
func archiveWithManifestName(t *testing.T, name string) []byte {
	t.Helper()
	files, err := tanks.Starter("python")
	if err != nil {
		t.Fatal(err)
	}
	files["bot.json"] = []byte(fmt.Sprintf(`{"name":%q,"language":"python","entry":"bot.py"}`, name))
	archive, err := proofs.TarFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

func problem(t *testing.T, err error, status int, code string) {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Status != status || p.Code != code {
		t.Fatalf("expected %d %s, got %v", status, code, err)
	}
}

func TestSyncCreatesHouseBotsIdempotent(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := games.Sync(ctx, d.AdminPool); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if err := games.Sync(ctx, d.AdminPool); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	var houseCount, activeCount int
	err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM game_bots WHERE house`).Scan(&houseCount); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM game_bots WHERE house AND active_version_id IS NOT NULL`).Scan(&activeCount)
	})
	if err != nil {
		t.Fatal(err)
	}
	if houseCount != 3 {
		t.Fatalf("expected 3 house bots, got %d", houseCount)
	}
	if activeCount != 3 {
		t.Fatalf("expected all house bots to have an active version, got %d", activeCount)
	}

	var kind string
	err = d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT kind FROM proof_tasks WHERE slug = 'tanks-bot'`).Scan(&kind)
	})
	if err != nil {
		t.Fatal(err)
	}
	if kind != "game_bot" {
		t.Fatalf("expected tanks-bot task kind game_bot, got %q", kind)
	}
}

func TestSaveBotCreatesAndRenames(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	m, err := f.svc.SaveBot(ctx, f.userID, "Ace")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.Name != "Ace" || m.Rating == 0 {
		t.Fatalf("unexpected bot: %+v", m)
	}

	renamed, err := f.svc.SaveBot(ctx, f.userID, "Ace2")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.ID != m.ID || renamed.Name != "Ace2" {
		t.Fatalf("expected the same bot renamed, got %+v", renamed)
	}

	var count int
	err = f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM game_bots WHERE owner_user_id = $1`, f.userID).Scan(&count)
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rename must not create a second bot, found %d", count)
	}
}

func TestSaveBotNameTaken(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// Collides with the house bot "hunter" in a different case.
	_, err := f.svc.SaveBot(ctx, f.userID, "Hunter")
	problem(t, err, 409, "name_taken")

	// Collides with another owner's bot in a different case.
	other, _, err := f.users.Signup(ctx, "other@example.com", "longenough1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SaveBot(ctx, other.ID, "Nightfall"); err != nil {
		t.Fatalf("create other bot: %v", err)
	}
	_, err = f.svc.SaveBot(ctx, f.userID, "NIGHTFALL")
	problem(t, err, 409, "name_taken")
}

func TestSaveBotBadName(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.SaveBot(ctx, f.userID, "no spaces allowed")
	problem(t, err, 422, "validation_failed")
}

func starterArchive(t *testing.T) []byte {
	t.Helper()
	files, err := tanks.Starter("python")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := proofs.TarFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestUploadVersionCreatesPendingAndJob(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.svc.SaveBot(ctx, f.userID, "Uploader"); err != nil {
		t.Fatal(err)
	}

	v, err := f.svc.UploadVersion(ctx, f.userID, starterArchive(t))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if v.Number != 1 || v.Status != "pending" || v.Source != "upload" {
		t.Fatalf("unexpected version: %+v", v)
	}

	var jobCount int
	err = f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE kind = 'check_bot' AND payload = $1::jsonb`,
			`{"version_id":"`+v.ID+`"}`).Scan(&jobCount)
	})
	if err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 {
		t.Fatalf("expected one check_bot job for version %s, got %d", v.ID, jobCount)
	}

	v2, err := f.svc.UploadVersion(ctx, f.userID, starterArchive(t))
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	if v2.Number != 2 {
		t.Fatalf("expected version number 2, got %d", v2.Number)
	}
}

func TestUploadWithoutBotUsesManifestName(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	v, err := f.svc.UploadVersion(ctx, f.userID, starterArchive(t))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if v.Number != 1 {
		t.Fatalf("unexpected version: %+v", v)
	}

	var name string
	err = f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT name FROM game_bots WHERE owner_user_id = $1`, f.userID).Scan(&name)
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "my-tank" {
		t.Fatalf("expected the bot's name to come from the starter's bot.json, got %q", name)
	}
}

func TestSecondUploaderOfUnmodifiedStarterGetsSuffixedName(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	v1, err := f.svc.UploadVersion(ctx, f.userID, starterArchive(t))
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	if v1.Number != 1 {
		t.Fatalf("unexpected first version: %+v", v1)
	}

	other, _, err := f.users.Signup(ctx, "other@example.com", "longenough1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UploadVersion(ctx, other.ID, starterArchive(t)); err != nil {
		t.Fatalf("second uploader: %v", err)
	}

	names := map[string]string{}
	err = f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT owner_user_id, name FROM game_bots WHERE owner_user_id = ANY($1)`, []string{f.userID, other.ID})
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var owner, name string
			if err := rows.Scan(&owner, &name); err != nil {
				return err
			}
			names[owner] = name
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if names[f.userID] != "my-tank" {
		t.Fatalf("expected first uploader's bot named my-tank, got %q", names[f.userID])
	}
	if names[other.ID] != "my-tank-2" {
		t.Fatalf("expected second uploader's bot named my-tank-2, got %q", names[other.ID])
	}
}

func TestUploadManifestNameFallsBackToAgentName(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.agents.Create(ctx, f.userID, agents.CreateInput{Name: "Ace-Agent"}); err != nil {
		t.Fatal(err)
	}

	archive := archiveWithManifestName(t, "My Tank 名前")
	if _, err := f.svc.UploadVersion(ctx, f.userID, archive); err != nil {
		t.Fatalf("upload: %v", err)
	}

	var name string
	err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT name FROM game_bots WHERE owner_user_id = $1`, f.userID).Scan(&name)
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "Ace-Agent" {
		t.Fatalf("expected the bot to fall back to the agent's name, got %q", name)
	}
}

func TestUploadFallsBackToRandomTankName(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// No agent, and a manifest name that doesn't match agents.NameRe.
	archive := archiveWithManifestName(t, "not a valid name!")
	if _, err := f.svc.UploadVersion(ctx, f.userID, archive); err != nil {
		t.Fatalf("upload: %v", err)
	}

	var name string
	err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT name FROM game_bots WHERE owner_user_id = $1`, f.userID).Scan(&name)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "tank-") || !agents.NameRe.MatchString(name) {
		t.Fatalf("expected a random tank-xxxxxx name matching agents.NameRe, got %q", name)
	}
}

func TestConcurrentUploadsGetDistinctVersionNumbers(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.svc.SaveBot(ctx, f.userID, "Racer"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	numbers := make([]int, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := f.svc.UploadVersion(ctx, f.userID, starterArchive(t))
			numbers[i], errs[i] = v.Number, err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}
	got := map[int]bool{numbers[0]: true, numbers[1]: true}
	if len(got) != 2 || !got[1] || !got[2] {
		t.Fatalf("expected version numbers 1 and 2, got %v", numbers)
	}
}

func TestMyTanks(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	empty, err := f.svc.MyTanks(ctx, f.userID)
	if err != nil {
		t.Fatalf("MyTanks with no bot: %v", err)
	}
	if empty.Bot != nil {
		t.Fatalf("expected no bot yet, got %+v", empty.Bot)
	}
	if empty.Versions == nil || len(empty.Versions) != 0 {
		t.Fatalf("expected an empty, non-nil Versions slice, got %#v", empty.Versions)
	}
	if empty.AgentRuns == nil || len(empty.AgentRuns) != 0 {
		t.Fatalf("expected an empty, non-nil AgentRuns slice, got %#v", empty.AgentRuns)
	}
	if empty.Matches == nil || len(empty.Matches) != 0 {
		t.Fatalf("expected an empty, non-nil Matches slice, got %#v", empty.Matches)
	}

	if _, err := f.svc.UploadVersion(ctx, f.userID, starterArchive(t)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	withBot, err := f.svc.MyTanks(ctx, f.userID)
	if err != nil {
		t.Fatalf("MyTanks with a bot: %v", err)
	}
	if withBot.Bot == nil || withBot.Bot.Name != "my-tank" {
		t.Fatalf("expected the uploaded bot, got %+v", withBot.Bot)
	}
	if len(withBot.Versions) != 1 || withBot.Versions[0].Status != "pending" {
		t.Fatalf("expected one pending version, got %+v", withBot.Versions)
	}

	a, err := f.agents.Create(ctx, f.userID, agents.CreateInput{Name: "fixer"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.agents.Heartbeat(ctx, a.ID, "0.1", "h"); err != nil {
		t.Fatal(err)
	}
	created, err := f.proofs.CreateWithRepo(ctx, f.userID, "tanks-bot", []byte("bot-repo"))
	if err != nil {
		t.Fatalf("create game_bot proof: %v", err)
	}
	claimed, _, err := f.proofs.Claim(ctx, a.ID)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if err := f.proofs.SubmitResult(ctx, a.ID, claimed.ID, proofs.ResultInput{Diff: "diff-body", LogTail: "log-body"}); err != nil {
		t.Fatal(err)
	}

	withRun, err := f.svc.MyTanks(ctx, f.userID)
	if err != nil {
		t.Fatalf("MyTanks with an agent run: %v", err)
	}
	if len(withRun.AgentRuns) != 1 || withRun.AgentRuns[0].ID != created.ID {
		t.Fatalf("expected the game_bot proof in AgentRuns, got %+v", withRun.AgentRuns)
	}
	if withRun.AgentRuns[0].Diff != "" || withRun.AgentRuns[0].AgentLogTail != "" {
		t.Fatalf("AgentRuns must not carry diff or log, got %+v", withRun.AgentRuns[0])
	}
	if withRun.AgentRuns[0].Status != proofs.StatusDiffSubmitted {
		t.Fatalf("expected status diff_submitted, got %s", withRun.AgentRuns[0].Status)
	}

	// The proof itself does carry the diff - proving AgentRuns' omission is deliberate, not accidental.
	full, err := f.proofs.Get(ctx, f.userID, created.ID)
	if err != nil || full.Diff != "diff-body" {
		t.Fatalf("expected the underlying proof to keep its diff: %v %+v", err, full)
	}
}

func TestUploadInvalidPackage(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.svc.UploadVersion(ctx, f.userID, []byte("not a tarball"))
	problem(t, err, 422, "invalid_package")
}

func TestUploadLimit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.svc.SaveBot(ctx, f.userID, "Limited"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if _, err := f.svc.UploadVersion(ctx, f.userID, starterArchive(t)); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}
	_, err := f.svc.UploadVersion(ctx, f.userID, starterArchive(t))
	problem(t, err, 429, "upload_limit")
}
