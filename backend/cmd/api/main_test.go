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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/routers"
	"github.com/jackc/pgx/v5"

	"tolerance/contracts/openapi"
	"tolerance/internal/admin"
	"tolerance/internal/agents"
	"tolerance/internal/arena"
	"tolerance/internal/challenges"
	"tolerance/internal/games"
	"tolerance/internal/games/match"
	"tolerance/internal/games/tanks"
	"tolerance/internal/identity"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ratelimit"
	"tolerance/internal/proofs"
	"tolerance/internal/proofs/sandbox"
	"tolerance/internal/qualifications"
	"tolerance/internal/skillrating"
	"tolerance/internal/skills"
)

type e2e struct {
	srv    *httptest.Server
	router routers.Router
	worker *proofs.Worker
	fake   *sandbox.Fake
	games  *games.Service
	db     *dbtest.DB
}

func newE2E(t *testing.T, providers ...map[string]identity.Provider) *e2e {
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
	sk, stasks, err := skills.LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if err := skills.SyncCatalog(ctx, d.AdminPool, sk, stasks); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg := config{addr: "127.0.0.1:0", adminEmails: []string{"admin@arena.local"}, sandbox: "fake", devLogin: true, skillMinPool: 3}
	// Effectively unlimited: the e2e test fires many requests back to back
	// from one IP, and the global rate limiter is not what this test is
	// exercising.
	scale := scaleConfig{role: "all", workerConcurrency: 1,
		rateIPRPS: 1e6, rateIPBurst: 1_000_000, rateKeyRPS: 1e6, rateKeyBurst: 1_000_000}
	ps := proofs.NewService(d.AppPool)
	// A real process launcher for house bots (in-process, no interpreter needed) and any uploaded bot
	// (python/js, via python3/node) - short check matches so the qualify-driving tests stay fast. The games
	// worker itself is never started here: tests call ScheduleTick/RunMatch/Qualify directly to arrange
	// their own fixtures deterministically.
	gamesSvc := games.NewService(d.AppPool, ps, match.WithHouse(match.ProcessLauncher{}), log, games.Config{CheckTicks: 200, WorkDir: t.TempDir()})
	qs := qualifications.NewService(d.AppPool, ps)
	// The e2e runs on this repository's practice catalog, which holds three tasks
	// per skill; the production floor (skills.MinPool) is sized for the private
	// rating catalog and would freeze every skill here.
	qs.SetMinPool(3)
	challengesSvc := challenges.NewService(d.AppPool, ps)
	agentsSvc := agents.NewService(d.AppPool, ps)
	agentsSvc.SetVersionListener(qs)
	agentsSvc.SetSkillsSource(qs)
	agentsSvc.SetChallengePlacesSource(challengesSvc)
	dp := deps{pool: d.AppPool, log: log, limiter: ratelimit.New(nil), users: identity.NewService(d.AppPool, cfg.adminEmails),
		agents: agentsSvc, proofs: ps, games: gamesSvc, quals: qs, arena: arena.NewService(d.AppPool, 3), admin: admin.NewService(d.AppPool), challenges: challengesSvc,
		ipLimiter:  ratelimit.NewTokenBuckets(scale.rateIPRPS, scale.rateIPBurst, 100),
		keyLimiter: ratelimit.NewTokenBuckets(scale.rateKeyRPS, scale.rateKeyBurst, 100),
		longPoll:   ratelimit.NewConcurrencyLimiter(2)}
	if len(providers) > 0 {
		dp.providers = providers[0]
		cfg.publicURL = "http://arena.test"
	}
	srv := httptest.NewServer(newHandler(cfg, scale, dp))
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
	// Both hooks, exactly as main.go wires them: a qualification proof advances
	// its run, a challenge proof records its entry's result.
	worker.SetFinishListener(finishBoth{qs, challengesSvc})
	return &e2e{srv: srv, router: router, worker: worker, fake: fake, games: gamesSvc, db: d}
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

// hasFieldPath reports whether fields contains a FieldError for the given path.
func hasFieldPath(fields []httpx.FieldError, path string) bool {
	for _, f := range fields {
		if f.Path == path {
			return true
		}
	}
	return false
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
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// devLogin signs c in through POST /auth/dev.
func (e *e2e) devLogin(t *testing.T, c *http.Client, email string) {
	t.Helper()
	if code := e.call(t, c, "POST", "/api/v1/auth/dev", "", map[string]string{"email": email}, nil); code != 200 {
		t.Fatalf("dev login %s: %d", email, code)
	}
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

// rawGet performs a GET without response validation and returns the body, for leak checks.
func (e *e2e) rawGet(t *testing.T, c *http.Client, path string) string {
	t.Helper()
	resp, err := c.Get(e.srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// noteDiff applies to any task repository: it adds a file no test reads.
const noteDiff = "diff --git a/NOTES.md b/NOTES.md\nnew file mode 100644\n--- /dev/null\n+++ b/NOTES.md\n@@ -0,0 +1 @@\n+agent notes\n"

func TestEndToEnd_SignInConnectProve(t *testing.T) {
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

	// sign in, me
	var me struct {
		User  identity.User `json:"user"`
		Agent *struct {
			agents.Overview
			LastProof *proofs.Proof `json:"last_proof"`
		} `json:"agent"`
	}
	e.devLogin(t, owner, "Owner@Example.com")
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.User.Email != "owner@example.com" || me.Agent != nil {
		t.Fatalf("me after sign in: %+v", me)
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

func TestDevLogin_RateLimited(t *testing.T) {
	e := newE2E(t)
	c := e.browser(t)
	for i := 0; i < 10; i++ {
		e.call(t, c, "POST", "/api/v1/auth/dev", "", map[string]string{"email": "x@example.com"}, nil)
	}
	if code := e.call(t, c, "POST", "/api/v1/auth/dev", "", map[string]string{"email": "x@example.com"}, nil); code != 429 {
		t.Fatalf("11th attempt: %d", code)
	}
}

func TestAuthProviders_ListsNothingWithoutKeysButDevLogin(t *testing.T) {
	e := newE2E(t)
	var out struct {
		Providers []string `json:"providers"`
		DevLogin  bool     `json:"dev_login"`
	}
	if code := e.call(t, &http.Client{}, "GET", "/api/v1/auth/providers", "", nil, &out); code != 200 {
		t.Fatalf("providers: %d", code)
	}
	if len(out.Providers) != 0 || !out.DevLogin {
		t.Fatalf("providers: %+v", out)
	}
}

func TestEndToEnd_OversizedResultFailsTheProof(t *testing.T) {
	e := newE2E(t)
	owner := e.browser(t)
	plain := &http.Client{}
	e.devLogin(t, owner, "big@example.com")
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

func TestEndToEnd_GitHubSignIn(t *testing.T) {
	var challenge string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer"}`))
		case "/user":
			_, _ = w.Write([]byte(`{"id": 777, "login": "octo"}`))
		case "/user/emails":
			_, _ = w.Write([]byte(`[{"email":"Octo@Example.com","primary":true,"verified":true}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(gh.Close)
	e := newE2E(t, map[string]identity.Provider{"github": &identity.GitHub{ClientID: "cid", ClientSecret: "sec",
		AuthURL: gh.URL + "/authorize", TokenURL: gh.URL + "/token", APIURL: gh.URL}})
	owner := e.browser(t)

	var providers struct {
		Providers []string `json:"providers"`
	}
	e.call(t, owner, "GET", "/api/v1/auth/providers", "", nil, &providers)
	if len(providers.Providers) != 1 || providers.Providers[0] != "github" {
		t.Fatalf("providers: %+v", providers)
	}

	// start → the provider, with state and a PKCE challenge
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/auth/github/start?next=/app/agent/connect", nil)
	resp, err := owner.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	openapi.ValidateResponse(t, e.router, req, resp, nil)
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != 302 || !strings.HasPrefix(loc.String(), gh.URL+"/authorize") {
		t.Fatalf("start: %d %s", resp.StatusCode, loc)
	}
	if loc.Query().Get("redirect_uri") != "http://arena.test/api/v1/auth/github/callback" {
		t.Fatalf("redirect_uri: %s", loc.Query().Get("redirect_uri"))
	}
	challenge = loc.Query().Get("code_challenge")

	// the provider sends the browser back with a code
	cb := "/api/v1/auth/github/callback?code=good-code&state=" + url.QueryEscape(loc.Query().Get("state"))
	req, _ = http.NewRequest("GET", e.srv.URL+cb, nil)
	resp, err = owner.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	openapi.ValidateResponse(t, e.router, req, resp, nil)
	if resp.StatusCode != 302 || resp.Header.Get("Location") != "/app/agent/connect" {
		t.Fatalf("callback: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	var me struct {
		User identity.User `json:"user"`
	}
	if code := e.call(t, owner, "GET", "/api/v1/me", "", nil, &me); code != 200 || me.User.Email != "octo@example.com" {
		t.Fatalf("me: %d %+v", code, me)
	}

	// replaying the same callback in the same browser: the state cookie was
	// cleared, so it is single use
	req, _ = http.NewRequest("GET", e.srv.URL+cb, nil)
	resp, _ = owner.Do(req)
	resp.Body.Close()
	if resp.Header.Get("Location") != "/login?error=oauth_state" {
		t.Fatalf("replay: %s", resp.Header.Get("Location"))
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

	// limit clamping and validation: an over-max limit is clamped (never an error), while anything that
	// doesn't parse as a positive integer is 422 validation_failed on fields.limit.
	var clamped struct {
		Items []games.LeaderboardEntry `json:"items"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/tanks/leaderboard?limit=9999", "", nil, &clamped); code != 200 {
		t.Fatalf("leaderboard limit=9999: %d", code)
	}
	if len(clamped.Items) > 500 {
		t.Fatalf("expected leaderboard clamped to at most 500 items, got %d", len(clamped.Items))
	}
	for _, path := range []string{"/api/v1/tanks/leaderboard?limit=0", "/api/v1/tanks/leaderboard?limit=abc"} {
		var problem httpx.Problem
		if code := e.call(t, plain, "GET", path, "", nil, &problem); code != 422 || problem.Code != "validation_failed" {
			t.Fatalf("%s: %d %+v", path, code, problem)
		}
		if !hasFieldPath(problem.Fields, "limit") {
			t.Fatalf("%s: expected fields.limit, got %+v", path, problem.Fields)
		}
	}
	{
		var problem httpx.Problem
		if code := e.call(t, plain, "GET", "/api/v1/tanks/matches?limit=0", "", nil, &problem); code != 422 || problem.Code != "validation_failed" {
			t.Fatalf("matches limit=0: %d %+v", code, problem)
		}
		if !hasFieldPath(problem.Fields, "limit") {
			t.Fatalf("matches limit=0: expected fields.limit, got %+v", problem.Fields)
		}
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

	e.devLogin(t, owner, "rookie@example.com")

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

	e.devLogin(t, owner, "ladder-owner@example.com")
	e.devLogin(t, other, "ladder-other@example.com")

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

	// I-2: the public "recent matches" feed (no bot_id) must also surface it - it's what /tanks shows to a
	// visitor who isn't looking at any one bot.
	var recent struct {
		Items []games.MatchView `json:"items"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/tanks/matches", "", nil, &recent); code != 200 {
		t.Fatalf("recent matches: %d", code)
	}
	found = false
	for _, m := range recent.Items {
		if m.ID == matchID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected match %s among recent matches (no bot_id): %+v", matchID, recent.Items)
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

	e.devLogin(t, owner, "agent-owner@example.com")
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

	e.devLogin(t, owner, "connector-upload@example.com")
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

func TestEndToEnd_Qualification(t *testing.T) {
	e := newE2E(t)
	owner := e.browser(t)
	plain := &http.Client{}
	e.devLogin(t, owner, "q@example.com")
	e.call(t, owner, "POST", "/api/v1/agent", "", map[string]string{"name": "Fixer-7"}, nil)
	var keyResp struct {
		Key string `json:"key"`
	}
	e.call(t, owner, "POST", "/api/v1/agent/keys", "", map[string]string{"name": "k"}, &keyResp)
	hb := map[string]any{"connector_version": "0.2.0", "hostname": "h", "version": map[string]string{"model": "claude-opus-5-5", "harness": "claude-code", "config_digest": "abc"}}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", keyResp.Key, hb, nil); code != 200 {
		t.Fatalf("heartbeat: %d", code)
	}

	// skills are blocked until the basic proof passes
	var sk struct {
		Items []struct {
			Slug          string `json:"slug"`
			CanStart      bool   `json:"can_start"`
			BlockedReason string `json:"blocked_reason"`
		} `json:"items"`
	}
	e.call(t, owner, "GET", "/api/v1/skills", "", nil, &sk)
	if len(sk.Items) != 2 || sk.Items[0].CanStart || sk.Items[0].BlockedReason != "not_operational" {
		t.Fatalf("skills before proof: %+v", sk.Items)
	}
	if code := e.call(t, owner, "POST", "/api/v1/qualifications", "", map[string]string{"skill": "go"}, nil); code != 409 {
		t.Fatalf("qualification before proof: %d", code)
	}

	// basic proof via the same path as slice 1
	var proof proofs.Proof
	e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, &proof)
	if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", keyResp.Key, nil, nil); code != 200 {
		t.Fatalf("claim basic proof: %d", code)
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+proof.ID+"/result", keyResp.Key, map[string]any{"diff": noteDiff}, nil); code >= 300 {
		t.Fatalf("basic result: %d", code)
	}
	if err := e.worker.RunProof(context.Background(), proof.ID); err != nil {
		t.Fatal(err)
	}

	e.call(t, owner, "GET", "/api/v1/skills", "", nil, &sk)
	if !sk.Items[0].CanStart {
		t.Fatalf("skills after proof: %+v", sk.Items)
	}
	var run qualifications.Run
	if code := e.call(t, owner, "POST", "/api/v1/qualifications", "", map[string]string{"skill": "go"}, &run); code != 201 || len(run.Tasks) != 1 {
		t.Fatalf("start: %d %+v", code, run)
	}
	e.call(t, owner, "GET", "/api/v1/skills", "", nil, &sk)
	if sk.Items[0].CanStart || sk.Items[0].BlockedReason != "in_progress" {
		t.Fatalf("skills during a run: %+v", sk.Items)
	}
	csk, ctasks, err := skills.LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := skills.HiddenNamesByTask(csk, ctasks)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		var next struct {
			ProofID string `json:"proof_id"`
			Kind    string `json:"kind"`
			Task    struct {
				Slug string `json:"slug"`
			} `json:"task"`
		}
		if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", keyResp.Key, nil, &next); code != 200 || next.Kind != "qualification" {
			t.Fatalf("task %d: %d %+v", i, code, next)
		}
		if code := e.call(t, owner, "POST", "/api/v1/proofs", "", map[string]string{"task_slug": "go-fix-retry"}, nil); code != 409 {
			t.Fatalf("a proof during a qualification run: %d", code)
		}
		if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+next.ProofID+"/result", keyResp.Key, map[string]any{"diff": noteDiff}, nil); code >= 300 {
			t.Fatalf("result %d: %d", i, code)
		}
		var tests []sandbox.TestResult
		for _, n := range hidden[next.Task.Slug] {
			tests = append(tests, sandbox.TestResult{Name: n, Passed: true})
		}
		e.fake.Result = sandbox.Result{ExitCode: 0, Tests: tests}
		if err := e.worker.RunProof(context.Background(), next.ProofID); err != nil {
			t.Fatal(err)
		}
	}
	e.call(t, owner, "GET", "/api/v1/qualifications/"+run.ID, "", nil, &run)
	if run.Status != "scored" || run.Score == nil || *run.Score != 1 || run.RatingAfter == nil || *run.RatingAfter != 2400 || len(run.Tasks) != 3 {
		t.Fatalf("scored run: %+v", run)
	}
	if run.Tasks[0].SandboxResult == nil || len(run.Tasks[0].SandboxResult.Tests) == 0 || run.Tasks[0].SandboxResult.Tests[0].Name != "hidden-1" || run.Tasks[0].SandboxResult.Output != "" {
		t.Fatalf("hidden tests leaked: %+v", run.Tasks[0].SandboxResult)
	}
	var list struct {
		Items []qualifications.Run `json:"items"`
	}
	if code := e.call(t, owner, "GET", "/api/v1/qualifications", "", nil, &list); code != 200 || len(list.Items) != 1 || list.Items[0].ID != run.ID {
		t.Fatalf("list: %d %+v", code, list.Items)
	}
	if code := e.call(t, owner, "GET", "/api/v1/qualifications/nope", "", nil, nil); code != 404 {
		t.Fatalf("unknown run: %d", code)
	}

	// a qualification proof read through GET /proofs/{id} is masked too
	var pe struct {
		Proof proofs.Proof
	}
	if code := e.call(t, owner, "GET", "/api/v1/proofs/"+run.Tasks[0].ID, "", nil, &pe.Proof); code != 200 {
		t.Fatalf("get qualification proof: %d", code)
	}
	if pe.Proof.SandboxResult == nil || len(pe.Proof.SandboxResult.Tests) == 0 || pe.Proof.SandboxResult.Tests[0].Name != "hidden-1" || pe.Proof.SandboxResult.Output != "" {
		t.Fatalf("GET /proofs/{id} leaked hidden tests: %+v", pe.Proof.SandboxResult)
	}

	// another owner cannot read the run
	other := e.browser(t)
	e.devLogin(t, other, "other@example.com")
	e.call(t, other, "POST", "/api/v1/agent", "", map[string]string{"name": "Other-1"}, nil)
	if code := e.call(t, other, "GET", "/api/v1/qualifications/"+run.ID, "", nil, nil); code != 404 {
		t.Fatalf("another user's run: %d", code)
	}

	var me struct {
		Agent struct {
			LastProof *proofs.Proof             `json:"last_proof"`
			Version   *agents.Version           `json:"version"`
			Skills    []skillrating.SkillRating `json:"skills"`
		} `json:"agent"`
	}
	e.call(t, owner, "GET", "/api/v1/me", "", nil, &me)
	if me.Agent.LastProof == nil || me.Agent.LastProof.ID != proof.ID || me.Agent.Version == nil || len(me.Agent.Skills) != 1 {
		t.Fatalf("/me: last_proof stays the basic proof, version and skills are there: %+v", me.Agent)
	}
	var st struct {
		Agent struct {
			Version *struct {
				Number int `json:"number"`
			} `json:"version"`
			Skills []skillrating.SkillRating `json:"skills"`
		} `json:"agent"`
	}
	e.call(t, plain, "GET", "/api/v1/connector/status", keyResp.Key, nil, &st)
	if st.Agent.Version == nil || st.Agent.Version.Number != 1 || len(st.Agent.Skills) != 1 {
		t.Fatalf("/connector/status: %+v", st.Agent)
	}

	var prof struct {
		Name    string                    `json:"name"`
		Skills  []skillrating.SkillRating `json:"skills"`
		Version *struct {
			Number int `json:"number"`
		} `json:"version"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/agents/fixer-7", "", nil, &prof); code != 200 || prof.Name != "Fixer-7" || len(prof.Skills) != 1 || !prof.Skills[0].Verified || prof.Skills[0].Tier != "strong" || prof.Version == nil || prof.Version.Number != 1 {
		t.Fatalf("public profile: %d %+v", code, prof)
	}
	if code := e.call(t, plain, "GET", "/api/v1/agents/nobody-here", "", nil, nil); code != 404 {
		t.Fatalf("unknown profile: %d", code)
	}
	raw := e.rawGet(t, plain, "/api/v1/agents/fixer-7")
	if strings.Contains(raw, "q@example.com") || strings.Contains(raw, keyResp.Key[:12]) {
		t.Fatalf("profile leaks private data: %s", raw)
	}

	// version change: confidence resets, verified drops until a run on v2
	hb["version"] = map[string]string{"model": "claude-sonnet-5", "harness": "claude-code", "config_digest": "def"}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", keyResp.Key, hb, nil); code != 200 {
		t.Fatalf("heartbeat v2: %d", code)
	}
	e.call(t, plain, "GET", "/api/v1/agents/fixer-7", "", nil, &prof)
	if prof.Version == nil || prof.Version.Number != 2 || len(prof.Skills) != 1 || prof.Skills[0].Verified || prof.Skills[0].OnCurrentVersion || prof.Skills[0].Rating != 2400 {
		t.Fatalf("after version change: %+v", prof)
	}
}

func TestArenaLeaderboardIsPublic(t *testing.T) {
	e := newE2E(t)
	// No session, no API key: a reader reaches the table straight from the landing page.
	plain := &http.Client{}

	var lb struct {
		Items []arena.Row `json:"items"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/leaderboard?skill=go", "", nil, &lb); code != 200 {
		t.Fatalf("leaderboard: %d", code)
	}
	if lb.Items == nil {
		t.Fatal("items must be [] rather than null on an empty arena")
	}
	if code := e.call(t, plain, "GET", "/api/v1/leaderboard", "", nil, nil); code != 422 {
		t.Fatalf("leaderboard without a skill: %d, want 422", code)
	}
	if code := e.call(t, plain, "GET", "/api/v1/leaderboard?skill=go&limit=0", "", nil, nil); code != 422 {
		t.Fatalf("leaderboard with limit=0: %d, want 422", code)
	}
}

func TestAgentPrivacyThroughTheAPI(t *testing.T) {
	e := newE2E(t)
	c := e.browser(t)
	e.devLogin(t, c, "shy@example.com")
	plain := &http.Client{}

	var created struct {
		Name   string `json:"name"`
		Public bool   `json:"public"`
	}
	if code := e.call(t, c, "POST", "/api/v1/agent", "", map[string]any{"name": "shyagent"}, &created); code != 201 {
		t.Fatalf("create agent: %d", code)
	}
	if !created.Public {
		t.Fatal("a new agent is public by default")
	}
	if code := e.call(t, plain, "GET", "/api/v1/agents/shyagent", "", nil, nil); code != 200 {
		t.Fatalf("public profile: %d, want 200", code)
	}

	var patched struct {
		Public bool `json:"public"`
	}
	if code := e.call(t, c, "PATCH", "/api/v1/agent", "", map[string]any{"public": false}, &patched); code != 200 {
		t.Fatalf("patch: %d", code)
	}
	if patched.Public {
		t.Fatal("patch must report public = false")
	}
	// 404, not 403: whether the agent exists is part of what the owner hid.
	if code := e.call(t, plain, "GET", "/api/v1/agents/shyagent", "", nil, nil); code != 404 {
		t.Fatalf("profile after opting out: %d, want 404", code)
	}
}

func TestSkillsReportPoolHealth(t *testing.T) {
	e := newE2E(t)
	c := e.browser(t)
	e.devLogin(t, c, "pool@example.com")
	if code := e.call(t, c, "POST", "/api/v1/agent", "", map[string]any{"name": "poolagent"}, nil); code != 201 {
		t.Fatal("create agent")
	}
	var body struct {
		Items []skills.SkillView `json:"items"`
	}
	if code := e.call(t, c, "GET", "/api/v1/skills", "", nil, &body); code != 200 {
		t.Fatalf("skills: %d", code)
	}
	if len(body.Items) == 0 {
		t.Fatal("the practice catalog must expose at least one skill")
	}
	for _, it := range body.Items {
		// The practice catalog holds three tasks per skill and the e2e floor is
		// three, so nothing is frozen and the pool is fully issuable.
		if it.PoolSize != 3 || it.Frozen {
			t.Fatalf("skill %s: pool_size = %d frozen = %v, want 3 and false", it.Slug, it.PoolSize, it.Frozen)
		}
	}
}

func TestAdminRoutesRefuseANonAdmin(t *testing.T) {
	e := newE2E(t)
	plain := &http.Client{}
	user := e.browser(t)
	e.devLogin(t, user, "ordinary@example.com")
	adminC := e.browser(t)
	e.devLogin(t, adminC, "admin@arena.local") // in cfg.adminEmails

	calls := []struct{ method, path string }{
		{"GET", "/api/v1/admin/skill-tasks?skill=go"},
		{"POST", "/api/v1/admin/skill-tasks/go-lru-cache-eviction/retire"},
		{"POST", "/api/v1/admin/qualifications/qrun_nope/void"},
		{"POST", "/api/v1/admin/agents/agent_nope/ban"},
		{"POST", "/api/v1/admin/agents/agent_nope/unban"},
	}
	body := map[string]any{"reason": "because"}
	for _, c := range calls {
		if code := e.call(t, plain, c.method, c.path, "", body, nil); code != 401 {
			t.Errorf("%s %s without a session: %d, want 401", c.method, c.path, code)
		}
		if code := e.call(t, user, c.method, c.path, "", body, nil); code != 403 {
			t.Errorf("%s %s as an ordinary user: %d, want 403", c.method, c.path, code)
		}
	}
	// No audit event may exist for a call that was refused.
	var audited int
	if err := e.db.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action LIKE 'admin.%'`).Scan(&audited)
	}); err != nil {
		t.Fatal(err)
	}
	if audited != 0 {
		t.Fatalf("%d admin audit events after refused calls, want 0", audited)
	}

	// The same admin call goes through with the admin role, and a blank reason is refused.
	if code := e.call(t, adminC, "GET", "/api/v1/admin/skill-tasks?skill=go", "", nil, nil); code != 200 {
		t.Error("an admin must be able to read task stats")
	}
	if code := e.call(t, adminC, "POST", "/api/v1/admin/skill-tasks/go-lru-cache-eviction/retire", "",
		map[string]any{"reason": "  "}, nil); code != 422 {
		t.Error("a blank reason must be refused")
	}
	if code := e.call(t, adminC, "POST", "/api/v1/admin/skill-tasks/go-lru-cache-eviction/retire", "",
		map[string]any{"reason": "leaked on a forum"}, nil); code != 200 {
		t.Error("an admin must be able to retire a task")
	}
}

func TestEndToEnd_ChallengeLifecycle(t *testing.T) {
	e := newE2E(t)
	adminC := e.browser(t)
	e.devLogin(t, adminC, "admin@arena.local")
	owner := e.browser(t)
	e.devLogin(t, owner, "cup@example.com")
	plain := &http.Client{}

	e.call(t, owner, "POST", "/api/v1/agent", "", map[string]string{"name": "Cupfighter"}, nil)
	var keyResp struct {
		Key string `json:"key"`
	}
	e.call(t, owner, "POST", "/api/v1/agent/keys", "", map[string]string{"name": "k"}, &keyResp)
	hb := map[string]any{"connector_version": "0.2.0", "hostname": "h",
		"version": map[string]string{"model": "claude-opus-5", "harness": "claude-code", "config_digest": "abc"}}
	if code := e.call(t, plain, "POST", "/api/v1/connector/heartbeat", keyResp.Key, hb, nil); code != 200 {
		t.Fatalf("heartbeat: %d", code)
	}

	// Entering a challenge needs the same basic proof a qualification run does.
	// The full connector cycle for one is covered by TestEndToEnd_Qualification;
	// here the row is seeded so this test stays about the challenge path.
	if err := e.db.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO proofs (id, agent_id, kind, task_slug, status, finished_at)
			SELECT 'proof_cup_seed', id, 'proof', 'go-fix-retry', 'passed', now() FROM agents WHERE name = 'Cupfighter'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	body := map[string]any{"slug": "autumn-cup", "title": "Autumn cup", "skill_task_slug": "go-cursor-pagination",
		"min_tier": "none", "opens_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"closes_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "prizes": "bragging rights"}
	if code := e.call(t, adminC, "POST", "/api/v1/admin/challenges", "", body, nil); code != 201 {
		t.Fatalf("create challenge: %d", code)
	}
	// Creating the challenge claimed its task: it must not also be handed out for
	// qualification while agents are competing on it.
	var reserved bool
	if err := e.db.AppPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT challenge_only FROM skill_tasks WHERE slug = 'go-cursor-pagination'`).Scan(&reserved)
	}); err != nil {
		t.Fatal(err)
	}
	if !reserved {
		t.Fatal("the challenge's task is still in the qualification pool")
	}
	// A draft is nobody's business, and an owner cannot enter one.
	if code := e.call(t, plain, "GET", "/api/v1/challenges/autumn-cup", "", nil, nil); code != 404 {
		t.Fatalf("draft challenge is public: %d", code)
	}
	if code := e.call(t, owner, "POST", "/api/v1/challenges/autumn-cup/enter", "", nil, nil); code != 409 {
		t.Fatalf("entering a draft: %d", code)
	}

	if code := e.call(t, adminC, "POST", "/api/v1/admin/challenges/autumn-cup/open", "", nil, nil); code != 200 {
		t.Fatalf("open: %d", code)
	}
	var lists struct {
		Open []struct{ Slug string } `json:"open"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/challenges", "", nil, &lists); code != 200 || len(lists.Open) != 1 {
		t.Fatalf("index: %d %+v", code, lists.Open)
	}

	var entry struct {
		ProofID string `json:"proof_id"`
	}
	if code := e.call(t, owner, "POST", "/api/v1/challenges/autumn-cup/enter", "", map[string]any{"consent_publish": true}, &entry); code != 201 {
		t.Fatalf("enter: %d", code)
	}
	// One attempt: a second entry is refused even before the first finishes.
	if code := e.call(t, owner, "POST", "/api/v1/challenges/autumn-cup/enter", "", nil, nil); code != 409 {
		t.Fatalf("second entry: %d", code)
	}

	var next struct {
		ProofID string `json:"proof_id"`
		Kind    string `json:"kind"`
		Task    struct {
			Slug string `json:"slug"`
		} `json:"task"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/connector/tasks/next?wait=1", keyResp.Key, nil, &next); code != 200 || next.Kind != "challenge" {
		t.Fatalf("claim: %d %+v", code, next)
	}
	if next.ProofID != entry.ProofID || next.Task.Slug != "go-cursor-pagination" {
		t.Fatalf("claimed the wrong task: %+v", next)
	}
	if code := e.call(t, plain, "POST", "/api/v1/connector/proofs/"+next.ProofID+"/result", keyResp.Key,
		map[string]any{"diff": noteDiff}, nil); code >= 300 {
		t.Fatalf("result: %d", code)
	}
	csk, ctasks, err := skills.LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := skills.HiddenNamesByTask(csk, ctasks)
	if err != nil {
		t.Fatal(err)
	}
	var tests []sandbox.TestResult
	for _, n := range hidden["go-cursor-pagination"] {
		tests = append(tests, sandbox.TestResult{Name: n, Passed: true})
	}
	e.fake.Result = sandbox.Result{ExitCode: 0, Tests: tests}
	if err := e.worker.RunProof(context.Background(), next.ProofID); err != nil {
		t.Fatal(err)
	}

	// Still open: the task must not leak to a reader while anyone could enter.
	var view struct {
		Status      string   `json:"status"`
		TaskMD      string   `json:"task_md"`
		HiddenTests []string `json:"hidden_tests"`
		Entrants    int      `json:"entrants"`
		Standings   []struct {
			AgentName string  `json:"agent_name"`
			Rank      int     `json:"rank"`
			Score     float64 `json:"score"`
			Diff      string  `json:"diff"`
		} `json:"standings"`
	}
	e.call(t, plain, "GET", "/api/v1/challenges/autumn-cup", "", nil, &view)
	if view.Status != "open" || view.TaskMD != "" || len(view.Standings) != 0 || view.Entrants != 1 {
		t.Fatalf("open challenge leaks: %+v", view)
	}

	if code := e.call(t, adminC, "POST", "/api/v1/admin/challenges/autumn-cup/close", "", nil, nil); code != 200 {
		t.Fatalf("close: %d", code)
	}
	e.call(t, plain, "GET", "/api/v1/challenges/autumn-cup", "", nil, &view)
	if view.Status != "closed" || len(view.Standings) != 1 || view.Standings[0].Rank != 1 || view.Standings[0].Score != 1 {
		t.Fatalf("closed challenge: %+v", view)
	}
	if view.TaskMD != "" || view.Standings[0].Diff != "" {
		t.Fatal("a closed challenge still hides the task and the diffs")
	}

	if code := e.call(t, adminC, "POST", "/api/v1/admin/challenges/autumn-cup/publish", "", nil, nil); code != 200 {
		t.Fatalf("publish: %d", code)
	}
	e.call(t, plain, "GET", "/api/v1/challenges/autumn-cup", "", nil, &view)
	if view.Status != "published" || view.TaskMD == "" || len(view.HiddenTests) == 0 || view.Standings[0].Diff == "" {
		t.Fatalf("published challenge: %+v", view)
	}

	// The place shows on the agent's public profile, and the rating did not move.
	var profile struct {
		Skills     []any `json:"skills"`
		Challenges []struct {
			Slug string `json:"slug"`
			Rank int    `json:"rank"`
			Of   int    `json:"of"`
		} `json:"challenges"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/agents/Cupfighter", "", nil, &profile); code != 200 {
		t.Fatalf("profile: %d", code)
	}
	if len(profile.Challenges) != 1 || profile.Challenges[0].Slug != "autumn-cup" ||
		profile.Challenges[0].Rank != 1 || profile.Challenges[0].Of != 1 {
		t.Fatalf("profile places: %+v", profile.Challenges)
	}
	if len(profile.Skills) != 0 {
		t.Fatalf("a challenge must not produce a skill rating: %+v", profile.Skills)
	}

	var mine struct {
		Items []struct {
			ChallengeSlug string `json:"challenge_slug"`
			Rank          *int   `json:"rank"`
		} `json:"items"`
	}
	if code := e.call(t, owner, "GET", "/api/v1/me/challenges", "", nil, &mine); code != 200 || len(mine.Items) != 1 {
		t.Fatalf("my entries: %d %+v", code, mine.Items)
	}
	if mine.Items[0].Rank == nil || *mine.Items[0].Rank != 1 {
		t.Fatalf("my entry rank: %+v", mine.Items[0])
	}
}

func TestArenaIsUsableWithoutAnAccount(t *testing.T) {
	e := newE2E(t)
	// The whole point of the arena is a reader who has not signed up. The page
	// draws its tabs from this route; behind a session it would show nothing.
	plain := &http.Client{}
	var skills struct {
		Items []arena.SkillSummary `json:"items"`
	}
	if code := e.call(t, plain, "GET", "/api/v1/arena/skills", "", nil, &skills); code != 200 {
		t.Fatalf("arena skills anonymously: %d, want 200", code)
	}
	if len(skills.Items) != 2 {
		t.Fatalf("items = %+v, want the two practice skills", skills.Items)
	}
	for _, s := range skills.Items {
		if s.Title == "" || s.PoolSize != 3 || s.Frozen {
			t.Fatalf("skill %+v: want a title and three issuable tasks, not frozen", s)
		}
		// And the table for it is readable too, so the page has something to show.
		var lb struct {
			Items []arena.Row `json:"items"`
		}
		if code := e.call(t, plain, "GET", "/api/v1/leaderboard?skill="+s.Slug, "", nil, &lb); code != 200 || lb.Items == nil {
			t.Fatalf("leaderboard for %s: %d %+v", s.Slug, code, lb.Items)
		}
	}
	// The owner-only route still needs a session.
	if code := e.call(t, plain, "GET", "/api/v1/skills", "", nil, nil); code != 401 {
		t.Fatalf("GET /skills anonymously: %d, want 401", code)
	}
}
