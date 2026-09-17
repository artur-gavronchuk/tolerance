package missions_test

import (
	"context"
	"testing"
	"time"

	"tolerance/internal/campaigns"
	"tolerance/internal/identity"
	"tolerance/internal/missions"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
)

func setup(t *testing.T) (*identity.Service, *campaigns.Service, *missions.Service) {
	t.Helper()
	d := dbtest.New(t)
	is := identity.NewService(d.AppPool)
	cs := campaigns.NewService(d.AppPool)
	return is, cs, missions.NewService(d.AppPool, cs)
}

func ownerActor(t *testing.T, ctx context.Context, is *identity.Service, subject string) identity.Actor {
	t.Helper()
	user, err := is.ResolveUser(ctx, auth.Claims{Issuer: "https://id.example", Subject: subject})
	if err != nil {
		t.Fatalf("resolve user: %v", err)
	}
	org, err := is.CreateOrganizationWithOwner(ctx, user.ID, "Org of "+subject)
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	return identity.Actor{UserID: user.ID, OrganizationID: org.ID, Role: "owner"}
}

func sampleInput() missions.CreateInput {
	return missions.CreateInput{
		Stage: "build", Title: "Незнакомый город", Brief: "Постройте выполнимый маршрут.",
		Deadline: time.Now().Add(7 * 24 * time.Hour),
		Requirements: []missions.RequirementInput{
			{StableKey: "B-G2", Gate: true, Weight: 0, Category: "gate", Text: "Маршрут выполним."},
		},
	}
}

func TestCreate_ProducesADraftWithARequirement(t *testing.T) {
	is, cs, ms := setup(t)
	ctx := context.Background()
	owner := ownerActor(t, ctx, is, "owner-1")
	c, err := cs.Create(ctx, owner, campaigns.CreateInput{Name: "Season", Mode: "private_trial", Currency: "USD"})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	m, err := ms.Create(ctx, owner, c.ID, sampleInput())
	if err != nil {
		t.Fatalf("create mission: %v", err)
	}
	if m.State != missions.StateDraft {
		t.Fatalf("expected draft state, got %s", m.State)
	}
	if m.CurrentVersion == nil || len(m.CurrentVersion.Requirements) != 1 {
		t.Fatalf("expected exactly one requirement, got %+v", m.CurrentVersion)
	}
	if m.CurrentVersion.ContractDigest != nil {
		t.Fatalf("expected an unpublished version to have no contract digest yet")
	}
}

func TestGet_ReturnsNotFoundAcrossOrganizations(t *testing.T) {
	is, cs, ms := setup(t)
	ctx := context.Background()
	ownerA := ownerActor(t, ctx, is, "owner-a")
	ownerB := ownerActor(t, ctx, is, "owner-b")
	c, err := cs.Create(ctx, ownerA, campaigns.CreateInput{Name: "Season", Mode: "private_trial", Currency: "USD"})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	m, err := ms.Create(ctx, ownerA, c.ID, sampleInput())
	if err != nil {
		t.Fatalf("create mission: %v", err)
	}

	if _, err := ms.Get(ctx, ownerB, m.ID); err == nil {
		t.Fatalf("expected org B to get not_found for org A's mission")
	}
}

func TestOpen_RejectsWithoutConfirmedCalibration(t *testing.T) {
	is, cs, ms := setup(t)
	ctx := context.Background()
	owner := ownerActor(t, ctx, is, "owner-1")
	c, err := cs.Create(ctx, owner, campaigns.CreateInput{Name: "Season", Mode: "private_trial", Currency: "USD"})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	m, err := ms.Create(ctx, owner, c.ID, sampleInput())
	if err != nil {
		t.Fatalf("create mission: %v", err)
	}

	if _, err := ms.Open(ctx, owner, m.ID, m.Version); err == nil {
		t.Fatalf("expected opening a draft mission (calibration not confirmed) to fail")
	}
}

func TestConfirmCalibrationThenOpen_FreezesTheContractWithADigest(t *testing.T) {
	is, cs, ms := setup(t)
	ctx := context.Background()
	owner := ownerActor(t, ctx, is, "owner-1")
	c, err := cs.Create(ctx, owner, campaigns.CreateInput{Name: "Season", Mode: "private_trial", Currency: "USD"})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	m, err := ms.Create(ctx, owner, c.ID, sampleInput())
	if err != nil {
		t.Fatalf("create mission: %v", err)
	}

	m, err = ms.ConfirmCalibration(ctx, owner, m.ID)
	if err != nil {
		t.Fatalf("confirm calibration: %v", err)
	}
	if m.State != missions.StateCalibrating {
		t.Fatalf("expected calibrating state, got %s", m.State)
	}

	opened, err := ms.Open(ctx, owner, m.ID, m.Version)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened.State != missions.StateOpen {
		t.Fatalf("expected open state, got %s", opened.State)
	}
	if opened.CurrentVersion.ContractDigest == nil || *opened.CurrentVersion.ContractDigest == "" {
		t.Fatalf("expected a contract digest to be set on open")
	}
	if opened.CurrentVersion.PublishedAt == nil {
		t.Fatalf("expected published_at to be set on open")
	}
	if opened.ActiveVersionID == nil || *opened.ActiveVersionID != opened.CurrentVersion.ID {
		t.Fatalf("expected active_version_id to point at the opened version")
	}
}

func TestOpen_ConcurrentCallsWithStaleVersionOnlyOneSucceeds(t *testing.T) {
	is, cs, ms := setup(t)
	ctx := context.Background()
	owner := ownerActor(t, ctx, is, "owner-1")
	c, err := cs.Create(ctx, owner, campaigns.CreateInput{Name: "Season", Mode: "private_trial", Currency: "USD"})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	m, err := ms.Create(ctx, owner, c.ID, sampleInput())
	if err != nil {
		t.Fatalf("create mission: %v", err)
	}
	m, err = ms.ConfirmCalibration(ctx, owner, m.ID)
	if err != nil {
		t.Fatalf("confirm calibration: %v", err)
	}

	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := ms.Open(ctx, owner, m.ID, m.Version)
			results <- err
		}()
	}
	successes, failures := 0, 0
	for i := 0; i < 2; i++ {
		if err := <-results; err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected exactly one success and one conflict, got successes=%d failures=%d", successes, failures)
	}
}
