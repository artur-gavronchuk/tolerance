package proofs_test

import (
	"context"
	"path/filepath"
	"testing"

	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/proofs"
)

type fixture struct {
	d      *dbtest.DB
	users  *identity.Service
	agents *agents.Service
	proofs *proofs.Service
	userID string
	agent  string
}

func setup(t *testing.T) fixture {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	tasks, err := proofs.LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := proofs.SyncCatalog(ctx, d.AdminPool, tasks); err != nil {
		t.Fatal(err)
	}
	ps := proofs.NewService(d.AppPool)
	as := agents.NewService(d.AppPool, ps)
	us := identity.NewService(d.AppPool, nil)
	u, _, err := us.SignIn(ctx, identity.Identity{Provider: "dev", Subject: "o@example.com", Email: "o@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	a, err := as.Create(ctx, u.ID, agents.CreateInput{Name: "fixer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := as.CreateKey(ctx, u.ID, "k"); err != nil {
		t.Fatal(err)
	}
	return fixture{d: d, users: us, agents: as, proofs: ps, userID: u.ID, agent: a.ID}
}
