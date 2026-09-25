package games_test

import (
	"context"
	"errors"
	"log/slog"
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
	return fixture{d: d, svc: svc, users: us, agents: as, userID: u.ID}
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
