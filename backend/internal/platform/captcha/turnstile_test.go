package captcha

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTurnstile_Verify_Success(t *testing.T) {
	var gotSecret, gotResponse, gotRemoteIP string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotSecret = r.Form.Get("secret")
		gotResponse = r.Form.Get("response")
		gotRemoteIP = r.Form.Get("remoteip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer srv.Close()

	v := &Turnstile{Secret: "secret", VerifyURL: srv.URL}
	if err := v.Verify(context.Background(), "good-token", "203.0.113.5"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if gotSecret != "secret" {
		t.Fatalf("secret = %q", gotSecret)
	}
	if gotResponse != "good-token" {
		t.Fatalf("response = %q", gotResponse)
	}
	if gotRemoteIP != "203.0.113.5" {
		t.Fatalf("remoteip = %q", gotRemoteIP)
	}
}

func TestTurnstile_Verify_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success": false, "error-codes": ["invalid-input-response"]}`))
	}))
	defer srv.Close()

	v := &Turnstile{Secret: "secret", VerifyURL: srv.URL}
	err := v.Verify(context.Background(), "bad-token", "203.0.113.5")
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("Verify: got %v, want ErrFailed", err)
	}
}

func TestTurnstile_Verify_MissingToken(t *testing.T) {
	v := &Turnstile{Secret: "secret", VerifyURL: "http://unused.invalid"}
	err := v.Verify(context.Background(), "", "203.0.113.5")
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("Verify: got %v, want ErrFailed", err)
	}
}

func TestTurnstile_Verify_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer srv.Close()

	v := &Turnstile{Secret: "secret", VerifyURL: srv.URL, Timeout: 20 * time.Millisecond}
	err := v.Verify(context.Background(), "slow-token", "203.0.113.5")
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("Verify: got %v, want ErrFailed", err)
	}
}

func TestTurnstile_Verify_NetworkError(t *testing.T) {
	// A port nothing listens on: connection refused rather than a timeout.
	v := &Turnstile{Secret: "secret", VerifyURL: "http://127.0.0.1:1"}
	err := v.Verify(context.Background(), "some-token", "")
	if !errors.Is(err, ErrFailed) {
		t.Fatalf("Verify: got %v, want ErrFailed", err)
	}
}
