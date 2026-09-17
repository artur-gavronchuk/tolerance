package idempotency_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/idempotency"
)

func setup(t *testing.T) (*dbtest.DB, identity.Actor) {
	t.Helper()
	d := dbtest.New(t)
	is := identity.NewService(d.AppPool)
	ctx := context.Background()
	user, err := is.ResolveUser(ctx, auth.Claims{Issuer: "https://id.example", Subject: "owner-1"})
	if err != nil {
		t.Fatalf("resolve user: %v", err)
	}
	org, err := is.CreateOrganizationWithOwner(ctx, user.ID, "Acme")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	return d, identity.Actor{UserID: user.ID, OrganizationID: org.ID, Role: "owner"}
}

func TestCommand_SameKeyAndBodyReplaysWithoutRunningTheMutationAgain(t *testing.T) {
	d, actor := setup(t)
	calls := 0
	handler := idempotency.Command(d.AppPool, "POST /widgets", func(r *http.Request, a identity.Actor, raw []byte) (any, int, error) {
		calls++
		return map[string]int{"calls": calls}, http.StatusCreated, nil
	})

	do := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/widgets", nil)
		r = r.WithContext(identity.WithActor(context.Background(), actor))
		r.Header.Set("Idempotency-Key", "same-key-0123456789")
		handler.ServeHTTP(w, r)
		return w
	}

	first := do()
	second := do()

	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("expected both responses to be 201, got %d and %d", first.Code, second.Code)
	}
	// The replay round-trips through a jsonb column, which reformats
	// whitespace (Postgres's own normalization), so the two bodies are
	// compared as JSON values, not as byte strings.
	var firstValue, secondValue map[string]int
	if err := json.Unmarshal(first.Body.Bytes(), &firstValue); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondValue); err != nil {
		t.Fatalf("decode second (replayed) response: %v", err)
	}
	if firstValue["calls"] != secondValue["calls"] {
		t.Fatalf("expected the replayed response to carry the same value, got %v and %v", firstValue, secondValue)
	}
	if calls != 1 {
		t.Fatalf("expected the mutation to run exactly once, ran %d times", calls)
	}
}

func TestCommand_SameKeyDifferentBodyIsAConflict(t *testing.T) {
	d, actor := setup(t)
	handler := idempotency.Command(d.AppPool, "POST /widgets", func(r *http.Request, a identity.Actor, raw []byte) (any, int, error) {
		return map[string]string{"body": string(raw)}, http.StatusCreated, nil
	})

	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/widgets", strings.NewReader(body))
		r = r.WithContext(identity.WithActor(context.Background(), actor))
		r.Header.Set("Idempotency-Key", "same-key-0123456789")
		handler.ServeHTTP(w, r)
		return w
	}

	if w := post("a"); w.Code != http.StatusCreated {
		t.Fatalf("expected the first request to succeed, got %d: %s", w.Code, w.Body.String())
	}
	w := post("b")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected reusing the key with a different body to be a conflict, got %d", w.Code)
	}
}

func TestCommand_RejectsAKeyThatIsTooShort(t *testing.T) {
	d, actor := setup(t)
	handler := idempotency.Command(d.AppPool, "POST /widgets", func(r *http.Request, a identity.Actor, raw []byte) (any, int, error) {
		return nil, http.StatusCreated, nil
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/widgets", nil)
	r = r.WithContext(identity.WithActor(context.Background(), actor))
	r.Header.Set("Idempotency-Key", "short")
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for a too-short key, got %d", w.Code)
	}
}
