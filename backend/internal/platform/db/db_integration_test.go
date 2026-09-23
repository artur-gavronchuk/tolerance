package db_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/dbtest"
	"tolerance/migrations"
)

func TestMigrate_AppliesCleanlyAndTwice(t *testing.T) {
	d := dbtest.New(t)
	if err := db.Migrate(d.AdminDSN); err != nil {
		t.Fatalf("second migrate must be a no-op, got: %v", err)
	}
	ctx := context.Background()
	for _, table := range []string{"users", "agents", "api_keys", "competitions", "submissions", "judgments",
		"matches", "match_events", "arena_queue", "agent_badges", "jobs", "audit_events", "idempotency_records",
		"attempts", "attempt_events", "check_runs", "check_results", "evidence_blobs"} {
		err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var n int
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
		})
		if err != nil {
			t.Fatalf("arena_app must be able to read %s: %v", table, err)
		}
	}
	for _, view := range []string{"competition_rankings", "agent_standings"} {
		err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var n int
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+view).Scan(&n)
		})
		if err != nil {
			t.Fatalf("arena_app must be able to read view %s: %v", view, err)
		}
	}
}

func TestAppRole_HasNoBypassAndNoSuperuser(t *testing.T) {
	d := dbtest.New(t)
	var rolsuper, rolbypassrls bool
	err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'arena_app'`).Scan(&rolsuper, &rolbypassrls)
	})
	if err != nil {
		t.Fatalf("query role: %v", err)
	}
	if rolsuper || rolbypassrls {
		t.Fatalf("arena_app must be neither superuser nor bypassrls, got super=%v bypass=%v", rolsuper, rolbypassrls)
	}
}

func TestCompetitions_ImmutableAfterPublish(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name) VALUES ('user_a', 'seed', 'a', 'a', 'A')`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, criteria, created_by, published_at)
			VALUES ('comp_a', 'a', 'A', 's', 'b', 'Bug fix', 'Easy', 'active', 100, now() + interval '1 day',
			        '[{"name":"Tests","weight":100,"description":"d"}]', 'user_a', now())`)
		return err
	})
	if err != nil {
		t.Fatalf("arrange: %v", err)
	}
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE competitions SET points = 200 WHERE id = 'comp_a'`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("expected trigger to reject the update with an 'immutable' error, got: %v", err)
	}
	// Operational columns stay writable.
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE competitions SET status = 'closed', closed_at = now(), version = version + 1 WHERE id = 'comp_a'`)
		return err
	})
	if err != nil {
		t.Fatalf("closing a published competition must be allowed: %v", err)
	}
}

// arrangeCompetition inserts a user, an agent and a published competition
// (with a task bundle) through the owner role and returns their ids.
func arrangeCompetition(t *testing.T, d *dbtest.DB) (userID, agentID, competitionID string) {
	t.Helper()
	ctx := context.Background()
	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name) VALUES ('user_m', 'seed', 'm', 'm', 'M')`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO agents (id, owner_user_id, name, model) VALUES ('agent_m', 'user_m', 'Mira', 'model')`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, criteria, created_by, published_at, task, check_suite)
			VALUES ('comp_m', 'm', 'M', 's', 'b', 'Full build', 'Medium', 'active', 500, now() + interval '1 day',
			        '[{"name":"Functionality","weight":100,"description":"d","source":"checks"}]', 'user_m', now(), '{"version":"1"}', 'city-day-planner')`)
		return err
	})
	if err != nil {
		t.Fatalf("arrange: %v", err)
	}
	return "user_m", "agent_m", "comp_m"
}

func insertSubmission(ctx context.Context, tx pgx.Tx, id, kind string, no int) error {
	_, err := tx.Exec(ctx, `INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, attempt_kind, attempt_no)
		VALUES ($1, 'comp_m', 'agent_m', 'manual', 'app', 'a summary long enough', $2, $3)`, id, kind, no)
	return err
}

func TestSchema_OneOfficialSubmissionButManyPractice(t *testing.T) {
	d := dbtest.New(t)
	arrangeCompetition(t, d)
	ctx := context.Background()

	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := insertSubmission(ctx, tx, "sub_1", "official", 1); err != nil {
			return err
		}
		if err := insertSubmission(ctx, tx, "sub_2", "practice", 2); err != nil {
			return err
		}
		return insertSubmission(ctx, tx, "sub_3", "practice", 3)
	})
	if err != nil {
		t.Fatalf("one official plus several practice submissions must be accepted: %v", err)
	}
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return insertSubmission(ctx, tx, "sub_4", "official", 4)
	})
	if err == nil || !strings.Contains(err.Error(), "submissions_one_official") {
		t.Fatalf("a second official submission must violate submissions_one_official, got: %v", err)
	}
}

func TestSchema_OneOfficialAttemptUntilVoided(t *testing.T) {
	d := dbtest.New(t)
	_, _, comp := arrangeCompetition(t, d)
	ctx := context.Background()
	insert := func(id, kind string, no int) error {
		return d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO attempts (id, competition_id, agent_id, kind, attempt_no, status, agent_snapshot)
				VALUES ($1, $2, 'agent_m', $3, $4, 'submitted', '{}')`, id, comp, kind, no)
			return err
		})
	}
	if err := insert("att_1", "official", 1); err != nil {
		t.Fatal(err)
	}
	if err := insert("att_2", "official", 2); err == nil || !strings.Contains(err.Error(), "attempts_one_official") {
		t.Fatalf("second official attempt must be rejected, got: %v", err)
	}
	if err := insert("att_3", "practice", 2); err != nil {
		t.Fatalf("practice attempt must be accepted: %v", err)
	}
	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE attempts SET voided_at = now(), void_reason = 'admin decision' WHERE id = 'att_1'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := insert("att_4", "official", 3); err != nil {
		t.Fatalf("a new official attempt must be allowed once the previous one is voided: %v", err)
	}
}

func TestSchema_TaskImmutableAfterPublish(t *testing.T) {
	d := dbtest.New(t)
	arrangeCompetition(t, d)
	ctx := context.Background()
	for _, stmt := range []string{
		`UPDATE competitions SET task = '{}' WHERE id = 'comp_m'`,
		`UPDATE competitions SET check_suite = 'other' WHERE id = 'comp_m'`,
	} {
		err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, stmt)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s: expected the immutability trigger, got: %v", stmt, err)
		}
	}
}

func TestSchema_AppRoleCanWriteNewTables(t *testing.T) {
	d := dbtest.New(t)
	_, _, comp := arrangeCompetition(t, d)
	ctx := context.Background()
	err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO attempts (id, competition_id, agent_id, kind, attempt_no, agent_snapshot)
			VALUES ('att_w', $1, 'agent_m', 'official', 1, '{}')`, comp); err != nil {
			return err
		}
		// attempt_events.id is a bigserial: this needs USAGE on its sequence.
		_, err := tx.Exec(ctx, `INSERT INTO attempt_events (attempt_id, kind) VALUES ('att_w', 'started')`)
		return err
	})
	if err != nil {
		t.Fatalf("arena_app must be able to write attempts and attempt_events: %v", err)
	}
}

func TestSchema_RankingsIgnorePracticeSubmissions(t *testing.T) {
	d := dbtest.New(t)
	arrangeCompetition(t, d)
	ctx := context.Background()
	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := insertSubmission(ctx, tx, "sub_o", "official", 1); err != nil {
			return err
		}
		if err := insertSubmission(ctx, tx, "sub_p", "practice", 2); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE submissions SET score_status = 'scored', total = 90, points_awarded = 450 WHERE id = 'sub_o'`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE submissions SET score_status = 'scored', total = 100, points_awarded = 500 WHERE id = 'sub_p'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var ranked, points, subs int
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM competition_rankings`).Scan(&ranked); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT points, submissions FROM agent_standings WHERE agent_id = 'agent_m'`).Scan(&points, &subs)
	})
	if err != nil {
		t.Fatal(err)
	}
	if ranked != 1 || points != 450 || subs != 1 {
		t.Fatalf("practice must not count: ranked=%d points=%d submissions=%d (want 1, 450, 1)", ranked, points, subs)
	}
}

func TestMigrate_DownAndUpAgain(t *testing.T) {
	d := dbtest.New(t)
	sqlDB, err := sql.Open("pgx", d.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	exists := func(table string) bool {
		var ok bool
		if err := sqlDB.QueryRow(`SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !exists("attempts") {
		t.Fatal("attempts must exist after Up")
	}
	if err := goose.DownTo(sqlDB, ".", 2); err != nil {
		t.Fatalf("down to 2: %v", err)
	}
	if exists("attempts") || exists("check_runs") || exists("evidence_blobs") {
		t.Fatal("00003 tables must be gone after Down")
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if !exists("attempts") || !exists("evidence_blobs") {
		t.Fatal("00003 tables must exist after re-applying Up")
	}
}
