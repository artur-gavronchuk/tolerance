package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_ResultRetriesAndTreats409AsDone(t *testing.T) {
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
