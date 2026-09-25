package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/routers"

	"tolerance/contracts/openapi"
	"tolerance/internal/agents"
	"tolerance/internal/games"
	"tolerance/internal/games/match"
	"tolerance/internal/games/tanks"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
)

type e2e struct {
	srv    *httptest.Server
	router routers.Router
	worker *proofs.Worker
	fake   *sandbox.Fake
	games  *games.Service
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
	if err := games.Sync(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg := config{addr: "127.0.0.1:0", adminEmails: []string{"admin@arena.local"}, sandbox: "fake"}
	ps := proofs.NewService(d.AppPool)
	// A real process launcher for house bots (in-process, no interpreter needed) and any uploaded bot
	// (python/js, via python3/node) - short check matches so the qualify-driving tests stay fast. The games
	// worker itself is never started here: tests call ScheduleTick/RunMatch/Qualify directly to arrange
	// their own fixtures deterministically.
	gamesSvc := games.NewService(d.AppPool, ps, match.WithHouse(match.ProcessLauncher{}), log, games.Config{CheckTicks: 200, WorkDir: t.TempDir()})
	dp := deps{pool: d.AppPool, log: log, limiter: ratelimit.New(nil), users: identity.NewService(d.AppPool, cfg.adminEmails),
		agents: agents.NewService(d.AppPool, ps), proofs: ps, games: gamesSvc}
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
	worker := proofs.NewWorker(d.AppPool, fake, t.TempDir(), log)
	worker.SetGameBotJudge(gamesSvc)
	return &e2e{srv: srv, router: router, worker: worker, fake: fake, games: gamesSvc}
}

// requirePython3 skips a test when python3 isn't on PATH, unless ARENA_TEST_REQUIRE_DOCKER=1 (CI always
// has it), in which case a missing interpreter fails the test rather than silently skipping it - the same
// convention internal/games's own tests use.
func requirePython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		if os.Getenv("ARENA_TEST_REQUIRE_DOCKER") == "1" {
			t.Fatalf("python3 not found in PATH: %v", err)
		}
		t.Skipf("python3 not found in PATH: %v", err)
	}
}

// tanksStarterArchive is the unmodified python starter kit, tarred as a bot upload.
func tanksStarterArchive(t *testing.T) []byte {
	t.Helper()
	files, err := tanks.Starter("python")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := proofs.TarFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	return archive
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

	// connector download is public (no API key), but newE2E leaves
	// cfg.connectorDir empty, so this server has no prebuilt binaries: 404
	// connector_unavailable, not 401.
	var problem httpx.Problem
	if code := e.call(t, plain, "GET", "/api/v1/connector/download?os=Darwin&arch=arm64", "", nil, &problem); code != 404 {
		t.Fatalf("connector download without connectorDir: %d", code)
	} else if problem.Code != "connector_unavailable" {
		t.Fatalf("connector download problem code: %q", problem.Code)
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

func TestTanksPublicEmpty(t *testing.T) {
	e := newE2E(t)
	plain := &http.Client{}

	var lb struct {
		Items []games.LeaderboardEntry `json:"items"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/tanks/leaderboard", "", nil, &lb); code != 200 {
		t.Fatalf("leaderboard: %d", code)
	}
	if len(lb.Items) != 2 {
		t.Fatalf("expected exactly hunter and sniper (idle hidden), got %+v", lb.Items)
	}
	names := map[string]bool{}
	for _, entry := range lb.Items {
		names[entry.Name] = true
	}
	if !names["hunter"] || !names["sniper"] || names["idle"] {
		t.Fatalf("unexpected leaderboard names: %+v", names)
	}

	var live games.LiveView
	if code := e.call(t, plain, "GET", "/api/v1/tanks/live", "", nil, &live); code != 200 {
		t.Fatalf("live: %d", code)
	}
	if live.MatchID != nil {
		t.Fatalf("expected no broadcast yet, got %+v", live)
	}

	var problem httpx.Problem
	if code := e.call(t, plain, "GET", "/api/v1/tanks/matches/nope", "", nil, &problem); code != 404 {
		t.Fatalf("unknown match: %d", code)
	}
}

func TestTanksUploadAndQualify(t *testing.T) {
	requirePython3(t)
	e := newE2E(t)
	owner := e.browser(t)

	if code := e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "rookie@example.com", "password": "longenough1"}, nil); code != 201 {
		t.Fatalf("signup: %d", code)
	}

	var bot games.MyBot
	if code := e.call(t, owner, "POST", "/api/v1/me/tanks/bot", "", map[string]string{"name": "rookie"}, &bot); code != 200 {
		t.Fatalf("save bot: %d", code)
	}

	// bad base64
	var problem httpx.Problem
	if code := e.call(t, owner, "POST", "/api/v1/me/tanks/versions", "", map[string]string{"archive_base64": "not-valid-base64!!"}, &problem); code != 422 || problem.Code != "validation_failed" {
		t.Fatalf("bad base64: %d %+v", code, problem)
	}
	fieldOK := false
	for _, f := range problem.Fields {
		if f.Path == "archive_base64" {
			fieldOK = true
		}
	}
	if !fieldOK {
		t.Fatalf("expected fields.archive_base64, got %+v", problem.Fields)
	}

	// junk archive: valid base64, not a valid bot package
	junk := base64.StdEncoding.EncodeToString([]byte("not a tarball"))
	if code := e.call(t, owner, "POST", "/api/v1/me/tanks/versions", "", map[string]string{"archive_base64": junk}, &problem); code != 422 || problem.Code != "invalid_package" {
		t.Fatalf("junk archive: %d %+v", code, problem)
	}

	// a real upload
	b64 := base64.StdEncoding.EncodeToString(tanksStarterArchive(t))
	var v games.VersionView
	if code := e.call(t, owner, "POST", "/api/v1/me/tanks/versions", "", map[string]string{"archive_base64": b64}, &v); code != 201 || v.Status != "pending" {
		t.Fatalf("upload: %d %+v", code, v)
	}

	if _, _, err := e.games.Qualify(context.Background(), v.ID); err != nil {
		t.Fatalf("qualify: %v", err)
	}

	var mine games.MyTanks
	if code := e.call(t, owner, "GET", "/api/v1/me/tanks", "", nil, &mine); code != 200 {
		t.Fatalf("my tanks: %d", code)
	}
	if mine.Bot == nil || mine.Bot.ActiveVersion == nil {
		t.Fatalf("expected an active version, got %+v", mine.Bot)
	}
	if len(mine.Versions) != 1 || len(mine.Versions[0].Checks) != 4 {
		t.Fatalf("expected 4 checks on the one version, got %+v", mine.Versions)
	}

	var profile games.BotProfile
	if code := e.call(t, owner, "GET", "/api/v1/tanks/bots/"+bot.ID, "", nil, &profile); code != 200 || profile.Name != "rookie" || len(profile.Versions) != 1 {
		t.Fatalf("bot profile: %d %+v", code, profile)
	}
}

func TestTanksLadderPublic(t *testing.T) {
	requirePython3(t)
	e := newE2E(t)
	ctx := context.Background()
	owner := e.browser(t)
	other := e.browser(t)
	plain := &http.Client{}

	if code := e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "ladder-owner@example.com", "password": "longenough1"}, nil); code != 201 {
		t.Fatalf("signup owner: %d", code)
	}
	if code := e.call(t, other, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "ladder-other@example.com", "password": "longenough1"}, nil); code != 201 {
		t.Fatalf("signup other: %d", code)
	}

	var bot games.MyBot
	if code := e.call(t, owner, "POST", "/api/v1/me/tanks/bot", "", map[string]string{"name": "lead"}, &bot); code != 200 {
		t.Fatalf("save bot: %d", code)
	}
	b64 := base64.StdEncoding.EncodeToString(tanksStarterArchive(t))
	var v games.VersionView
	if code := e.call(t, owner, "POST", "/api/v1/me/tanks/versions", "", map[string]string{"archive_base64": b64}, &v); code != 201 {
		t.Fatalf("upload: %d", code)
	}
	if _, _, err := e.games.Qualify(ctx, v.ID); err != nil {
		t.Fatalf("qualify: %v", err)
	}

	matchID, err := e.games.ScheduleTick(ctx, 1, 0)
	if err != nil {
		t.Fatalf("schedule tick: %v", err)
	}
	if matchID == "" {
		t.Fatal("expected a scheduled ladder match")
	}
	if err := e.games.RunMatch(ctx, matchID); err != nil {
		t.Fatalf("run match: %v", err)
	}

	var matches struct {
		Items []games.MatchView `json:"items"`
	}
	if code := e.call(t, owner, "GET", "/api/v1/tanks/matches?bot_id="+bot.ID, "", nil, &matches); code != 200 {
		t.Fatalf("matches: %d", code)
	}
	found := false
	for _, m := range matches.Items {
		if m.ID == matchID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected match %s in %+v", matchID, matches.Items)
	}

	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/tanks/matches/"+matchID+"/replay", nil)
	resp, err := plain.Do(req)
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/gzip" {
		t.Fatalf("replay: %v %v", err, resp)
	}
	raw, _ := io.ReadAll(resp.Body)
	openapi.ValidateResponse(t, e.router, req, resp, raw)
	resp.Body.Close()
	if _, err := tanks.DecodeReplay(raw); err != nil {
		t.Fatalf("decode replay: %v", err)
	}

	var log games.MatchLog
	if code := e.call(t, owner, "GET", "/api/v1/me/tanks/matches/"+matchID+"/log", "", nil, &log); code != 200 || log.MatchID != matchID {
		t.Fatalf("own match log: %d %+v", code, log)
	}
	if code := e.call(t, other, "GET", "/api/v1/me/tanks/matches/"+matchID+"/log", "", nil, nil); code != 404 {
		t.Fatalf("another owner's match log: %d", code)
	}
}

func TestTanksAgentRun(t *testing.T) {
	e := newE2E(t)
	owner := e.browser(t)
	plain := &http.Client{}

	if code := e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "agent-owner@example.com", "password": "longenough1"}, nil); code != 201 {
		t.Fatalf("signup: %d", code)
	}
	if code := e.call(t, owner, "POST", "/api/v1/agent", "", map[string]string{"name": "runner"}, nil); code != 201 {
		t.Fatalf("create agent: %d", code)
	}
	var key struct {
		Key string `json:"key"`
	}
	if code := e.call(t, owner, "POST", "/api/v1/agent/keys", "", map[string]string{"name": "k"}, &key); code != 201 {
		t.Fatalf("create key: %d", code)
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", key.Key, map[string]string{"connector_version": "0.1.0", "hostname": "h"}, nil); code != 200 {
		t.Fatalf("heartbeat: %d", code)
	}

	var proof proofs.Proof
	if code := e.call(t, owner, "POST", "/api/v1/me/tanks/agent-runs", "", nil, &proof); code != 201 || proof.Kind != proofs.KindGameBot {
		t.Fatalf("agent run: %d %+v", code, proof)
	}

	var next struct {
		ProofID string      `json:"proof_id"`
		Kind    string      `json:"kind"`
		Task    proofs.Task `json:"task"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", key.Key, nil, &next); code != 200 || next.Kind != proofs.KindGameBot || next.ProofID != proof.ID {
		t.Fatalf("next: %d %+v", code, next)
	}

	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/connector/proofs/"+proof.ID+"/repo.tar.gz", nil)
	req.Header.Set("Authorization", "Bearer "+key.Key)
	resp, err := plain.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("repo download: %v %v", err, resp)
	}
	raw, _ := io.ReadAll(resp.Body)
	openapi.ValidateResponse(t, e.router, req, resp, raw)
	resp.Body.Close()
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != next.Task.RepoSHA256 {
		t.Fatalf("repo sha256 mismatch: got %x want %s", sum, next.Task.RepoSHA256)
	}
}

func TestTanksConnectorUpload(t *testing.T) {
	e := newE2E(t)
	owner := e.browser(t)
	plain := &http.Client{}

	if code := e.call(t, owner, "POST", "/api/v1/auth/signup", "", map[string]string{"email": "connector-upload@example.com", "password": "longenough1"}, nil); code != 201 {
		t.Fatalf("signup: %d", code)
	}
	if code := e.call(t, owner, "POST", "/api/v1/agent", "", map[string]string{"name": "uploader"}, nil); code != 201 {
		t.Fatalf("create agent: %d", code)
	}
	var key struct {
		Key string `json:"key"`
	}
	if code := e.call(t, owner, "POST", "/api/v1/agent/keys", "", map[string]string{"name": "k"}, &key); code != 201 {
		t.Fatalf("create key: %d", code)
	}

	b64 := base64.StdEncoding.EncodeToString(tanksStarterArchive(t))
	var v games.VersionView
	if code := e.call(t, plain, "POST", "/api/v1/connector/tanks/versions", key.Key, map[string]string{"archive_base64": b64}, &v); code != 201 || v.Source != "upload" {
		t.Fatalf("connector upload: %d %+v", code, v)
	}
}
