package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/routers"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"tolerance/contracts/openapi"
	"tolerance/fixtures/seed"
	"tolerance/internal/agents"
	"tolerance/internal/attempts"
	"tolerance/internal/competitions"
	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/standings"
)

const e2eIssuer = "https://id.example/realms/arena"
const e2eAudience = "arena-web"

// testIdP signs tokens directly against an in-process key set (via
// auth.NewVerifierWithFetcher), the same shape as slice 1's original
// prototype but without a real HTTP JWKS round trip.
type testIdP struct {
	private jwk.Key
	public  jwk.Set
}

func newTestIdP(t *testing.T) *testIdP {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	priv, err := jwk.Import(raw)
	if err != nil {
		t.Fatalf("import key: %v", err)
	}
	if err := jwk.AssignKeyID(priv); err != nil {
		t.Fatalf("assign kid: %v", err)
	}
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256())
	pub, err := jwk.PublicKeyOf(priv)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}
	var kid string
	_ = priv.Get(jwk.KeyIDKey, &kid)
	_ = pub.Set(jwk.KeyIDKey, kid)
	_ = pub.Set(jwk.AlgorithmKey, jwa.RS256())
	set := jwk.NewSet()
	if err := set.AddKey(pub); err != nil {
		t.Fatalf("add key: %v", err)
	}
	return &testIdP{private: priv, public: set}
}

func (idp *testIdP) verifier() *auth.Verifier {
	return auth.NewVerifierWithFetcher(e2eIssuer, e2eAudience, func(ctx context.Context, url string) (jwk.Set, error) {
		return idp.public, nil
	})
}

func (idp *testIdP) token(t *testing.T, subject, email, name string) string {
	t.Helper()
	b := jwt.NewBuilder().Issuer(e2eIssuer).Audience([]string{e2eAudience}).
		Subject(subject).IssuedAt(time.Now()).Expiration(time.Now().Add(time.Hour)).
		Claim("email", email).Claim("name", name)
	token, err := b.Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), idp.private))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

func newTestServer(t *testing.T) (*httptest.Server, *testIdP) {
	t.Helper()
	d := dbtest.New(t)
	idp := newTestIdP(t)
	cfg := config{addr: "127.0.0.1:0", webOrigin: "http://localhost:3000", oidcIssuer: e2eIssuer, oidcAudience: e2eAudience,
		adminEmails: []string{"admin@arena.local"}}
	st := standings.NewService(d.AppPool)
	dp := deps{pool: d.AppPool, verifier: idp.verifier(), users: identity.NewService(d.AppPool, cfg.adminEmails),
		agents: agents.NewService(d.AppPool, st), standings: st, competitions: competitions.NewService(d.AppPool), attempts: attempts.NewService(d.AppPool)}
	srv := httptest.NewServer(newHandler(cfg, dp))
	t.Cleanup(srv.Close)
	return srv, idp
}

// call performs a request, validates the response against openapi.yaml,
// and decodes the JSON body into out (if non-nil).
func call(t *testing.T, router routers.Router, srv *httptest.Server, method, path, token string, body any, out any) int {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, srv.URL+path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if method == http.MethodPost {
		req.Header.Set("Idempotency-Key", idgen.New("idem"))
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	openapi.ValidateResponse(t, router, req, resp, raw)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode %s %s: %v\n%s", method, path, err, raw)
		}
	}
	return resp.StatusCode
}

func TestEndToEnd_Slice1(t *testing.T) {
	srv, idp := newTestServer(t)
	router, err := openapi.Router()
	if err != nil {
		t.Fatal(err)
	}
	adminTok := idp.token(t, "admin-sub", "admin@arena.local", "Admin")
	userTok := idp.token(t, "mira-sub", "mira@example.com", "Mira")

	// /healthz sits outside the versioned /api/v1 prefix and outside the
	// OpenAPI contract; hit it with a plain request instead of call().
	if resp, err := srv.Client().Get(srv.URL + "/healthz"); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %v %v", resp, err)
	}

	// Admin publishes a competition; a plain user may not create one.
	input := map[string]any{"slug": "weekend-planner", "title": "Weekend planner", "summary": "s", "brief": "b",
		"category": "Full build", "difficulty": "Hard", "points": 500, "deadline": time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339),
		"match_duration_seconds": 900, "criteria": []map[string]any{{"name": "Functionality", "weight": 60, "description": "d"}, {"name": "UX & polish", "weight": 40, "description": "d"}}}
	if code := call(t, router, srv, "POST", "/api/v1/admin/competitions", userTok, input, nil); code != http.StatusForbidden {
		t.Fatalf("non-admin must get 403, got %d", code)
	}
	var created struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if code := call(t, router, srv, "POST", "/api/v1/admin/competitions", adminTok, input, &created); code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	if code := call(t, router, srv, "GET", "/api/v1/competitions/weekend-planner", "", nil, nil); code != http.StatusNotFound {
		t.Fatalf("draft must be invisible, got %d", code)
	}
	if code := call(t, router, srv, "POST", "/api/v1/admin/competitions/"+created.ID+"/publish", adminTok,
		map[string]any{"expected_version": created.Version}, nil); code != http.StatusOK {
		t.Fatalf("publish: %d", code)
	}
	var list struct {
		Items []struct {
			Slug   string `json:"slug"`
			Status string `json:"status"`
		} `json:"items"`
	}
	call(t, router, srv, "GET", "/api/v1/competitions?status=active", "", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Slug != "weekend-planner" || list.Items[0].Status != "active" {
		t.Fatalf("public list: %+v", list)
	}

	// User: no agent → null; create; second → 409; key works on /agent/me, revoked → 401.
	var me struct {
		Agent *json.RawMessage `json:"agent"`
	}
	call(t, router, srv, "GET", "/api/v1/me", userTok, nil, &me)
	if me.Agent != nil && string(*me.Agent) != "null" {
		t.Fatalf("agent must be null before creation: %s", *me.Agent)
	}
	if code := call(t, router, srv, "POST", "/api/v1/me/agent", userTok, map[string]any{"name": "Atlas", "model": "Custom", "bio": "b"}, nil); code != http.StatusCreated {
		t.Fatalf("create agent: %d", code)
	}
	if code := call(t, router, srv, "POST", "/api/v1/me/agent", userTok, map[string]any{"name": "Other", "model": "Custom"}, nil); code != http.StatusConflict {
		t.Fatalf("second agent: %d", code)
	}
	var key struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if code := call(t, router, srv, "POST", "/api/v1/me/agent/api-keys", userTok, map[string]any{"name": "laptop"}, &key); code != http.StatusCreated {
		t.Fatalf("create key: %d", code)
	}
	if code := call(t, router, srv, "GET", "/api/v1/agent/me", key.Key, nil, nil); code != http.StatusOK {
		t.Fatalf("agent/me with key: %d", code)
	}
	if code := call(t, router, srv, "GET", "/api/v1/me", key.Key, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("api key on /me must be 401, got %d", code)
	}
	if code := call(t, router, srv, "DELETE", "/api/v1/me/agent/api-keys/"+key.ID, userTok, nil, nil); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	if code := call(t, router, srv, "GET", "/api/v1/agent/me", key.Key, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("revoked key must be 401, got %d", code)
	}

	// Public profile and leaderboard.
	var prof struct {
		Profile struct {
			Agent  string `json:"agent"`
			Author string `json:"author"`
		} `json:"profile"`
		Rank *int `json:"rank"`
	}
	call(t, router, srv, "GET", "/api/v1/agents/atlas", "", nil, &prof)
	if prof.Profile.Agent != "Atlas" || prof.Profile.Author != "mira" || prof.Rank != nil {
		t.Fatalf("profile: %+v", prof)
	}
	var board struct {
		Items []any `json:"items"`
	}
	call(t, router, srv, "GET", "/api/v1/leaderboard", "", nil, &board)
	if len(board.Items) != 0 {
		t.Fatalf("no ranked agents yet: %+v", board)
	}
	var stats struct {
		ActiveCompetitions int `json:"active_competitions"`
	}
	call(t, router, srv, "GET", "/api/v1/stats", "", nil, &stats)
	if stats.ActiveCompetitions != 1 {
		t.Fatalf("stats: %+v", stats)
	}
}

func TestEndToEnd_TaskCompetitionAndDataset(t *testing.T) {
	srv, idp := newTestServer(t)
	router, err := openapi.Router()
	if err != nil {
		t.Fatal(err)
	}
	adminTok := idp.token(t, "admin-sub", "admin@arena.local", "Admin")

	in, err := seed.TaskCompetitionInput("city-day-planner", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if code := call(t, router, srv, "POST", "/api/v1/admin/competitions", adminTok, in, &created); code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	// A draft has no public dataset.
	if code := call(t, router, srv, "GET", "/api/v1/competitions/city-day-planner/dataset", "", nil, nil); code != http.StatusNotFound {
		t.Fatalf("draft dataset: %d", code)
	}
	if code := call(t, router, srv, "POST", "/api/v1/admin/competitions/"+created.ID+"/publish", adminTok,
		map[string]any{"expected_version": created.Version}, nil); code != http.StatusOK {
		t.Fatalf("publish: %d", code)
	}

	var comp struct {
		CheckSuite string         `json:"check_suite"`
		Task       map[string]any `json:"task"`
		Criteria   []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"criteria"`
	}
	if code := call(t, router, srv, "GET", "/api/v1/competitions/city-day-planner", "", nil, &comp); code != http.StatusOK {
		t.Fatalf("get: %d", code)
	}
	if comp.CheckSuite != "city-day-planner" || comp.Task["ui_contract"] == nil || comp.Task["travel_rule"] == nil {
		t.Fatalf("the public competition must carry the task: %+v", comp)
	}
	if comp.Criteria[0].Name != "Functionality" || comp.Criteria[0].Source != "checks" {
		t.Fatalf("criteria: %+v", comp.Criteria)
	}

	var ds struct {
		City   string `json:"city"`
		Places []struct {
			ID string `json:"id"`
		} `json:"places"`
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/competitions/city-day-planner/dataset", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("the dataset is cacheable, got Cache-Control %q", got)
	}
	if code := call(t, router, srv, "GET", "/api/v1/competitions/city-day-planner/dataset", "", nil, &ds); code != http.StatusOK {
		t.Fatalf("dataset: %d", code)
	}
	if ds.City != "Alderhaven" || len(ds.Places) != 24 {
		t.Fatalf("dataset: %+v", ds)
	}
}

func TestEndToEnd_Attempts(t *testing.T) {
	srv, idp := newTestServer(t)
	router, err := openapi.Router()
	if err != nil {
		t.Fatal(err)
	}
	adminTok := idp.token(t, "admin-sub", "admin@arena.local", "Admin")
	userTok := idp.token(t, "mira-sub", "mira@example.com", "Mira")

	in, err := seed.TaskCompetitionInput("city-day-planner", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	call(t, router, srv, "POST", "/api/v1/admin/competitions", adminTok, in, &created)
	call(t, router, srv, "POST", "/api/v1/admin/competitions/"+created.ID+"/publish", adminTok, map[string]any{"expected_version": created.Version}, nil)

	call(t, router, srv, "POST", "/api/v1/me/agent", userTok, map[string]any{"name": "Atlas", "model": "model-a", "bio": "b"}, nil)
	var key struct {
		Key string `json:"key"`
	}
	call(t, router, srv, "POST", "/api/v1/me/agent/api-keys", userTok, map[string]any{"name": "laptop"}, &key)

	base := "/api/v1/agent/competitions/city-day-planner"
	// Wrong kind of credential.
	if code := call(t, router, srv, "GET", base+"/task", userTok, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("a user token on an agent route: %d", code)
	}
	if code := call(t, router, srv, "GET", base+"/task", "", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("no credentials: %d", code)
	}

	var task struct {
		DatasetURL        string `json:"dataset_url"`
		OfficialAvailable bool   `json:"official_available"`
		Attempts          []any  `json:"attempts"`
		Competition       struct {
			Slug string `json:"slug"`
		} `json:"competition"`
	}
	if code := call(t, router, srv, "GET", base+"/task", key.Key, nil, &task); code != http.StatusOK {
		t.Fatalf("task: %d", code)
	}
	if task.DatasetURL != "/api/v1/competitions/city-day-planner/dataset" || !task.OfficialAvailable || len(task.Attempts) != 0 || task.Competition.Slug != "city-day-planner" {
		t.Fatalf("unexpected task response: %+v", task)
	}

	start := map[string]any{"kind": "official", "agent_config": map[string]any{"adapter": "command", "connector_version": "0.1.0", "os": "linux/amd64"}}
	var started struct {
		Attempt struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
			No   int    `json:"no"`
		} `json:"attempt"`
		Resumed bool `json:"resumed"`
	}
	if code := call(t, router, srv, "POST", base+"/attempts", key.Key, start, &started); code != http.StatusCreated {
		t.Fatalf("start: %d", code)
	}
	if started.Resumed || started.Attempt.Kind != "official" || started.Attempt.No != 1 {
		t.Fatalf("start: %+v", started)
	}
	attemptID := started.Attempt.ID
	var again struct {
		Attempt struct {
			ID string `json:"id"`
		} `json:"attempt"`
		Resumed bool `json:"resumed"`
	}
	if code := call(t, router, srv, "POST", base+"/attempts", key.Key, start, &again); code != http.StatusOK || !again.Resumed || again.Attempt.ID != attemptID {
		t.Fatalf("the running attempt must be resumed with 200: %d %+v", code, again)
	}
	if code := call(t, router, srv, "POST", base+"/attempts", key.Key, map[string]any{"kind": "sideways", "agent_config": map[string]any{}}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad kind: %d", code)
	}

	events := "/api/v1/agent/attempts/" + attemptID + "/events"
	good := map[string]any{"events": []map[string]any{
		{"kind": "phase", "phase_index": 3, "text": "Writing the scheduling logic"},
		{"kind": "log", "text": "export ANTHROPIC_API_KEY=sk-ant-abc123def456"},
	}}
	if code := call(t, router, srv, "POST", events, key.Key, good, nil); code != http.StatusNoContent {
		t.Fatalf("events: %d", code)
	}
	forged := map[string]any{"events": []map[string]any{{"kind": "result", "text": "score 100"}}}
	if code := call(t, router, srv, "POST", events, key.Key, forged, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("a connector must not forge a result event: %d", code)
	}

	if code := call(t, router, srv, "POST", "/api/v1/agent/attempts/"+attemptID+"/abandon", key.Key, nil, nil); code != http.StatusNoContent {
		t.Fatalf("abandon: %d", code)
	}
	if code := call(t, router, srv, "POST", events, key.Key, good, nil); code != http.StatusConflict {
		t.Fatalf("events after abandon: %d", code)
	}
	// The official slot stays used until an admin voids the attempt.
	if code := call(t, router, srv, "POST", base+"/attempts", key.Key, start, nil); code != http.StatusConflict {
		t.Fatalf("second official attempt: %d", code)
	}
	void := "/api/v1/admin/attempts/" + attemptID + "/void"
	if code := call(t, router, srv, "POST", void, userTok, map[string]any{"reason": "a perfectly good reason"}, nil); code != http.StatusForbidden {
		t.Fatalf("a non-admin voiding: %d", code)
	}
	if code := call(t, router, srv, "POST", void, key.Key, map[string]any{"reason": "a perfectly good reason"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("an API key on an admin route: %d", code)
	}
	if code := call(t, router, srv, "POST", void, adminTok, map[string]any{"reason": "short"}, nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("a short reason: %d", code)
	}
	if code := call(t, router, srv, "POST", void, adminTok, map[string]any{"reason": "abandoned by mistake, granting a retry"}, nil); code != http.StatusNoContent {
		t.Fatalf("void: %d", code)
	}
	var retry struct {
		Attempt struct {
			No int `json:"no"`
		} `json:"attempt"`
	}
	if code := call(t, router, srv, "POST", base+"/attempts", key.Key, start, &retry); code != http.StatusCreated || retry.Attempt.No != 2 {
		t.Fatalf("a new official attempt after the void: %d %+v", code, retry)
	}
}
