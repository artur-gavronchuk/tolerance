package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/routers"

	"tolerance/contracts/openapi"
	"tolerance/internal/agents"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
)

type e2e struct {
	srv    *httptest.Server
	router routers.Router
	worker *proofs.Worker
	fake   *sandbox.Fake
}

func newE2E(t *testing.T) *e2e {
	t.Helper()
	d := dbtest.New(t)
	ctx := context.Background()
	tasks, err := proofs.LoadCatalog(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := proofs.SyncCatalog(ctx, d.AdminPool, tasks); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg := config{addr: "127.0.0.1:0", adminEmails: []string{"admin@arena.local"}, sandbox: "fake"}
	ps := proofs.NewService(d.AppPool)
	dp := deps{pool: d.AppPool, log: log, limiter: ratelimit.New(nil), users: identity.NewService(d.AppPool, cfg.adminEmails),
		agents: agents.NewService(d.AppPool, ps), proofs: ps}
	srv := httptest.NewServer(newHandler(cfg, dp))
	t.Cleanup(srv.Close)
	router, err := openapi.Router()
	if err != nil {
		t.Fatal(err)
	}
	// The hidden tests of fixtures/proofs/go-fix-retry: a pass needs all of them.
	var tests []sandbox.TestResult
	for _, n := range []string{"TestHidden_BackoffSequence", "TestHidden_BackoffCapsAtMax", "TestHidden_BackoffZeroAndNegative",
		"TestHidden_DoStopsAtMaxAttempts", "TestHidden_DoReturnsContextErrorWhileWaiting"} {
		tests = append(tests, sandbox.TestResult{Name: n, Passed: true})
	}
	fake := &sandbox.Fake{Result: sandbox.Result{ExitCode: 0, Tests: tests}}
	return &e2e{srv: srv, router: router, worker: proofs.NewWorker(d.AppPool, fake, t.TempDir(), log), fake: fake}
}

func (e *e2e) browser(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

// call performs a request (cookie jar on the client, optional bearer key),
// validates the response against openapi.yaml and decodes into out.
func (e *e2e) call(t *testing.T, c *http.Client, method, path, key string, body any, out any) int {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	openapi.ValidateResponse(t, e.router, req, resp, raw)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode %s %s: %v\n%s", method, path, err, raw)
		}
	}
	return resp.StatusCode
}

func TestEndToEnd_SignupConnectProve(t *testing.T) {
	e := newE2E(t)
	owner := e.browser(t)
	plain := &http.Client{}

	// anonymous
	if code := e.call(t, plain, "GET", "/api/v1/me", "", nil, nil); code != 401 {
		t.Fatalf("anonymous /me: %d", code)
	}

	// signup, me
	var me struct {
		User  identity.User `json:"user"`
		Agent *struct {
			agents.Overview
			LastProof *proofs.Proof `json:"last_proof"`
		} `json:"agent"`
	}
	if code := e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "Owner@Example.com", "password": "longenough1"}, nil); code != 201 {
		t.Fatalf("signup: %d", code)
	}
	if code := e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "owner@example.com", "password": "longenough1"}, nil); code != 409 {
		t.Fatalf("duplicate signup: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.User.Email != "owner@example.com" || me.Agent != nil {
		t.Fatalf("me after signup: %+v", me)
	}

	// agent + key
	var created agents.Private
	if code := e.call(t, owner, "POST", "/api/v1/agent", "", map[string]string{"name": "fixer-7", "description": "go"}, &created); code != 201 {
		t.Fatalf("create agent: %d", code)
	}
	var keyResp struct {
		Key string `json:"key"`
		ID  string `json:"id"`
	}
	if code := e.call(t, owner, "POST", "/api/v1/agent/keys", "", map[string]string{"name": "laptop"}, &keyResp); code != 201 {
		t.Fatalf("create key: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent == nil || me.Agent.Stage != agents.StageRegistered || len(me.Agent.APIKeys) != 1 || me.Agent.LastProof != nil {
		t.Fatalf("me after key: %+v", me.Agent)
	}

	// `arena status` before the connector ever connected: it answers, but it
	// is not a heartbeat, so the agent must not look online afterwards.
	var st struct {
		Agent struct {
			Name  string `json:"name"`
			Stage string `json:"stage"`
		} `json:"agent"`
		LastProof *proofs.Proof `json:"last_proof"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/status", keyResp.Key, nil, &st); code != 200 || st.Agent.Name != "fixer-7" || st.Agent.Stage != agents.StageRegistered || st.LastProof != nil {
		t.Fatalf("status before connect: %d %+v", code, st)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.Stage != agents.StageRegistered {
		t.Fatalf("status must not mark the agent online, stage %s", me.Agent.Stage)
	}

	// proof before the connector is online
	if code := e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, nil); code != 409 {
		t.Fatalf("proof while offline: %d", code)
	}

	// connector
	var hb struct {
		Agent struct{ Stage string } `json:"agent"`
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", keyResp.Key, map[string]string{"connector_version": "0.1.0", "hostname": "laptop"}, &hb); code != 200 || hb.Agent.Stage != agents.StageConnected {
		t.Fatalf("heartbeat: %d %+v", code, hb)
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", "ak_bogus", map[string]string{}, nil); code != 401 {
		t.Fatalf("bogus key: %d", code)
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=0", keyResp.Key, nil, nil); code != 204 {
		t.Fatalf("empty queue: %d", code)
	}

	// proof lifecycle
	var proof proofs.Proof
	if code := e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, &proof); code != 201 || proof.Status != proofs.StatusQueued {
		t.Fatalf("create proof: %d %+v", code, proof)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.Stage != agents.StageChecking || me.Agent.LastProof == nil || me.Agent.LastProof.ID != proof.ID || me.Agent.LastProof.Status != proofs.StatusQueued {
		t.Fatalf("me while queued: %+v", me.Agent)
	}
	var next struct {
		ProofID string      `json:"proof_id"`
		Task    proofs.Task `json:"task"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", keyResp.Key, nil, &next); code != 200 || next.ProofID != proof.ID || next.Task.TaskMD == "" {
		t.Fatalf("next: %d %+v", code, next)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/connector/proofs/"+proof.ID+"/repo.tar.gz", nil)
	req.Header.Set("Authorization", "Bearer "+keyResp.Key)
	resp, err := plain.Do(req)
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/gzip" {
		t.Fatalf("repo: %v %v", err, resp)
	}
	raw, _ := io.ReadAll(resp.Body)
	openapi.ValidateResponse(t, e.router, req, resp, raw)
	resp.Body.Close()
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/started", keyResp.Key, map[string]string{}, nil); code != 204 {
		t.Fatalf("started: %d", code)
	}
	diff := "--- a/retry.go\n+++ b/retry.go\n@@ -19,7 +19,14 @@ func Backoff(attempt int) time.Duration {\n \tif attempt < 1 {\n \t\treturn 0\n \t}\n-\treturn Base * time.Duration(attempt)\n+\tif attempt > 20 {\n+\t\treturn Max\n+\t}\n+\td := Base << uint(attempt-1)\n+\tif d > Max {\n+\t\treturn Max\n+\t}\n+\treturn d\n }\n \n // Do calls fn until it succeeds or maxAttempts is used up.\n"
	res := map[string]any{"diff": diff, "log_tail": "fixed\n", "duration_ms": 4200, "exit_code": 0}
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/result", keyResp.Key, res, nil); code != 204 {
		t.Fatalf("result: %d", code)
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/result", keyResp.Key, res, nil); code != 409 {
		t.Fatalf("duplicate result: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/proofs/"+proof.ID, "", nil, &proof)
	if proof.Status != proofs.StatusDiffSubmitted {
		t.Fatalf("after result: %s", proof.Status)
	}

	// sandbox (fake) via the worker
	if err := e.worker.RunProof(context.Background(), proof.ID); err != nil {
		t.Fatalf("run proof: %v", err)
	}
	e.call(t, owner, "GET", "/api/v1/proofs/"+proof.ID, "", nil, &proof)
	if proof.Status != proofs.StatusPassed || proof.SandboxResult == nil || len(proof.SandboxResult.Tests) != 5 {
		t.Fatalf("after sandbox: %+v", proof)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.Stage != agents.StageOperational || me.Agent.LastProof == nil || me.Agent.LastProof.Status != proofs.StatusPassed || me.Agent.LastProof.Diff != "" {
		t.Fatalf("final me: %+v", me.Agent)
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/status", keyResp.Key, nil, &st); code != 200 || st.Agent.Stage != agents.StageOperational || st.LastProof == nil || st.LastProof.Status != proofs.StatusPassed {
		t.Fatalf("status after pass: %d %+v", code, st)
	}
	var list struct{ Items []proofs.Proof }
	e.call(t, owner, "GET", "/api/v1/proofs", "", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Diff != "" {
		t.Fatalf("list: %+v", list)
	}

	// revoke key → connector dies, owner can no longer start a proof once presence goes stale
	if code := e.call(t, owner, "DELETE", "/api/v1/agent/keys/"+keyResp.ID, "", nil, nil); code != 204 {
		t.Fatalf("revoke: %d", code)
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", keyResp.Key, map[string]string{}, nil); code != 401 {
		t.Fatalf("revoked key heartbeat: %d", code)
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/status", keyResp.Key, nil, nil); code != 401 {
		t.Fatalf("revoked key status: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.Stage != agents.StageRegistered {
		t.Fatalf("stage after revoke: %s", me.Agent.Stage)
	}

	// logout
	if code := e.call(t, owner, "POST", "/api/v1/auth/logout", "", nil, nil); code != 204 {
		t.Fatalf("logout: %d", code)
	}
	if code := e.call(t, owner, "GET", "/api/v1/me", "", nil, nil); code != 401 {
		t.Fatalf("me after logout: %d", code)
	}
}

func TestLogin_RateLimited(t *testing.T) {
	e := newE2E(t)
	c := e.browser(t)
	for i := 0; i < 10; i++ {
		e.call(t, c, "POST", "/api/v1/auth/login", "", map[string]string{"email": "x@example.com", "password": "wrongwrongwrong"}, nil)
	}
	if code := e.call(t, c, "POST", "/api/v1/auth/login", "", map[string]string{"email": "x@example.com", "password": "wrongwrongwrong"}, nil); code != 429 {
		t.Fatalf("11th attempt: %d", code)
	}
}

func TestEndToEnd_OversizedResultFailsTheProof(t *testing.T) {
	e := newE2E(t)
	owner := e.browser(t)
	plain := &http.Client{}
	e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "big@example.com", "password": "longenough1"}, nil)
	e.call(t, owner, "POST", "/api/v1/agent", "", map[string]string{"name": "big-diff"}, nil)
	var key struct {
		Key string `json:"key"`
	}
	e.call(t, owner, "POST", "/api/v1/agent/keys", "", map[string]string{"name": "k"}, &key)
	e.call(t, plain, "POST", "/api/v1/connector/heartbeat", key.Key, map[string]string{"connector_version": "0.1.0", "hostname": "h"}, nil)
	var proof proofs.Proof
	if code := e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, &proof); code != 201 {
		t.Fatalf("create proof: %d", code)
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", key.Key, nil, nil); code != 200 {
		t.Fatalf("next: %d", code)
	}
	// Over the 1 MiB body limit: the server cannot even decode it, and must
	// still end the proof instead of letting it expire.
	res := map[string]any{"diff": strings.Repeat("+x\n", 400_000), "log_tail": "", "duration_ms": 1, "exit_code": 0}
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/result", key.Key, res, nil); code != 413 {
		t.Fatalf("oversized result: %d", code)
	}
	e.call(t, owner, "GET", "/api/v1/proofs/"+proof.ID, "", nil, &proof)
	if proof.Status != proofs.StatusFailed || proof.FailureReason != "diff_too_large" || proof.FinishedAt == nil {
		t.Fatalf("after an oversized result: %+v", proof)
	}
}
