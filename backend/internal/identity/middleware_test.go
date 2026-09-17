package identity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
)

type fakeVerifier struct{ err error }

func (f fakeVerifier) Verify(context.Context, string) (auth.Claims, error) {
	return auth.Claims{Issuer: "iss", Subject: "s"}, f.err
}

type fakeLookup struct{ id string }

func (f fakeLookup) AgentIDByKeyHash(context.Context, string) (string, error) {
	if f.id == "" {
		return "", identity.ErrNoAgent
	}
	return f.id, nil
}

func TestRequireUser_RejectsMissingAndAPIKeyTokens(t *testing.T) {
	h := identity.RequireUser(fakeVerifier{}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("must not reach") }))
	for _, hdr := range []string{"", "Bearer ak_" + "0123456789abcdef0123456789abcdef01234567", "Basic xyz"} {
		r := httptest.NewRequest("GET", "/api/v1/me", nil)
		if hdr != "" {
			r.Header.Set("Authorization", hdr)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("header %q: want 401, got %d", hdr, w.Code)
		}
	}
}

func TestRequireAgent_RejectsJWTAndUnknownKey(t *testing.T) {
	h := identity.RequireAgent(fakeLookup{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("must not reach") }))
	for _, hdr := range []string{"Bearer eyJhbGciOiJSUzI1NiJ9.x.y", "Bearer ak_deadbeef"} {
		r := httptest.NewRequest("GET", "/api/v1/agent/me", nil)
		r.Header.Set("Authorization", hdr)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("header %q: want 401, got %d", hdr, w.Code)
		}
	}
}

func TestRequireAgent_AttachesAgentActor(t *testing.T) {
	var got identity.Actor
	h := identity.RequireAgent(fakeLookup{id: "agent_1"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity.MustFromContext(r.Context())
	}))
	r := httptest.NewRequest("GET", "/api/v1/agent/me", nil)
	r.Header.Set("Authorization", "Bearer ak_0123456789abcdef0123456789abcdef01234567")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if got.Kind != identity.KindAgent || got.ID != "agent_1" || got.AgentID != "agent_1" {
		t.Fatalf("unexpected actor %+v", got)
	}
}

func TestRequireAdmin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for role, want := range map[string]int{"user": 403, "admin": 204} {
		r := httptest.NewRequest("GET", "/api/v1/admin/competitions", nil)
		r = r.WithContext(identity.WithActor(r.Context(), identity.Actor{Kind: identity.KindUser, ID: "user_1", UserID: "user_1", Role: role}))
		w := httptest.NewRecorder()
		identity.RequireAdmin(next).ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("role %s: want %d got %d", role, want, w.Code)
		}
	}
}
