package idempotency_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/idempotency"
)

func TestCommand_ReplaysAndConflicts(t *testing.T) {
	d := dbtest.New(t)
	calls := 0
	h := idempotency.Command(d.AppPool, "POST /test", func(r *http.Request, actor identity.Actor, raw []byte) (any, int, error) {
		calls++
		return map[string]any{"n": calls, "actor": actor.ID}, http.StatusCreated, nil
	})
	do := func(actorID, key, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
		r.Header.Set("Idempotency-Key", key)
		r = r.WithContext(identity.WithActor(r.Context(), identity.Actor{Kind: identity.KindAgent, ID: actorID}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	first := do("agent_1", "key-00000001", `{"a":1}`)
	second := do("agent_1", "key-00000001", `{"a":1}`)
	if first.Code != 201 || second.Code != 201 || calls != 1 {
		t.Fatalf("replay must not rerun the mutation: %d %d calls=%d", first.Code, second.Code, calls)
	}
	var a, b map[string]any
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if a["n"] != b["n"] {
		t.Fatal("replayed body must equal the original")
	}
	if w := do("agent_1", "key-00000001", `{"a":2}`); w.Code != 409 {
		t.Fatalf("same key, different body must be 409, got %d", w.Code)
	}
	if w := do("agent_2", "key-00000001", `{"a":1}`); w.Code != 201 || calls != 2 {
		t.Fatalf("keys are scoped per actor: %d calls=%d", w.Code, calls)
	}
	if w := do("agent_2", "short", `{}`); w.Code != 422 {
		t.Fatalf("short key must be 422, got %d", w.Code)
	}
}
