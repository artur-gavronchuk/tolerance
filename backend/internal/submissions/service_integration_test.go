package submissions_test

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
	"tolerance/internal/submissions"
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
	at     *attempts.Service
	svc    *submissions.Service
	agents *agents.Service
	comps  *competitions.Service
	compID string
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
	at := attempts.NewService(d.AppPool)
	return &fixture{d: d, at: at, svc: submissions.NewService(d.AppPool, at, "http://localhost:3000/", false),
		agents: agents.NewService(d.AppPool, standings.NewService(d.AppPool)), comps: comps, compID: pub.ID}
}

func (f *fixture) newAgent(t *testing.T, name string) (identity.Actor, string) {
	t.Helper()
	ctx := context.Background()
	u, err := identity.NewService(f.d.AppPool, nil).ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: name, Email: name + "@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.agents.Create(ctx, u.ID, agents.CreateInput{Name: name, Model: "model-a", Bio: "bio"})
	if err != nil {
		t.Fatal(err)
	}
	return identity.Actor{Kind: identity.KindAgent, ID: p.ID, UserID: u.ID, AgentID: p.ID}, u.ID
}

var cfg = attempts.AgentConfig{Adapter: "claude-code", AdapterModel: "sonnet", ConnectorVersion: "0.1.0", OS: "darwin/arm64"}

func (f *fixture) start(t *testing.T, a identity.Actor, kind string) attempts.Attempt {
	t.Helper()
	at, _, err := f.at.Start(context.Background(), a, slug, kind, cfg)
	if err != nil {
		t.Fatalf("start %s: %v", kind, err)
	}
	return at
}

func input() submissions.Input {
	return submissions.Input{Summary: "Static planner with greedy scheduling, remove/replace and localStorage.",
		PreviewURL: "https://a.github.io/planner/"}
}

func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

// score forces a submission into a scored state, as the checks module will.
func (f *fixture) score(t *testing.T, id string, total, points int) {
	t.Helper()
	err := f.d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE submissions SET score_status = 'scored', total = $2, points_awarded = $3, judged_at = now() WHERE id = $1`, id, total, points)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubmit_CreatesPendingSubmissionAndCheckJob(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	att := f.start(t, a, "official")
	ctx := context.Background()

	in := input()
	in.RepoURL, in.CommitSHA = "https://github.com/o/r", strings.Repeat("a", 40)
	in.Cost = &submissions.Cost{USD: 1.42, Source: "claude-code-cli"}
	in.Notes = "deployed to pages"
	got, err := f.svc.Submit(ctx, a, att.ID, in)
	if err != nil {
		t.Fatal(err)
	}

	if got.ResultURL != "http://localhost:3000/submissions/"+got.ID {
		t.Fatalf("result url: %q", got.ResultURL)
	}
	if got.ScoreStatus != "pending" || got.Total != nil || got.Scores != nil || got.PointsAwarded != nil || got.Rank != nil {
		t.Fatalf("a new submission is pending and unscored: %+v", got.View)
	}
	if got.Agent != "Atlas" || got.CompetitionSlug != slug || got.Source != "manual" || got.Artifact != "app" {
		t.Fatalf("identity fields: %+v", got.View)
	}
	if got.Attempt.Kind != "official" || got.Attempt.No != 1 || got.Attempt.ID == nil || *got.Attempt.ID != att.ID ||
		got.Attempt.StartedAt == nil || got.Attempt.DurationSeconds == nil {
		t.Fatalf("attempt info: %+v", got.Attempt)
	}
	var snap attempts.Snapshot
	if err := json.Unmarshal(got.AgentSnapshot, &snap); err != nil || snap.Model != "model-a" || snap.Adapter != "claude-code" || snap.OS != "darwin/arm64" {
		t.Fatalf("snapshot copied from the attempt: %+v %v", snap, err)
	}
	if got.Verification != "self_reported" || got.CommitLink != "unverified" || got.CommitSHA == nil || *got.CommitSHA != in.CommitSHA {
		t.Fatalf("verification and commit: %+v", got.View)
	}
	var cost submissions.Cost
	if err := json.Unmarshal(got.ReportedCost, &cost); err != nil || cost.USD != 1.42 || cost.Source != "claude-code-cli" {
		t.Fatalf("reported cost: %s %v", got.ReportedCost, err)
	}
	if got.CheckRun == nil || got.CheckRun.Status != "queued" || got.CheckRun.Suite != slug || got.CheckRun.SuiteVersion != "1" || got.CheckRun.FunctionalScore != nil {
		t.Fatalf("check run: %+v", got.CheckRun)
	}
	want := []string{"self_reported_run", "mutable_preview_url", "commit_unverified"}
	if strings.Join(got.Limitations, ",") != strings.Join(want, ",") {
		t.Fatalf("limitations: %v, want %v", got.Limitations, want)
	}

	if n := f.count(t, `SELECT count(*) FROM jobs WHERE kind = 'check_submission' AND state = 'queued' AND payload->>'submission_id' = $1 AND payload->>'check_run_id' = $2`, got.ID, got.CheckRun.ID); n != 1 {
		t.Fatalf("exactly one check job must be queued, got %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM attempts WHERE id = $1 AND status = 'submitted' AND finished_at IS NOT NULL`, att.ID); n != 1 {
		t.Fatal("the attempt must be closed")
	}
	if n := f.count(t, `SELECT count(*) FROM attempt_events WHERE attempt_id = $1 AND kind = 'submitted'`, att.ID); n != 1 {
		t.Fatal("a submitted event must be recorded")
	}
	if n := f.count(t, `SELECT count(*) FROM audit_events WHERE action = 'submission.created' AND aggregate_id = $1`, got.ID); n != 1 {
		t.Fatal("the submission must be audited")
	}
}

func TestSubmit_MinimalInputHasNoOptionalFields(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	att := f.start(t, a, "practice")
	got, err := f.svc.Submit(context.Background(), a, att.ID, input())
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoURL != nil || got.CommitSHA != nil || got.Notes != nil || got.ReportedCost != nil && string(got.ReportedCost) != "null" || got.CommitLink != "not_provided" {
		t.Fatalf("optional fields must stay empty: %+v", got.View)
	}
	if !contains(got.Limitations, "commit_not_provided") {
		t.Fatalf("limitations: %v", got.Limitations)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestSubmit_RejectsInvalidInputWithoutSideEffects(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	att := f.start(t, a, "official")
	bad := input()
	bad.PreviewURL = "http://evil.example.com"
	if _, err := f.svc.Submit(context.Background(), a, att.ID, bad); problem(t, err).Code != "invalid_body" {
		t.Fatalf("invalid input: %v", err)
	}
	if f.count(t, `SELECT count(*) FROM submissions`) != 0 || f.count(t, `SELECT count(*) FROM jobs`) != 0 {
		t.Fatal("a rejected submission must leave nothing behind")
	}
	if n := f.count(t, `SELECT count(*) FROM attempts WHERE id = $1 AND status = 'running'`, att.ID); n != 1 {
		t.Fatal("the attempt must stay running so the participant can fix the input")
	}
}

func TestSubmit_SecondOnSameAttemptIsRefused(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	att := f.start(t, a, "official")
	if _, err := f.svc.Submit(context.Background(), a, att.ID, input()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(context.Background(), a, att.ID, input()); problem(t, err).Code != "already_submitted" {
		t.Fatalf("a second submission for the attempt: %v", err)
	}
}

func TestSubmit_ConcurrentSubmitsCreateOne(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	att := f.start(t, a, "official")
	var mu sync.Mutex
	created, refused := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Submit(context.Background(), a, att.ID, input())
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				created++
			case problem(t, err).Code == "already_submitted":
				refused++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if created != 1 || refused != 5 {
		t.Fatalf("exactly one concurrent submit may win: created=%d refused=%d", created, refused)
	}
	if f.count(t, `SELECT count(*) FROM submissions`) != 1 || f.count(t, `SELECT count(*) FROM jobs WHERE kind = 'check_submission'`) != 1 {
		t.Fatal("one submission and one check job")
	}
}

func TestSubmit_DeadlineBoundary(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	ctx := context.Background()
	setDeadline := func(sql string) {
		t.Helper()
		// The immutability trigger guards published competitions; lift it for this arrangement only.
		err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			for _, q := range []string{`ALTER TABLE competitions DISABLE TRIGGER competitions_immutable`,
				`UPDATE competitions SET deadline = ` + sql + ` WHERE id = '` + f.compID + `'`,
				`ALTER TABLE competitions ENABLE TRIGGER competitions_immutable`} {
				if _, err := tx.Exec(ctx, q); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	first := f.start(t, a, "practice")
	setDeadline(`now() + interval '2 seconds'`)
	if _, err := f.svc.Submit(ctx, a, first.ID, input()); err != nil {
		t.Fatalf("a submission just before the deadline is accepted: %v", err)
	}
	second := f.start(t, a, "practice")
	time.Sleep(2200 * time.Millisecond)
	if _, err := f.svc.Submit(ctx, a, second.ID, input()); problem(t, err).Code != "deadline_passed" {
		t.Fatalf("a submission after the deadline: %v", err)
	}
	if f.count(t, `SELECT count(*) FROM submissions`) != 1 {
		t.Fatal("the late submission must not exist")
	}
}

func TestSubmit_AttemptOwnershipAndState(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	b, _ := f.newAgent(t, "Nova")
	ctx := context.Background()
	att := f.start(t, a, "official")

	if _, err := f.svc.Submit(ctx, b, att.ID, input()); problem(t, err).Code != "not_found" {
		t.Fatalf("another agent's attempt: %v", err)
	}
	if _, err := f.svc.Submit(ctx, a, "att_missing", input()); problem(t, err).Code != "not_found" {
		t.Fatalf("unknown attempt: %v", err)
	}
	user := identity.Actor{Kind: identity.KindUser, ID: "user_x", UserID: "user_x", Role: "user"}
	if _, err := f.svc.Submit(ctx, user, att.ID, input()); problem(t, err).Code != "forbidden" {
		t.Fatalf("only agents submit: %v", err)
	}
	if err := f.at.Abandon(ctx, a, att.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(ctx, a, att.ID, input()); problem(t, err).Code != "attempt_not_running" {
		t.Fatalf("an abandoned attempt: %v", err)
	}
	if err := f.at.Void(ctx, adminActor, att.ID, "abandoned by mistake, retry granted"); err != nil {
		t.Fatal(err)
	}
	retry := f.start(t, a, "official")
	if err := f.at.Void(ctx, adminActor, retry.ID, "voiding the retry as well"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(ctx, a, retry.ID, input()); problem(t, err).Code != "attempt_not_running" {
		t.Fatalf("a voided attempt: %v", err)
	}
}

func TestSubmit_CompetitionWithoutSuiteTakesNoConnectorSubmissions(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	ctx := context.Background()
	plain := competitions.Input{Slug: "plain-one", Title: "T", Summary: "S", Brief: "B", Category: "Bug fix", Difficulty: "Easy", Points: 100,
		Deadline: time.Now().Add(48 * time.Hour), MatchDurationSeconds: 900,
		Criteria: []competitions.Criterion{{Name: "Tests", Weight: 100, Description: "d"}}}
	c, err := f.comps.Create(ctx, adminActor, plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.comps.Publish(ctx, adminActor, c.ID, c.Version); err != nil {
		t.Fatal(err)
	}
	att, _, err := f.at.Start(ctx, a, "plain-one", "practice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Submit(ctx, a, att.ID, input()); problem(t, err).Code != "state_conflict" {
		t.Fatalf("no check suite, no connector submission: %v", err)
	}
}

func TestSubmit_PracticeIsNeverRanked(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	b, _ := f.newAgent(t, "Nova")
	ctx := context.Background()

	official, err := f.svc.Submit(ctx, a, f.start(t, a, "official").ID, input())
	if err != nil {
		t.Fatal(err)
	}
	practice, err := f.svc.Submit(ctx, b, f.start(t, b, "practice").ID, input())
	if err != nil {
		t.Fatal(err)
	}
	f.score(t, official.ID, 80, 400)
	f.score(t, practice.ID, 100, 500) // a better practice score must not outrank the official one

	list, err := f.svc.ListForCompetition(ctx, slug, submissions.ListOptions{})
	if err != nil || len(list) != 1 || list[0].ID != official.ID {
		t.Fatalf("the default competition list is official only: %+v %v", list, err)
	}
	if list[0].Rank == nil || *list[0].Rank != 1 || *list[0].RankOf != 1 {
		t.Fatalf("rank of the only official result: %+v", list[0].Rank)
	}
	all, err := f.svc.ListForCompetition(ctx, slug, submissions.ListOptions{Kind: "all"})
	if err != nil || len(all) != 2 {
		t.Fatalf("kind=all: %+v %v", all, err)
	}
	if all[0].ID != practice.ID || all[0].Rank != nil {
		t.Fatalf("a practice submission has no rank even when its score is higher: %+v", all[0])
	}
	only, _ := f.svc.ListForCompetition(ctx, slug, submissions.ListOptions{Kind: "practice"})
	if len(only) != 1 || only[0].ID != practice.ID {
		t.Fatalf("kind=practice: %+v", only)
	}
	if _, err := f.svc.ListForCompetition(ctx, slug, submissions.ListOptions{Kind: "bogus"}); problem(t, err).Code != "invalid_body" {
		t.Fatalf("unknown kind: %v", err)
	}
}

func TestList_OrderingAndPending(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ids := map[string]string{}
	for _, name := range []string{"Aa", "Bb", "Cc", "Dd"} {
		a, _ := f.newAgent(t, name)
		got, err := f.svc.Submit(ctx, a, f.start(t, a, "official").ID, input())
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = got.ID
		time.Sleep(15 * time.Millisecond)
	}
	f.score(t, ids["Aa"], 70, 350)
	f.score(t, ids["Bb"], 90, 450)
	f.score(t, ids["Cc"], 90, 450) // ties with Bb; the earlier submission ranks first
	err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE submissions SET score_status = 'unverifiable' WHERE id = $1`, ids["Dd"])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	scored, err := f.svc.ListForCompetition(ctx, slug, submissions.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, v := range scored {
		order = append(order, v.Agent)
	}
	if strings.Join(order, ",") != "Bb,Cc,Aa" {
		t.Fatalf("scored order: %v", order)
	}
	if *scored[0].Rank != 1 || *scored[1].Rank != 2 || *scored[2].Rank != 3 || *scored[2].RankOf != 3 {
		t.Fatalf("ranks: %d %d %d of %d", *scored[0].Rank, *scored[1].Rank, *scored[2].Rank, *scored[2].RankOf)
	}
	withPending, _ := f.svc.ListForCompetition(ctx, slug, submissions.ListOptions{IncludeUnscored: true})
	if len(withPending) != 4 || withPending[3].Agent != "Dd" || withPending[3].ScoreStatus != "unverifiable" || withPending[3].Rank != nil {
		t.Fatalf("unscored submissions come last, unranked: %+v", withPending)
	}

	byAgent, err := f.svc.ListForAgent(ctx, "bB", submissions.ListOptions{})
	if err != nil || len(byAgent) != 1 || byAgent[0].Agent != "Bb" {
		t.Fatalf("agent lookup ignores case: %+v %v", byAgent, err)
	}
	if none, err := f.svc.ListForAgent(ctx, "Dd", submissions.ListOptions{}); err != nil || len(none) != 0 {
		t.Fatalf("an unscored submission is hidden by default: %+v %v", none, err)
	}
	if _, err := f.svc.ListForAgent(ctx, "Nobody", submissions.ListOptions{}); problem(t, err).Code != "not_found" {
		t.Fatalf("unknown agent: %v", err)
	}
	if _, err := f.svc.ListForCompetition(ctx, "no-such-competition", submissions.ListOptions{}); problem(t, err).Code != "not_found" {
		t.Fatalf("unknown competition: %v", err)
	}
	if _, err := f.svc.Get(ctx, "sub_missing"); problem(t, err).Code != "not_found" {
		t.Fatalf("unknown submission: %v", err)
	}
}

func TestListForOwner_ShowsEverythingOfOneUser(t *testing.T) {
	f := newFixture(t)
	a, owner := f.newAgent(t, "Atlas")
	b, _ := f.newAgent(t, "Nova")
	ctx := context.Background()
	off, _ := f.svc.Submit(ctx, a, f.start(t, a, "official").ID, input())
	prac, _ := f.svc.Submit(ctx, a, f.start(t, a, "practice").ID, input())
	if _, err := f.svc.Submit(ctx, b, f.start(t, b, "official").ID, input()); err != nil {
		t.Fatal(err)
	}
	list, err := f.svc.ListForOwner(ctx, owner)
	if err != nil || len(list) != 2 {
		t.Fatalf("owner list: %+v %v", list, err)
	}
	if list[0].ID != prac.ID || list[1].ID != off.ID || list[0].ScoreStatus != "pending" {
		t.Fatalf("newest first, any kind, any status: %s %s", list[0].ID, list[1].ID)
	}
}

func TestView_LegacyScoresAreEnrichedFromCriteria(t *testing.T) {
	f := newFixture(t)
	a, _ := f.newAgent(t, "Atlas")
	got, err := f.svc.Submit(context.Background(), a, f.start(t, a, "official").ID, input())
	if err != nil {
		t.Fatal(err)
	}
	err = f.d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE submissions SET score_status = 'scored', total = 80,
			scores = '[{"name":"Functionality","score":80,"rationale":"7 of 8"},{"name":"Creativity","score":null,"rationale":"","reason":"judge_disabled"}]' WHERE id = $1`, got.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.Get(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	fn, cr := v.Scores[0], v.Scores[1]
	if fn.Weight != 60 || fn.Source != "checks" || fn.Status != "rated" || fn.Score == nil || *fn.Score != 80 {
		t.Fatalf("functionality entry: %+v", fn)
	}
	if cr.Weight != 10 || cr.Source != "llm" || cr.Status != "not_rated" || cr.Score != nil || cr.Reason != "judge_disabled" {
		t.Fatalf("creativity entry: %+v", cr)
	}
	if !contains(v.Limitations, "llm_judge_disabled") {
		t.Fatalf("limitations must say the judge is off: %v", v.Limitations)
	}
}

func TestView_CommitLinkAndReviewLimitations(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cases := map[string]string{"declared_match": "commit_declared_only", "mismatch": "commit_mismatch"}
	i := 0
	for link, code := range cases {
		i++
		a, _ := f.newAgent(t, "Agent"+strings.Repeat("x", i))
		in := input()
		in.RepoURL, in.CommitSHA = "https://github.com/o/r", strings.Repeat("b", 40)
		got, err := f.svc.Submit(ctx, a, f.start(t, a, "official").ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE submissions SET commit_link = $2 WHERE id = $1`, got.ID, link)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		v, _ := f.svc.Get(ctx, got.ID)
		if !contains(v.Limitations, code) || contains(v.Limitations, "commit_unverified") {
			t.Fatalf("%s: limitations %v", link, v.Limitations)
		}
	}

	a, _ := f.newAgent(t, "Reviewed")
	got, err := f.svc.Submit(ctx, a, f.start(t, a, "official").ID, input())
	if err != nil {
		t.Fatal(err)
	}
	if contains(got.Limitations, "manual_override_applied") {
		t.Fatal("no override yet")
	}
	err = f.d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO check_results (id, check_run_id, check_id, title, requirement, required, weight, status, override_status, override_reason, override_by, override_at)
			VALUES ('res_1', $1, 'mobile', 't', 'R7', true, 15, 'failed', 'passed', 'checker viewport bug', 'user_admin', now())`, got.CheckRun.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.Get(ctx, got.ID)
	if !contains(v.Limitations, "manual_override_applied") {
		t.Fatalf("an override must be reported: %v", v.Limitations)
	}
}
