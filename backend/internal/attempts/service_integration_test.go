package attempts_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/fixtures/seed"
	"tolerance/internal/agents"
	"tolerance/internal/attempts"
	"tolerance/internal/competitions"
	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/standings"
)

const slug = "city-day-planner"

var adminActor = identity.Actor{Kind: identity.KindUser, ID: "user_admin", UserID: "user_admin", Role: "admin"}

func problem(t *testing.T, err error) *httpx.Problem {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected a problem, got %v", err)
	}
	return p
}

type fixture struct {
	d      *dbtest.DB
	svc    *attempts.Service
	agents *agents.Service
	compID string
}

func (f *fixture) newAgent(t *testing.T, name string) identity.Actor {
	t.Helper()
	ctx := context.Background()
	users := identity.NewService(f.d.AppPool, nil)
	u, err := users.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: name, Email: name + "@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.agents.Create(ctx, u.ID, agents.CreateInput{Name: name, Model: "model-a", Bio: "original bio"})
	if err != nil {
		t.Fatal(err)
	}
	return identity.Actor{Kind: identity.KindAgent, ID: p.ID, UserID: u.ID, AgentID: p.ID}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name, role) VALUES ('user_admin','seed','adm','admin','Admin','admin')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	comps := competitions.NewService(d.AppPool)
	in, err := seed.TaskCompetitionInput(slug, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c, err := comps.Create(ctx, adminActor, in)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := comps.Publish(ctx, adminActor, c.ID, c.Version)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{d: d, svc: attempts.NewService(d.AppPool), agents: agents.NewService(d.AppPool, standings.NewService(d.AppPool)), compID: pub.ID}
}

var cfg = attempts.AgentConfig{Adapter: "claude-code", AdapterModel: "sonnet", ConnectorVersion: "0.1.0", OS: "darwin/arm64"}

// submit marks the attempt as submitted the way the submissions module will.
func (f *fixture) markSubmitted(t *testing.T, attemptID string) {
	t.Helper()
	err := f.d.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return f.svc.MarkSubmitted(ctx, tx, attemptID)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) start(t *testing.T, a identity.Actor, kind string) attempts.Attempt {
	t.Helper()
	at, _, err := f.svc.Start(context.Background(), a, slug, kind, cfg)
	if err != nil {
		t.Fatalf("start %s: %v", kind, err)
	}
	return at
}

func TestAttempts_OfficialOnlyOnce(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	first := f.start(t, a, "official")
	if first.Kind != "official" || first.No != 1 || first.Status != "running" || first.CompetitionSlug != slug {
		t.Fatalf("unexpected first attempt: %+v", first)
	}
	f.markSubmitted(t, first.ID)

	if _, _, err := f.svc.Start(context.Background(), a, slug, "official", cfg); problem(t, err).Code != "official_attempt_used" {
		t.Fatalf("a second official attempt must be refused, got %v", err)
	}
	if ok, err := f.svc.OfficialAvailable(context.Background(), a.AgentID, f.compID); err != nil || ok {
		t.Fatalf("official must be unavailable: %v %v", ok, err)
	}
}

func TestAttempts_StartResumesRunning(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	first, resumed, err := f.svc.Start(context.Background(), a, slug, "practice", cfg)
	if err != nil || resumed {
		t.Fatalf("first start: %v resumed=%v", err, resumed)
	}
	again, resumed, err := f.svc.Start(context.Background(), a, slug, "practice", cfg)
	if err != nil || !resumed || again.ID != first.ID {
		t.Fatalf("second start must resume the running attempt: %v resumed=%v %s vs %s", err, resumed, again.ID, first.ID)
	}
}

func TestAttempts_ConcurrentStartsCreateOneAttempt(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	var mu sync.Mutex
	ids := map[string]bool{}
	fresh := 0
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			at, resumed, err := f.svc.Start(context.Background(), a, slug, "official", cfg)
			if err != nil {
				t.Errorf("start: %v", err)
				return
			}
			mu.Lock()
			ids[at.ID] = true
			if !resumed {
				fresh++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(ids) != 1 || fresh != 1 {
		t.Fatalf("concurrent starts must yield one attempt: %d ids, %d created", len(ids), fresh)
	}
}

func TestAttempts_PracticeAfterOfficialKeepsNumbering(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	off := f.start(t, a, "official")
	f.markSubmitted(t, off.ID)
	p1 := f.start(t, a, "practice")
	if p1.Kind != "practice" || p1.No != 2 {
		t.Fatalf("practice after official must be attempt #2: %+v", p1)
	}
	f.markSubmitted(t, p1.ID)
	p2 := f.start(t, a, "practice")
	if p2.No != 3 {
		t.Fatalf("numbering is continuous: %+v", p2)
	}
	list, err := f.svc.ListForAgent(context.Background(), a.AgentID, f.compID)
	if err != nil || len(list) != 3 || list[0].No != 1 || list[2].No != 3 {
		t.Fatalf("list: %+v %v", list, err)
	}
}

func TestAttempts_SnapshotSurvivesProfileEdit(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	first := f.start(t, a, "official")
	f.markSubmitted(t, first.ID)

	newModel := "model-b"
	if _, err := f.agents.Patch(context.Background(), a.UserID, agents.PatchInput{Model: &newModel}); err != nil {
		t.Fatal(err)
	}
	second := f.start(t, a, "practice")

	var old, fresh attempts.Snapshot
	list, _ := f.svc.ListForAgent(context.Background(), a.AgentID, f.compID)
	if err := json.Unmarshal(list[0].AgentSnapshot, &old); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.AgentSnapshot, &fresh); err != nil {
		t.Fatal(err)
	}
	if old.Model != "model-a" || fresh.Model != "model-b" {
		t.Fatalf("editing the profile must not rewrite history: old=%q fresh=%q", old.Model, fresh.Model)
	}
	if old.Name != "Atlas" || old.Bio != "original bio" || old.Adapter != "claude-code" || old.AdapterModel != "sonnet" ||
		old.ConnectorVersion != "0.1.0" || old.OS != "darwin/arm64" {
		t.Fatalf("snapshot is missing fields: %+v", old)
	}
}

func TestAttempts_VoidAllowsNewOfficial(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	first := f.start(t, a, "official")
	if err := f.svc.Abandon(context.Background(), a, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Start(context.Background(), a, slug, "official", cfg); problem(t, err).Code != "official_attempt_used" {
		t.Fatalf("abandoning does not free the official slot: %v", err)
	}
	if err := f.svc.Void(context.Background(), adminActor, first.ID, "abandoned by mistake, granting a retry"); err != nil {
		t.Fatal(err)
	}
	retry, _, err := f.svc.Start(context.Background(), a, slug, "official", cfg)
	if err != nil {
		t.Fatalf("after a void a new official attempt is allowed: %v", err)
	}
	if retry.No != 2 || retry.Kind != "official" {
		t.Fatalf("the retry keeps counting: %+v", retry)
	}
	if err := f.svc.Void(context.Background(), adminActor, first.ID, "second void of the same attempt"); problem(t, err).Code != "state_conflict" {
		t.Fatalf("voiding twice must conflict: %v", err)
	}
}

func TestAttempts_VoidRules(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	at := f.start(t, a, "official")
	ctx := context.Background()

	if err := f.svc.Void(ctx, a, at.ID, "an agent may not void anything"); problem(t, err).Code != "forbidden" {
		t.Fatalf("only admins void: %v", err)
	}
	if err := f.svc.Void(ctx, adminActor, at.ID, "short"); problem(t, err).Code != "invalid_body" {
		t.Fatalf("a reason of 10+ characters is required: %v", err)
	}
	if err := f.svc.Void(ctx, adminActor, "att_missing", "a perfectly long reason"); problem(t, err).Code != "not_found" {
		t.Fatalf("unknown attempt: %v", err)
	}

	f.markSubmitted(t, at.ID)
	err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, attempt_id)
			VALUES ('sub_x', $1, $2, 'manual', 'app', 'a summary long enough', $3)`, f.compID, a.AgentID, at.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Void(ctx, adminActor, at.ID, "trying to erase a submitted attempt"); problem(t, err).Code != "has_submission" {
		t.Fatalf("an attempt with a submission cannot be voided: %v", err)
	}

	var n int
	_ = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action = 'attempt.voided'`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("refused voids must not be audited as done, found %d", n)
	}
}

func TestAttempts_EventsAreStoredSanitised(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	at := f.start(t, a, "practice")
	ctx := context.Background()
	three, zero := 3, 0
	err := f.svc.AddEvents(ctx, a, at.ID, []attempts.EventInput{
		{Kind: "phase", PhaseIndex: &three, Text: "Writing the scheduling logic"},
		{Kind: "log", Text: "export ANTHROPIC_API_KEY=sk-ant-abc123def456 \x1b[31m!\x1b[0m"},
		{Kind: "log", Text: "\x00\x01"}, // nothing left after cleaning: dropped
		{Kind: "phase", PhaseIndex: &zero, Text: ""},
		{Kind: "preview_available", Text: "Deployed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := f.svc.Events(ctx, at.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, e := range events {
		kinds = append(kinds, e.Kind)
		if e.Text != nil && (strings.Contains(*e.Text, "sk-ant") || strings.Contains(*e.Text, "\x1b")) {
			t.Fatalf("unsanitised text stored: %q", *e.Text)
		}
	}
	if got := strings.Join(kinds, ","); got != "started,phase,log,phase,preview_available" {
		t.Fatalf("events: %s", got)
	}
	// Cursor: only events after the given id.
	after, _ := f.svc.Events(ctx, at.ID, events[2].ID, 100)
	if len(after) != 2 {
		t.Fatalf("cursor must return the 2 later events, got %d", len(after))
	}
}

func TestAttempts_PhaseMayGoBack(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	at := f.start(t, a, "practice")
	five, two := 5, 2
	err := f.svc.AddEvents(context.Background(), a, at.ID, []attempts.EventInput{
		{Kind: "phase", PhaseIndex: &five}, {Kind: "phase", PhaseIndex: &two, Text: "Back to planning"},
	})
	if err != nil {
		t.Fatalf("returning to an earlier phase is allowed: %v", err)
	}
	events, _ := f.svc.Events(context.Background(), at.ID, 0, 100)
	last := events[len(events)-1]
	if last.PhaseIndex == nil || *last.PhaseIndex != 2 {
		t.Fatalf("the later event must keep phase 2: %+v", last)
	}
}

func TestAttempts_EventsRejectBadInput(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	at := f.start(t, a, "practice")
	ctx := context.Background()
	nine, one := 9, 1
	cases := map[string][]attempts.EventInput{
		"server kind result":    {{Kind: "result", Text: "score 100"}},
		"server kind submitted": {{Kind: "submitted"}},
		"unknown kind":          {{Kind: "nope"}},
		"phase out of range":    {{Kind: "phase", PhaseIndex: &nine}},
		"phase without index":   {{Kind: "phase", Text: "x"}},
		"empty batch":           {},
		"too many events":       make([]attempts.EventInput, 21),
		"bad event among good":  {{Kind: "phase", PhaseIndex: &one}, {Kind: "check_finished"}},
	}
	for i := range cases["too many events"] {
		cases["too many events"][i] = attempts.EventInput{Kind: "log", Text: "x"}
	}
	for name, batch := range cases {
		if err := f.svc.AddEvents(ctx, a, at.ID, batch); problem(t, err).Code != "invalid_body" {
			t.Errorf("%s: want invalid_body, got %v", name, err)
		}
	}
	events, _ := f.svc.Events(ctx, at.ID, 0, 100)
	if len(events) != 1 {
		t.Fatalf("rejected batches must store nothing; found %d events", len(events))
	}
}

func TestAttempts_EventsOwnershipAndState(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	b := f.newAgent(t, "Nova")
	at := f.start(t, a, "practice")
	ctx := context.Background()
	log := []attempts.EventInput{{Kind: "log", Text: "hello"}}

	if err := f.svc.AddEvents(ctx, b, at.ID, log); problem(t, err).Code != "not_found" {
		t.Fatalf("another agent's attempt looks like it does not exist: %v", err)
	}
	if err := f.svc.AddEvents(ctx, a, "att_missing", log); problem(t, err).Code != "not_found" {
		t.Fatalf("unknown attempt: %v", err)
	}
	if err := f.svc.Abandon(ctx, b, at.ID); problem(t, err).Code != "not_found" {
		t.Fatalf("another agent cannot abandon it: %v", err)
	}
	if err := f.svc.Abandon(ctx, a, at.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Abandon(ctx, a, at.ID); err != nil {
		t.Fatalf("abandoning twice is a no-op: %v", err)
	}
	if err := f.svc.AddEvents(ctx, a, at.ID, log); problem(t, err).Code != "attempt_not_running" {
		t.Fatalf("no events after abandoning: %v", err)
	}
	user := identity.Actor{Kind: identity.KindUser, ID: "user_x", UserID: "user_x", Role: "user"}
	if err := f.svc.AddEvents(ctx, user, at.ID, log); problem(t, err).Code != "forbidden" {
		t.Fatalf("only agents send events: %v", err)
	}
}

func TestAttempts_EventsAreRateLimitedPerAttempt(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	at := f.start(t, a, "practice")
	other := f.start(t, f.newAgent(t, "Nova"), "practice")
	ctx := context.Background()
	log := []attempts.EventInput{{Kind: "log", Text: "tick"}}
	limited := 0
	for i := 0; i < 10; i++ {
		if err := f.svc.AddEvents(ctx, a, at.ID, log); err != nil {
			if problem(t, err).Code != "rate_limited" {
				t.Fatalf("unexpected error: %v", err)
			}
			limited++
		}
	}
	if limited == 0 {
		t.Fatal("ten immediate batches must hit the limit")
	}
	nova := identity.Actor{Kind: identity.KindAgent, ID: other.AgentID, AgentID: other.AgentID}
	if err := f.svc.AddEvents(ctx, nova, other.ID, log); err != nil {
		t.Fatalf("the limit is per attempt: %v", err)
	}
}

func TestAttempts_DeadlineAndInactiveCompetition(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	ctx := context.Background()
	// Push the deadline into the past directly; the immutability trigger
	// guards published competitions, so disable it for this arrangement only.
	err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `ALTER TABLE competitions DISABLE TRIGGER competitions_immutable`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE competitions SET deadline = now() - interval '1 minute' WHERE id = $1`, f.compID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `ALTER TABLE competitions ENABLE TRIGGER competitions_immutable`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Start(ctx, a, slug, "official", cfg); problem(t, err).Code != "deadline_passed" {
		t.Fatalf("after the deadline: %v", err)
	}
	if _, _, err := f.svc.Start(ctx, a, "no-such-competition", "official", cfg); problem(t, err).Code != "not_found" {
		t.Fatalf("unknown competition: %v", err)
	}
	if _, _, err := f.svc.Start(ctx, a, slug, "bogus", cfg); problem(t, err).Code != "invalid_body" {
		t.Fatalf("unknown kind: %v", err)
	}
	long := cfg
	long.OS = strings.Repeat("x", 81)
	if _, _, err := f.svc.Start(ctx, a, slug, "practice", long); problem(t, err).Code != "invalid_body" {
		t.Fatalf("over-long config field: %v", err)
	}
}

func TestAttempts_SystemEventsAndMarkSubmitted(t *testing.T) {
	f := newFixture(t)
	a := f.newAgent(t, "Atlas")
	at := f.start(t, a, "official")
	ctx := context.Background()
	err := f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := f.svc.RecordSystemEvent(ctx, tx, at.ID, "submitted", map[string]any{"submission_id": "sub_1"}); err != nil {
			return err
		}
		if err := f.svc.RecordSystemEvent(ctx, tx, at.ID, "log", nil); err == nil {
			t.Error("a connector kind is not a system event")
		}
		return f.svc.MarkSubmitted(ctx, tx, at.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error { return f.svc.MarkSubmitted(ctx, tx, at.ID) })
	if problem(t, err).Code != "attempt_not_running" {
		t.Fatalf("submitting twice: %v", err)
	}
	list, _ := f.svc.ListForAgent(ctx, a.AgentID, f.compID)
	if list[0].Status != "submitted" || list[0].FinishedAt == nil {
		t.Fatalf("attempt must be closed: %+v", list[0])
	}
}
