package identity_test

import (
	"context"
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
)

func TestResolveUser_UpsertsByIssuerAndSubject(t *testing.T) {
	d := dbtest.New(t)
	service := identity.NewService(d.AppPool)
	ctx := context.Background()
	claims := auth.Claims{Issuer: "https://id.example", Subject: "sub-1", Email: "a@example.com", Name: "Alice"}

	first, err := service.ResolveUser(ctx, claims)
	if err != nil {
		t.Fatalf("resolve user (first): %v", err)
	}
	if first.ID == "" || first.Email != "a@example.com" {
		t.Fatalf("unexpected first resolution: %+v", first)
	}

	second, err := service.ResolveUser(ctx, claims)
	if err != nil {
		t.Fatalf("resolve user (second): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the same (issuer, subject) to resolve to the same user id, got %s and %s", first.ID, second.ID)
	}

	other, err := service.ResolveUser(ctx, auth.Claims{Issuer: "https://id.example", Subject: "sub-2", Email: "b@example.com"})
	if err != nil {
		t.Fatalf("resolve user (other): %v", err)
	}
	if other.ID == first.ID {
		t.Fatalf("expected a different subject to resolve to a different user")
	}
}

func TestCreateOrganizationWithOwner_GrantsActiveOwnerMembership(t *testing.T) {
	d := dbtest.New(t)
	service := identity.NewService(d.AppPool)
	ctx := context.Background()
	user, err := service.ResolveUser(ctx, auth.Claims{Issuer: "https://id.example", Subject: "owner-1"})
	if err != nil {
		t.Fatalf("resolve user: %v", err)
	}

	org, err := service.CreateOrganizationWithOwner(ctx, user.ID, "Acme")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	role, ok, err := service.ActiveMembership(ctx, org.ID, user.ID)
	if err != nil {
		t.Fatalf("check membership: %v", err)
	}
	if !ok || role != "owner" {
		t.Fatalf("expected the creator to have an active owner membership, got ok=%v role=%q", ok, role)
	}
}

func TestActiveMembership_FalseForUnrelatedUserAndOrganization(t *testing.T) {
	d := dbtest.New(t)
	service := identity.NewService(d.AppPool)
	ctx := context.Background()
	owner, _ := service.ResolveUser(ctx, auth.Claims{Issuer: "https://id.example", Subject: "owner-2"})
	org, err := service.CreateOrganizationWithOwner(ctx, owner.ID, "Acme")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	stranger, _ := service.ResolveUser(ctx, auth.Claims{Issuer: "https://id.example", Subject: "stranger-1"})

	_, ok, err := service.ActiveMembership(ctx, org.ID, stranger.ID)
	if err != nil {
		t.Fatalf("check membership: %v", err)
	}
	if ok {
		t.Fatalf("expected a user with no membership to have ok=false")
	}
}

func TestMemberships_ListsAcrossOrganizationsForTheUserOnly(t *testing.T) {
	d := dbtest.New(t)
	service := identity.NewService(d.AppPool)
	ctx := context.Background()
	alice, _ := service.ResolveUser(ctx, auth.Claims{Issuer: "https://id.example", Subject: "alice"})
	bob, _ := service.ResolveUser(ctx, auth.Claims{Issuer: "https://id.example", Subject: "bob"})

	if _, err := service.CreateOrganizationWithOwner(ctx, alice.ID, "Alice's Org A"); err != nil {
		t.Fatalf("create org A: %v", err)
	}
	if _, err := service.CreateOrganizationWithOwner(ctx, alice.ID, "Alice's Org B"); err != nil {
		t.Fatalf("create org B: %v", err)
	}
	if _, err := service.CreateOrganizationWithOwner(ctx, bob.ID, "Bob's Org"); err != nil {
		t.Fatalf("create bob's org: %v", err)
	}

	memberships, err := service.Memberships(ctx, alice.ID)
	if err != nil {
		t.Fatalf("list memberships: %v", err)
	}
	if len(memberships) != 2 {
		t.Fatalf("expected alice to see exactly her 2 organizations, got %d: %+v", len(memberships), memberships)
	}
	for _, m := range memberships {
		if m.Role != "owner" {
			t.Fatalf("expected owner role, got %+v", m)
		}
	}
}
