package campaigns_test

import (
	"context"
	"testing"

	"tolerance/internal/campaigns"
	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
)

func setup(t *testing.T) (*dbtest.DB, *identity.Service, *campaigns.Service) {
	t.Helper()
	d := dbtest.New(t)
	return d, identity.NewService(d.AppPool), campaigns.NewService(d.AppPool)
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

func TestCreate_RejectsParticipantRole(t *testing.T) {
	_, is, cs := setup(t)
	ctx := context.Background()
	actor := ownerActor(t, ctx, is, "owner-1")
	actor.Role = "participant"

	_, err := cs.Create(ctx, actor, campaigns.CreateInput{Name: "Season", Mode: "private_trial", Currency: "USD"})
	if err == nil {
		t.Fatalf("expected a participant to be forbidden from creating a campaign")
	}
}

func TestGet_ReturnsNotFoundForAnotherOrganizationsCampaign(t *testing.T) {
	_, is, cs := setup(t)
	ctx := context.Background()
	ownerA := ownerActor(t, ctx, is, "owner-a")
	ownerB := ownerActor(t, ctx, is, "owner-b")

	c, err := cs.Create(ctx, ownerA, campaigns.CreateInput{Name: "Season A", Mode: "private_trial", Currency: "USD"})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	if _, err := cs.Get(ctx, ownerB, c.ID); err == nil {
		t.Fatalf("expected org B to get not_found for org A's campaign")
	}
}

func TestActivate_RejectsTransitionFromNonDraftState(t *testing.T) {
	_, is, cs := setup(t)
	ctx := context.Background()
	owner := ownerActor(t, ctx, is, "owner-1")
	c, err := cs.Create(ctx, owner, campaigns.CreateInput{Name: "Season", Mode: "private_trial", Currency: "USD"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	active, err := cs.Activate(ctx, owner, c.ID, c.Version)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if active.State != campaigns.StateActive {
		t.Fatalf("expected active state, got %s", active.State)
	}

	if _, err := cs.Activate(ctx, owner, c.ID, active.Version); err == nil {
		t.Fatalf("expected activating an already-active campaign to fail")
	}
}

func TestActivate_ConcurrentCallsWithStaleVersionOnlyOneSucceeds(t *testing.T) {
	_, is, cs := setup(t)
	ctx := context.Background()
	owner := ownerActor(t, ctx, is, "owner-1")
	c, err := cs.Create(ctx, owner, campaigns.CreateInput{Name: "Season", Mode: "private_trial", Currency: "USD"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := cs.Activate(ctx, owner, c.ID, c.Version)
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
