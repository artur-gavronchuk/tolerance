package identity_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
)

func TestResolveUser_CreatesDedupesAndPromotesAdmin(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	s := identity.NewService(d.AppPool, []string{"Admin@Arena.local"})

	u1, err := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s1", Email: "mira@example.com", Name: "Mira"})
	if err != nil {
		t.Fatal(err)
	}
	if u1.Handle != "mira" || u1.Role != "user" || u1.DisplayName != "Mira" {
		t.Fatalf("unexpected user %+v", u1)
	}
	again, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s1", Email: "mira@example.com", Name: "Mira K"})
	if again.ID != u1.ID || again.DisplayName != "Mira K" {
		t.Fatalf("same (iss,sub) must return the same user with refreshed name, got %+v", again)
	}
	u2, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s2", Email: "mira@other.com"})
	if u2.Handle != "mira-2" {
		t.Fatalf("handle collision must add a suffix, got %q", u2.Handle)
	}
	admin, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "s3", Email: "admin@arena.local"})
	if admin.Role != "admin" {
		t.Fatalf("email from ARENA_ADMIN_EMAILS (case-insensitive) must get role admin, got %q", admin.Role)
	}
}

func TestResolveUser_AdoptsSeedUserByEmail(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, handle, display_name)
			VALUES ('user_seed', 'seed', 'dev@arena.local', 'dev@arena.local', 'nualimov', 'Dev')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	s := identity.NewService(d.AppPool, nil)
	u, err := s.ResolveUser(ctx, auth.Claims{Issuer: "http://dex", Subject: "abc", Email: "dev@arena.local"})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "user_seed" || u.Handle != "nualimov" {
		t.Fatalf("seed user must be adopted by email, got %+v", u)
	}
}

func TestUpdate_HandleTaken(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	s := identity.NewService(d.AppPool, nil)
	a, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "a", Email: "a1@x.y"})
	b, _ := s.ResolveUser(ctx, auth.Claims{Issuer: "iss", Subject: "b", Email: "b1@x.y"})
	h := a.Handle
	_, err := s.Update(ctx, b.ID, identity.UpdateInput{Handle: &h})
	if p := asProblem(t, err); p.Code != "handle_taken" {
		t.Fatalf("want handle_taken, got %+v", p)
	}
}
