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
	"tolerance/internal/agents"
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
		agents: agents.NewService(d.AppPool, st), standings: st, competitions: competitions.NewService(d.AppPool)}
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
