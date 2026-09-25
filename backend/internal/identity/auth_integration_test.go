package identity_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
)

func gh(subject, email string) identity.Identity {
	return identity.Identity{Provider: "github", Subject: subject, Email: email, EmailVerified: true, Login: "octo"}
}

func TestSignIn_CreatesThenReturnsTheSameUser(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, []string{"Admin@Arena.local"})
	ctx := context.Background()

	u, tok, err := s.SignIn(ctx, gh("42", " User@Example.com "))
	if err != nil {
		t.Fatalf("first sign-in: %v", err)
	}
	if u.Email != "user@example.com" || u.Role != "user" || tok == "" {
		t.Fatalf("unexpected user %+v", u)
	}
	got, err := s.UserBySession(ctx, tok)
	if err != nil || got.ID != u.ID {
		t.Fatalf("session lookup: %v %+v", err, got)
	}
	again, tok2, err := s.SignIn(ctx, gh("42", "renamed@example.com"))
	if err != nil || again.ID != u.ID {
		t.Fatalf("returning sign-in must find the same user: %v %+v", err, again)
	}
	if again.Email != "user@example.com" {
		t.Fatalf("users.email keeps the first address, got %q", again.Email)
	}
	if err := s.Logout(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, tok); !errors.Is(err, identity.ErrNoSession) {
		t.Fatalf("logged-out session must be gone, got %v", err)
	}
	if _, err := s.UserBySession(ctx, tok2); err != nil {
		t.Fatalf("other session must survive: %v", err)
	}
}

func TestSignIn_LinksASecondProviderByVerifiedEmail(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, nil)
	ctx := context.Background()
	u, _, err := s.SignIn(ctx, gh("42", "same@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := s.SignIn(ctx, identity.Identity{Provider: "google", Subject: "g-1", Email: "Same@Example.com", EmailVerified: true})
	if err != nil || g.ID != u.ID {
		t.Fatalf("google with the same verified email must link: %v %+v", err, g)
	}
	var n int
	if err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE user_id = $1`, u.ID).Scan(&n)
	}); err != nil || n != 2 {
		t.Fatalf("want 2 identities, got %d %v", n, err)
	}
}

func TestSignIn_RefusesANewIdentityWithoutAVerifiedEmail(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, nil)
	ctx := context.Background()
	_, _, err := s.SignIn(ctx, identity.Identity{Provider: "github", Subject: "7", Email: "x@example.com", EmailVerified: false})
	if !errors.Is(err, identity.ErrEmailUnverified) {
		t.Fatalf("want ErrEmailUnverified, got %v", err)
	}
	_, _, err = s.SignIn(ctx, identity.Identity{Provider: "github", Subject: "8", Email: "", EmailVerified: true})
	if !errors.Is(err, identity.ErrEmailUnverified) {
		t.Fatalf("empty email must be refused too, got %v", err)
	}
	var n int
	_ = d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("no user may be created, got %d", n)
	}
}

func TestSignIn_AdminRoleFollowsConfig(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, []string{"Admin@Arena.local"})
	u, _, err := s.SignIn(context.Background(), gh("1", "admin@arena.local"))
	if err != nil || u.Role != "admin" {
		t.Fatalf("admin email must get admin role: %v %+v", err, u)
	}
}

func TestSignIn_ConcurrentFirstSignInsMakeOneUser(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, nil)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make([]string, 4)
	errs := make([]error, 4)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, _, err := s.SignIn(ctx, gh("99", "race@example.com"))
			ids[i], errs[i] = u.ID, err
		}()
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Fatalf("attempt %d: %v id=%s want %s", i, errs[i], ids[i], ids[0])
		}
	}
	var users, idents int
	_ = d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM user_identities`).Scan(&idents)
	})
	if users != 1 || idents != 1 {
		t.Fatalf("want 1 user and 1 identity, got %d and %d", users, idents)
	}
}
