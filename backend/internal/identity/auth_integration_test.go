package identity_test

import (
	"context"
	"errors"
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
)

func TestSignupLoginLogout(t *testing.T) {
	d := dbtest.New(t)
	s := identity.NewService(d.AppPool, []string{"Admin@Arena.local"})
	ctx := context.Background()

	u, tok, err := s.Signup(ctx, " User@Example.com ", "longenough1")
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if u.Email != "user@example.com" || u.Role != "user" {
		t.Fatalf("unexpected user %+v", u)
	}
	got, err := s.UserBySession(ctx, tok)
	if err != nil || got.ID != u.ID {
		t.Fatalf("session lookup: %v %+v", err, got)
	}

	_, _, err = s.Signup(ctx, "user@example.com", "longenough1")
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Status != 409 || p.Code != "email_taken" {
		t.Fatalf("expected 409 email_taken, got %v", err)
	}
	_, _, err = s.Signup(ctx, "x@example.com", "short")
	if !errors.As(err, &p) || p.Status != 422 {
		t.Fatalf("expected 422 for short password, got %v", err)
	}

	_, _, err = s.Login(ctx, "user@example.com", "wrong-password")
	if !errors.As(err, &p) || p.Status != 401 || p.Code != "invalid_credentials" {
		t.Fatalf("expected 401 invalid_credentials, got %v", err)
	}
	_, _, err = s.Login(ctx, "nobody@example.com", "longenough1")
	if !errors.As(err, &p) || p.Status != 401 || p.Code != "invalid_credentials" {
		t.Fatalf("unknown email must look exactly like a wrong password, got %v", err)
	}
	_, tok2, err := s.Login(ctx, "USER@example.com", "longenough1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := s.Logout(ctx, tok); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := s.UserBySession(ctx, tok); !errors.Is(err, identity.ErrNoSession) {
		t.Fatalf("logged-out session must be gone, got %v", err)
	}
	if _, err := s.UserBySession(ctx, tok2); err != nil {
		t.Fatalf("other session must survive: %v", err)
	}

	admin, _, err := s.Signup(ctx, "admin@arena.local", "longenough1")
	if err != nil || admin.Role != "admin" {
		t.Fatalf("admin email must get admin role: %v %+v", err, admin)
	}
}
