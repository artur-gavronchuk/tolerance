package budget_test

import (
	"context"
	"testing"

	"tolerance/internal/budget"
	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
)

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

func TestSetThenGet_RoundTrips(t *testing.T) {
	d := dbtest.New(t)
	is := identity.NewService(d.AppPool)
	bs := budget.NewService(d.AppPool)
	ctx := context.Background()
	owner := ownerActor(t, ctx, is, "owner-1")

	set, err := bs.Set(ctx, owner, budget.SetInput{ScopeKind: "campaign", ScopeID: "camp_1", Currency: "USD", LimitMinor: 100000})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if set.Version != 1 {
		t.Fatalf("expected version 1 on first set, got %d", set.Version)
	}

	got, err := bs.Get(ctx, owner, "campaign", "camp_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LimitMinor != 100000 {
		t.Fatalf("expected limit_minor 100000, got %d", got.LimitMinor)
	}

	updated, err := bs.Set(ctx, owner, budget.SetInput{ScopeKind: "campaign", ScopeID: "camp_1", Currency: "USD", LimitMinor: 50000})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.LimitMinor != 50000 || updated.Version != 2 {
		t.Fatalf("expected the limit to update in place with an incremented version, got %+v", updated)
	}
}

func TestSet_RejectsParticipantRole(t *testing.T) {
	d := dbtest.New(t)
	is := identity.NewService(d.AppPool)
	bs := budget.NewService(d.AppPool)
	ctx := context.Background()
	owner := ownerActor(t, ctx, is, "owner-1")
	participant := owner
	participant.Role = "participant"

	if _, err := bs.Set(ctx, participant, budget.SetInput{ScopeKind: "campaign", ScopeID: "camp_1", Currency: "USD", LimitMinor: 1}); err == nil {
		t.Fatalf("expected a participant to be forbidden from setting a budget")
	}
}

func TestGet_ReturnsNotFoundAcrossOrganizations(t *testing.T) {
	d := dbtest.New(t)
	is := identity.NewService(d.AppPool)
	bs := budget.NewService(d.AppPool)
	ctx := context.Background()
	ownerA := ownerActor(t, ctx, is, "owner-a")
	ownerB := ownerActor(t, ctx, is, "owner-b")

	if _, err := bs.Set(ctx, ownerA, budget.SetInput{ScopeKind: "campaign", ScopeID: "camp_shared_id", Currency: "USD", LimitMinor: 1}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := bs.Get(ctx, ownerB, "campaign", "camp_shared_id"); err == nil {
		t.Fatalf("expected org B to get not_found even for the same scope_id under org A")
	}
}
