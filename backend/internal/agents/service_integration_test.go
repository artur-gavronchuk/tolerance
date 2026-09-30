package agents_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

func problem(t *testing.T, err error) *httpx.Problem {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected problem, got %v", err)
	}
	return p
}

// createUser inserts a user row directly, bypassing SignIn.
func createUser(t *testing.T, d *dbtest.DB, email string) string {
	t.Helper()
	id := idgen.New("user")
	err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1, $2)`, id, email)
		return err
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func strPtr(s string) *string { return &s }

func TestAgents_CreatePatchKeysRevoke(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	uID := createUser(t, d, "mira@example.com")
	otherID := createUser(t, d, "other@example.com")
	s := agents.NewService(d.AppPool, agents.NoProofFacts{})

	if got, _ := s.Overview(ctx, uID); got != nil {
		t.Fatal("no agent yet")
	}
	if _, err := s.Create(ctx, uID, agents.CreateInput{Name: "bad name!"}); problem(t, err).Code != "invalid_body" {
		t.Fatal("name format must be validated")
	}
	p, err := s.Create(ctx, uID, agents.CreateInput{Name: "Atlas", Description: "desc"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Atlas" || p.Description != "desc" {
		t.Fatalf("unexpected private view %+v", p)
	}
	if _, err := s.Create(ctx, uID, agents.CreateInput{Name: "Second"}); problem(t, err).Code != "agent_exists" {
		t.Fatal("second agent for the same user must be rejected")
	}
	if _, err := s.Create(ctx, otherID, agents.CreateInput{Name: "atlas"}); problem(t, err).Code != "name_taken" {
		t.Fatal("name must be unique case-insensitively")
	}

	kv, key, err := s.CreateKey(ctx, uID, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "ak_") || kv.Prefix != key[:12] {
		t.Fatalf("bad key %q %+v", key, kv)
	}
	id, err := s.AgentIDByKeyHash(ctx, auth.HashAPIKey(key))
	if err != nil || id != p.ID {
		t.Fatalf("key must resolve to the agent: %v %q", err, id)
	}
	if err := s.RevokeKey(ctx, otherID, kv.ID); problem(t, err).Status != 404 {
		t.Fatal("another user must not revoke the key")
	}
	if err := s.RevokeKey(ctx, uID, kv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentIDByKeyHash(ctx, auth.HashAPIKey(key)); !errors.Is(err, identity.ErrNoAgent) {
		t.Fatal("revoked key must not resolve")
	}

	patched, err := s.Patch(ctx, uID, agents.PatchInput{Description: strPtr("new bio")})
	if err != nil {
		t.Fatal(err)
	}
	if patched.Name != "Atlas" || patched.Description != "new bio" {
		t.Fatalf("unexpected patched view %+v", patched)
	}

	thirdID := createUser(t, d, "third@example.com")
	if _, err := s.Create(ctx, thirdID, agents.CreateInput{Name: "Zeta"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Patch(ctx, thirdID, agents.PatchInput{Name: strPtr("atlas")}); problem(t, err).Code != "name_taken" {
		t.Fatal("renaming to a name only distinct by case must be rejected")
	}
}

func boolPtr(b bool) *bool { return &b }

func TestAgents_PrivacyRemovesThePublicProfile(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	s := agents.NewService(d.AppPool, agents.NoProofFacts{})
	owner := createUser(t, d, "quiet@example.com")
	if _, err := s.Create(ctx, owner, agents.CreateInput{Name: "quiet"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Public by default: an owner who does nothing is listed.
	p, err := s.PublicByName(ctx, "quiet")
	if err != nil || p.Name != "quiet" {
		t.Fatalf("default profile = %+v, %v; want a visible profile", p, err)
	}

	priv, err := s.Patch(ctx, owner, agents.PatchInput{Public: boolPtr(false)})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if priv.Public {
		t.Error("the owner's own view must report public = false after opting out")
	}
	_, err = s.PublicByName(ctx, "quiet")
	// 404, not 403: whether an agent exists at all is part of what the owner hid.
	if got := problem(t, err).Status; got != 404 {
		t.Fatalf("profile of an opted-out agent = %d, want 404", got)
	}

	if _, err := s.Patch(ctx, owner, agents.PatchInput{Public: boolPtr(true)}); err != nil {
		t.Fatalf("patch back: %v", err)
	}
	if _, err := s.PublicByName(ctx, "quiet"); err != nil {
		t.Fatalf("opting back in must restore the profile: %v", err)
	}
}

func TestAgents_PatchWithoutPublicLeavesItAlone(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	s := agents.NewService(d.AppPool, agents.NoProofFacts{})
	owner := createUser(t, d, "keep@example.com")
	if _, err := s.Create(ctx, owner, agents.CreateInput{Name: "keeper"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Patch(ctx, owner, agents.PatchInput{Public: boolPtr(false)}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	// A rename must not silently republish an agent the owner hid.
	got, err := s.Patch(ctx, owner, agents.PatchInput{Description: strPtr("still hidden")})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if got.Public {
		t.Fatal("a patch that omits public must leave it false")
	}
}

func TestAgents_BannedAgentHasNoPublicProfile(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	s := agents.NewService(d.AppPool, agents.NoProofFacts{})
	owner := createUser(t, d, "banned@example.com")
	if _, err := s.Create(ctx, owner, agents.CreateInput{Name: "naughty"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE agents SET banned_at = now(), banned_reason = 'test' WHERE name = 'naughty'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublicByName(ctx, "naughty"); problem(t, err).Status != 404 {
		t.Fatal("a banned agent must have no public profile")
	}
}
