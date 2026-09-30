package agents_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
)

type recorder struct{ calls []string }

func (r *recorder) OnNewVersion(_ context.Context, _ pgx.Tx, agentID, versionID string) error {
	r.calls = append(r.calls, versionID)
	return nil
}

func TestEnsureVersion_NewDigestCreatesNumberedVersion(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	us := identity.NewService(d.AppPool, nil)
	u, _, err := us.SignIn(ctx, identity.Identity{Provider: "dev", Subject: "o@example.com", Email: "o@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	s := agents.NewService(d.AppPool, agents.NoProofFacts{})
	rec := &recorder{}
	s.SetVersionListener(rec)
	a, _ := s.Create(ctx, u.ID, agents.CreateInput{Name: "fixer"})

	if v, _ := s.CurrentVersion(ctx, a.ID); v != nil {
		t.Fatalf("no version before the first heartbeat")
	}
	v1, created, err := s.EnsureVersion(ctx, a.ID, agents.VersionInput{Model: "claude-opus-5-5", Harness: "claude-code", ConfigDigest: "d1"})
	if err != nil || !created || v1.Number != 1 {
		t.Fatalf("v1: %v %v %+v", err, created, v1)
	}
	same, created, _ := s.EnsureVersion(ctx, a.ID, agents.VersionInput{Model: "claude-opus-5-5", Harness: "claude-code", ConfigDigest: "d1"})
	if created || same.ID != v1.ID {
		t.Fatalf("same digest must not create a version")
	}
	// Only the model text changed: the connector hashes the whole agent block, so the digest changes too.
	v2, created, _ := s.EnsureVersion(ctx, a.ID, agents.VersionInput{Model: "claude-sonnet-5", Harness: "claude-code", ConfigDigest: "d2"})
	if !created || v2.Number != 2 {
		t.Fatalf("v2: %+v", v2)
	}
	cur, _ := s.CurrentVersion(ctx, a.ID)
	if cur == nil || cur.ID != v2.ID {
		t.Fatalf("current must be v2")
	}
	if len(rec.calls) != 2 || rec.calls[1] != v2.ID {
		t.Fatalf("listener calls: %v", rec.calls)
	}
	o, _ := s.Overview(ctx, u.ID)
	if o.Version == nil || o.Version.Number != 2 || o.Version.Model != "claude-sonnet-5" {
		t.Fatalf("overview version: %+v", o.Version)
	}
	// Back to the first config: no new number, but v1 is current again and
	// the listener hears about it (its ratings are re-proven like any change).
	back, created, _ := s.EnsureVersion(ctx, a.ID, agents.VersionInput{Model: "claude-opus-5-5", Harness: "claude-code", ConfigDigest: "d1"})
	if created || back.ID != v1.ID {
		t.Fatalf("known digest must reuse its version: %+v", back)
	}
	if cur, _ := s.CurrentVersion(ctx, a.ID); cur == nil || cur.ID != v1.ID || len(rec.calls) != 3 {
		t.Fatalf("switching back must make v1 current and notify: %+v %v", cur, rec.calls)
	}
}
