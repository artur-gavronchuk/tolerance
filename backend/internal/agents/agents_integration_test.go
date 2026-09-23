package agents_test

import (
	"context"
	"testing"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
)

func TestAgentLifecycle_StageFollowsKeysAndPresence(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	users := identity.NewService(d.AppPool, nil)
	u, _, err := users.Signup(ctx, "o@example.com", "longenough1")
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	s := agents.NewService(d.AppPool, agents.NoProofFacts{})

	if o, err := s.Overview(ctx, u.ID); err != nil || o != nil {
		t.Fatalf("expected no agent yet, got %+v %v", o, err)
	}
	p, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "fixer-7", Description: "go backend"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Create(ctx, u.ID, agents.CreateInput{Name: "second"}); err == nil {
		t.Fatalf("second agent must be rejected")
	}
	o, _ := s.Overview(ctx, u.ID)
	if o.Stage != agents.StageRegistered {
		t.Fatalf("no key: want registered, got %s", o.Stage)
	}
	kv, key, err := s.CreateKey(ctx, u.ID, "laptop")
	if err != nil || key == "" {
		t.Fatalf("create key: %v", err)
	}
	o, _ = s.Overview(ctx, u.ID)
	if o.Stage != agents.StageRegistered {
		t.Fatalf("key but no heartbeat: want registered, got %s", o.Stage)
	}
	if err := s.Heartbeat(ctx, p.ID, "0.1.0", "laptop.local"); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	o, _ = s.Overview(ctx, u.ID)
	if o.Stage != agents.StageConnected || o.Presence == nil || o.Presence.Hostname != "laptop.local" {
		t.Fatalf("after heartbeat: %+v", o)
	}
	if err := s.RevokeKey(ctx, u.ID, kv.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	o, _ = s.Overview(ctx, u.ID)
	if o.Stage != agents.StageRegistered {
		t.Fatalf("no active key: want registered, got %s", o.Stage)
	}
}
