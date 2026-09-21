package competitions_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/fixtures/seed"
	"tolerance/internal/competitions"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
)

var admin = identity.Actor{Kind: identity.KindUser, ID: "user_admin", UserID: "user_admin", Role: "admin"}

func seedAdmin(t *testing.T, d *dbtest.DB) {
	t.Helper()
	if err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name, role) VALUES ('user_admin','seed','adm','admin','Admin','admin')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func code(t *testing.T, err error) string {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected problem, got %v", err)
	}
	return p.Code
}

func TestCompetitions_Lifecycle(t *testing.T) {
	d := dbtest.New(t)
	seedAdmin(t, d)
	ctx := context.Background()
	s := competitions.NewService(d.AppPool)

	c, err := s.Create(ctx, admin, valid())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, admin, valid()); code(t, err) != "slug_taken" {
		t.Fatal("duplicate slug must be slug_taken")
	}
	if _, err := s.GetPublicBySlug(ctx, c.Slug); code(t, err) != "not_found" {
		t.Fatal("drafts are not public")
	}
	if _, err := s.Publish(ctx, admin, c.ID, 999); code(t, err) != "state_conflict" {
		t.Fatal("stale expected_version must conflict")
	}
	pub, err := s.Publish(ctx, admin, c.ID, c.Version)
	if err != nil || pub.Status != competitions.StatusActive || pub.PublishedAt == nil {
		t.Fatalf("publish: %v %+v", err, pub)
	}
	in := valid()
	in.Points = 1
	if _, err := s.Update(ctx, admin, c.ID, in); code(t, err) != "state_conflict" {
		t.Fatal("published competitions are immutable via the service")
	}
	list, _ := s.ListPublic(ctx, "active")
	if len(list) != 1 || list[0].Public().Status != "active" || list[0].Participants != 0 {
		t.Fatalf("public list: %+v", list)
	}
	closed, err := s.Close(ctx, admin, c.ID, pub.Version, "done")
	if err != nil || closed.Status != competitions.StatusClosed || closed.Public().Status != "past" {
		t.Fatalf("close: %v %+v", err, closed)
	}
	if _, err := s.Close(ctx, admin, c.ID, closed.Version, "again"); code(t, err) != "state_conflict" {
		t.Fatal("closing twice must conflict")
	}
	if err := s.Delete(ctx, admin, c.ID); code(t, err) != "state_conflict" {
		t.Fatal("only drafts can be deleted")
	}
}

func TestCompetitions_ForbidsNonAdminAndCountsParticipants(t *testing.T) {
	d := dbtest.New(t)
	seedAdmin(t, d)
	ctx := context.Background()
	s := competitions.NewService(d.AppPool)
	user := identity.Actor{Kind: identity.KindUser, ID: "user_admin", UserID: "user_admin", Role: "user"}
	if _, err := s.Create(ctx, user, valid()); code(t, err) != "forbidden" {
		t.Fatal("non-admin must be forbidden")
	}
	c, _ := s.Create(ctx, admin, valid())
	c, _ = s.Publish(ctx, admin, c.ID, c.Version)
	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// A parameterized query cannot share a prepared statement with other
		// commands, so each statement here is its own Exec call.
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name) VALUES ('user_a','seed','a','a','A'), ('user_b','seed','b','b','B')`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO agents (id, owner_user_id, name, model) VALUES ('agent_a','user_a','A','m'), ('agent_b','user_b','B','m')`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, score_status, total, points_awarded)
			VALUES ('sub_a', $1, 'agent_a', 'manual', 'app', 's', 'scored', 80, 400), ('sub_b', $1, 'agent_b', 'manual', 'app', 's', 'pending', NULL, NULL)`, c.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetPublicBySlug(ctx, c.Slug)
	if got.Participants != 2 || got.ScoredCount != 1 {
		t.Fatalf("participants=%d scored=%d", got.Participants, got.ScoredCount)
	}
}

func TestCloseExpired(t *testing.T) {
	d := dbtest.New(t)
	seedAdmin(t, d)
	ctx := context.Background()
	s := competitions.NewService(d.AppPool)
	c, _ := s.Create(ctx, admin, valid())
	c, _ = s.Publish(ctx, admin, c.ID, c.Version)
	// Move the deadline into the past directly (Validate forbids it via the API).
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE competitions SET status = 'draft', published_at = NULL WHERE id = $1`, c.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE competitions SET deadline = now() - interval '1 second', status = 'active', published_at = now() - interval '1 day' WHERE id = $1`, c.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	n, err := s.CloseExpired(ctx)
	if err != nil || n != 1 {
		t.Fatalf("expected one closed, got %d %v", n, err)
	}
	got, _ := s.GetByID(ctx, c.ID)
	if got.Status != competitions.StatusClosed || got.ClosedAt == nil {
		t.Fatalf("not closed: %+v", got)
	}
	_ = time.Second
}

func TestCompetitions_TaskBundleAndDataset(t *testing.T) {
	d := dbtest.New(t)
	seedAdmin(t, d)
	ctx := context.Background()
	s := competitions.NewService(d.AppPool)

	in, err := seed.TaskCompetitionInput("city-day-planner", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Create(ctx, admin, in)
	if err != nil {
		t.Fatal(err)
	}
	if c.CheckSuite != "city-day-planner" || len(c.Task) == 0 {
		t.Fatalf("task bundle not stored: suite=%q task=%d bytes", c.CheckSuite, len(c.Task))
	}
	pub, err := s.Publish(ctx, admin, c.ID, c.Version)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPublicBySlug(ctx, pub.Slug)
	if err != nil {
		t.Fatal(err)
	}
	view := got.Public()
	var task map[string]any
	if err := json.Unmarshal(view.Task, &task); err != nil || task["what_to_build"] == nil {
		t.Fatalf("public view must carry the task: %v %v", err, task)
	}
	if view.Criteria[0].Source != "checks" {
		t.Fatalf("Functionality must be sourced from checks, got %q", view.Criteria[0].Source)
	}

	places, err := s.Dataset(ctx, pub.Slug)
	if err != nil {
		t.Fatal(err)
	}
	var ds struct {
		Places []json.RawMessage `json:"places"`
	}
	if err := json.Unmarshal(places, &ds); err != nil || len(ds.Places) != 24 {
		t.Fatalf("dataset: %v (%d places)", err, len(ds.Places))
	}

	plain, err := s.Create(ctx, admin, valid())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, admin, plain.ID, plain.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dataset(ctx, "weekend-planner"); code(t, err) != "not_found" {
		t.Fatal("a competition without a suite has no dataset")
	}
}

func TestCompetitions_ParticipantsCountOfficialSubmissionsOnly(t *testing.T) {
	d := dbtest.New(t)
	seedAdmin(t, d)
	ctx := context.Background()
	s := competitions.NewService(d.AppPool)
	c, err := s.Create(ctx, admin, valid())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, admin, c.ID, c.Version); err != nil {
		t.Fatal(err)
	}
	err = d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, h := range []string{"a", "b"} {
			if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name) VALUES ($1,'seed',$2,$2,$2)`, "user_"+h, h); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO agents (id, owner_user_id, name, model) VALUES ($1,$2,$3,'m')`, "agent_"+h, "user_"+h, "Agent"+h); err != nil {
				return err
			}
		}
		ins := `INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, attempt_kind, attempt_no, score_status, total)
			VALUES ($1,$2,$3,'manual','app','a summary long enough',$4,$5,$6,80)`
		// agent a: official (scored) plus a practice; agent b: practice only.
		for _, r := range [][]any{
			{"sub_a1", c.ID, "agent_a", "official", 1, "scored"},
			{"sub_a2", c.ID, "agent_a", "practice", 2, "scored"},
			{"sub_b1", c.ID, "agent_b", "practice", 1, "scored"},
		} {
			if _, err := tx.Exec(ctx, ins, r...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Participants != 1 {
		t.Fatalf("only agents with an official submission are participants, got %d", got.Participants)
	}
	if got.ScoredCount != 1 {
		t.Fatalf("scored_count counts official scored submissions, got %d", got.ScoredCount)
	}
}
