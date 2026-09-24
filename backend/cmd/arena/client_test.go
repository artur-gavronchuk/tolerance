package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_ResultRetriesAndTreats409AsDone(t *testing.T) {
	shortRetries(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ak_test" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/api/v1/connector/proofs/p1/result":
			n := atomic.AddInt32(&calls, 1)
			if n == 1 {
				hj, _ := w.(http.Hijacker)
				conn, _, _ := hj.Hijack()
				conn.Close() // simulate a dropped connection
				return
			}
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "state_conflict", "message": "already"})
		case "/api/v1/connector/tasks/next":
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Result(ctx, "p1", result{Diff: "x"}); err != nil {
		t.Fatalf("expected success after retry + 409, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
	nt, err := c.NextTask(ctx, time.Second)
	if err != nil || nt != nil {
		t.Fatalf("204 must be nil, nil: %v %+v", err, nt)
	}
}

func shortRetries(t *testing.T) {
	t.Helper()
	retryBase = 10 * time.Millisecond
	t.Cleanup(func() { retryBase = 2 * time.Second })
}

func TestClient_RepoRetriesDroppedConnectionsAndServerErrors(t *testing.T) {
	shortRetries(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch atomic.AddInt32(&calls, 1) {
		case 1: // the connection drops halfway through a 200 body
			conn, buf, _ := w.(http.Hijacker).Hijack()
			buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\npartial")
			buf.Flush()
			conn.Close()
		case 2: // the server has a bad moment
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = w.Write([]byte("tarball"))
		}
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	got, err := c.Repo(context.Background(), "p1")
	if err != nil || string(got) != "tarball" || atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("repo: err=%v body=%q calls=%d", err, got, calls)
	}
}

func TestClient_RepoGivesUpOnClientErrors(t *testing.T) {
	shortRetries(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "gone"})
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	_, err := c.Repo(context.Background(), "p1")
	var ae *apiError
	if !errors.As(err, &ae) || ae.Status != 404 || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("a 404 must end the download at once: err=%v calls=%d", err, calls)
	}
}

func TestClient_ResultRetriesServerErrorsUntilTheDeadline(t *testing.T) {
	shortRetries(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := c.Result(ctx, "p1", result{}); err == nil || atomic.LoadInt32(&calls) < 2 {
		t.Fatalf("expected retries and then an error at the deadline: err=%v calls=%d", err, calls)
	}
}

func TestClient_ResultTooLargeIsFinal(t *testing.T) {
	shortRetries(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "diff_too_large", "message": "Diff exceeds 256 KiB"})
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	err := c.Result(context.Background(), "p1", result{})
	var ae *apiError
	if !errors.As(err, &ae) || ae.Code != "diff_too_large" || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("a 413 must not be retried: err=%v calls=%d", err, calls)
	}
}

func TestClient_StatusDoesNotHeartbeat(t *testing.T) {
	var heartbeats int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/connector/heartbeat":
			atomic.AddInt32(&heartbeats, 1)
		case "/api/v1/connector/status":
			_, _ = w.Write([]byte(`{"agent":{"id":"a1","name":"fixer","stage":"check_failed"},
				"last_proof":{"task_slug":"go-fix-retry","status":"failed","failure_reason":"tests_failed","created_at":"2026-09-25T10:00:00Z"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := &client{base: srv.URL, key: "ak_test", http: srv.Client()}
	st, err := c.Status(context.Background())
	if err != nil || atomic.LoadInt32(&heartbeats) != 0 {
		t.Fatalf("status: err=%v heartbeats=%d", err, heartbeats)
	}
	now := time.Date(2026, 9, 25, 10, 3, 0, 0, time.UTC)
	if got, want := formatStatus(st, now), "fixer: check_failed\nlast proof: go-fix-retry failed (tests_failed), started 3m0s ago\n"; got != want {
		t.Fatalf("formatStatus:\n got %q\nwant %q", got, want)
	}
	st.LastProof = nil
	if got, want := formatStatus(st, now), "fixer: check_failed\nlast proof: none yet\n"; got != want {
		t.Fatalf("formatStatus without proofs:\n got %q\nwant %q", got, want)
	}
}
