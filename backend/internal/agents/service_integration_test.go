package agents_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/standings"
)

func problem(t *testing.T, err error) *httpx.Problem {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected problem, got %v", err)
	}
	return p
}

func TestAgents_CreateKeysProfile(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	users := identity.NewService(d.AppPool, nil)
	u, _ := users.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s", Email: "mira@example.com"})
	other, _ := users.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "o", Email: "other@example.com"})
	s := agents.NewService(d.AppPool, standings.NewService(d.AppPool))

	if got, _ := s.PrivateForUser(ctx, u.ID); got != nil {
		t.Fatal("no agent yet")
	}
	if _, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "bad name!", Model: "m"}); problem(t, err).Code != "invalid_body" {
		t.Fatal("name format must be validated")
	}
	p, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "Atlas", Model: "Custom · GPT-based", Bio: "bio"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "Second", Model: "m"}); problem(t, err).Code != "agent_exists" {
		t.Fatal("second agent for the same user must be rejected")
	}
	if _, err := s.Create(ctx, other.ID, agents.CreateInput{Name: "atlas", Model: "m"}); problem(t, err).Code != "name_taken" {
		t.Fatal("name must be unique case-insensitively")
	}

	kv, key, err := s.CreateKey(ctx, u.ID, "laptop")
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
	if err := s.RevokeKey(ctx, other.ID, kv.ID); problem(t, err).Status != 404 {
		t.Fatal("another user must not revoke the key")
	}
	if err := s.RevokeKey(ctx, u.ID, kv.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentIDByKeyHash(ctx, auth.HashAPIKey(key)); !errors.Is(err, identity.ErrNoAgent) {
		t.Fatal("revoked key must not resolve")
	}

	prof, st, err := s.ProfileByName(ctx, "ATLAS")
	if err != nil {
		t.Fatal(err)
	}
	if prof.Agent != "Atlas" || prof.Author != "mira" || st.Rank != nil || len(prof.Badges) != 0 {
		t.Fatalf("unexpected profile %+v %+v", prof, st)
	}
}
