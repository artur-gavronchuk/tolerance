package httpx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tolerance/internal/platform/httpx"
)

func TestWriteError_UnknownErrorBecomesInternalWithoutLeakingItsText(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	httpx.WriteError(w, r, context.DeadlineExceeded)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	var body httpx.Problem
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Code != "internal_error" {
		t.Fatalf("expected internal_error, got %s", body.Code)
	}
	if body.Message == context.DeadlineExceeded.Error() {
		t.Fatalf("internal error must not leak the underlying error text")
	}
}

func TestWriteError_ProblemPassesThroughWithStatusAndRequestID(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler := httpx.WithRequestID(func() string { return "req_test" })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, httpx.NotFound())
	}))
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	var body httpx.Problem
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.RequestID != "req_test" {
		t.Fatalf("expected request id to be attached, got %q", body.RequestID)
	}
}

func TestDecode_RejectsUnknownFieldsAndTrailingData(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}
	var dst input
	if err := httpx.Decode([]byte(`{"name":"a","extra":1}`), &dst); err == nil {
		t.Fatalf("expected unknown field to be rejected")
	}
	if err := httpx.Decode([]byte(`{"name":"a"}{"name":"b"}`), &dst); err == nil {
		t.Fatalf("expected trailing JSON value to be rejected")
	}
	if err := httpx.Decode([]byte(`{"name":"a"}`), &dst); err != nil {
		t.Fatalf("expected valid single object to decode, got %v", err)
	}
	if dst.Name != "a" {
		t.Fatalf("expected decoded value, got %+v", dst)
	}
}
