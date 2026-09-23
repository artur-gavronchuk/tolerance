package identity_test

import (
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
)

func TestDeriveHandle(t *testing.T) {
	cases := []struct {
		name   string
		claims auth.Claims
		want   string
	}{
		{"preferred_username wins", auth.Claims{PreferredUsername: "Mira_K", Email: "x@y.z"}, "mira-k"},
		{"email local part", auth.Claims{Email: "Dmitri.Ivanov@example.com"}, "dmitri-ivanov"},
		{"strips invalid chars", auth.Claims{Email: "a+b!!c@example.com"}, "a-b-c"},
		{"too short pads", auth.Claims{Email: "a@example.com"}, "a-user"},
		{"truncates to 32", auth.Claims{PreferredUsername: "abcdefghijklmnopqrstuvwxyz0123456789"}, "abcdefghijklmnopqrstuvwxyz012345"},
		{"fallback", auth.Claims{Subject: "sub-1"}, "user-"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := identity.DeriveHandle(c.claims)
			if c.name == "fallback" {
				if len(got) != len("user-")+6 || got[:5] != "user-" {
					t.Fatalf("fallback must be user-<6 hex>, got %q", got)
				}
				return
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
			if !identity.ValidHandle(got) {
				t.Fatalf("%q must be a valid handle", got)
			}
		})
	}
}

func TestValidHandle(t *testing.T) {
	for h, ok := range map[string]bool{"mira": true, "a1-b2": true, "-bad": false, "UPPER": false, "x": false, "has space": false} {
		if identity.ValidHandle(h) != ok {
			t.Fatalf("ValidHandle(%q) = %v, want %v", h, !ok, ok)
		}
	}
}
