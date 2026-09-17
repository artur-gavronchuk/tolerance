package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"tolerance/internal/budget"
	"tolerance/internal/campaigns"
	"tolerance/internal/identity"
	"tolerance/internal/missions"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
)

const e2eIssuer = "https://id.example/realms/forge"
const e2eAudience = "forge-api"

// testIdP is a minimal, real JWKS endpoint so the end-to-end test drives
// the actual verification path cmd/api uses, not a stand-in.
type testIdP struct {
	server  *httptest.Server
	private jwk.Key
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
	idp := &testIdP{private: priv}
	idp.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(set)
	}))
	t.Cleanup(idp.server.Close)
	return idp
}

func (idp *testIdP) sign(t *testing.T, subject string) string {
	t.Helper()
	token, err := jwt.NewBuilder().Issuer(e2eIssuer).Audience([]string{e2eAudience}).
		Subject(subject).Expiration(time.Now().Add(time.Hour)).Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), idp.private))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

type client struct {
	t       *testing.T
	baseURL string
	token   string
	orgID   string
}

func (c *client) do(method, path string, body any, withOrg bool) (*http.Response, map[string]any) {
	c.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		c.t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if withOrg && c.orgID != "" {
		req.Header.Set("X-Organization-Id", c.orgID)
	}
	if method == http.MethodPost {
		req.Header.Set("Idempotency-Key", fmt.Sprintf("test-%s-%d", path, time.Now().UnixNano()))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	var parsed map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	return resp, parsed
}

// TestEndToEnd_OwnerPublishesContractAndItStaysIsolatedAndImmutable is the
// slice 1 acceptance path from docs/plans/slice-1-contract-and-access.md:
// an owner creates an organization, a campaign, and a mission; confirms
// calibration and opens it, freezing a contract digest; and a second,
// unrelated organization cannot see any of it.
func TestEndToEnd_OwnerPublishesContractAndItStaysIsolatedAndImmutable(t *testing.T) {
	d := dbtest.New(t)
	idp := newTestIdP(t)
	verifier := auth.NewVerifier(e2eIssuer, e2eAudience, idp.server.URL, time.Minute)
	identityService := identity.NewService(d.AppPool)
	campaignService := campaigns.NewService(d.AppPool)
	missionService := missions.NewService(d.AppPool, campaignService)
	budgetService := budget.NewService(d.AppPool)

	cfg := config{spaOrigin: "https://app.example"}
	handler := newHandler(cfg, d.AppPool, verifier, identityService, campaignService, missionService, budgetService)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	if resp, _ := (&client{t: t, baseURL: server.URL}).do(http.MethodGet, "/healthz", nil, false); resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /healthz to answer without auth, got %d", resp.StatusCode)
	}

	owner := &client{t: t, baseURL: server.URL, token: idp.sign(t, "owner-1")}
	resp, org := owner.do(http.MethodPost, "/api/v1/organizations", map[string]string{"name": "Acme"}, false)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create organization: expected 201, got %d: %v", resp.StatusCode, org)
	}
	owner.orgID = org["id"].(string)

	resp, campaign := owner.do(http.MethodPost, "/api/v1/campaigns",
		map[string]any{"name": "Season 01", "mode": "private_trial", "currency": "USD"}, true)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create campaign: expected 201, got %d: %v", resp.StatusCode, campaign)
	}
	campaignID := campaign["id"].(string)

	deadline := time.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	resp, mission := owner.do(http.MethodPost, "/api/v1/campaigns/"+campaignID+"/missions", map[string]any{
		"stage": "build", "title": "Незнакомый город", "brief": "Постройте выполнимый маршрут.",
		"deadline": deadline,
		"requirements": []map[string]any{
			{"stable_key": "B-G2", "gate": true, "weight": 0, "category": "gate", "text": "Маршрут выполним."},
		},
	}, true)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create mission: expected 201, got %d: %v", resp.StatusCode, mission)
	}
	missionID := mission["id"].(string)
	if mission["state"] != "draft" {
		t.Fatalf("expected a freshly created mission to be draft, got %v", mission["state"])
	}

	resp, mission = owner.do(http.MethodPost, "/api/v1/missions/"+missionID+"/confirm-calibration", map[string]any{}, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm calibration: expected 200, got %d: %v", resp.StatusCode, mission)
	}
	version := int(mission["version"].(float64))

	resp, opened := owner.do(http.MethodPost, "/api/v1/missions/"+missionID+"/open",
		map[string]any{"expected_version": version}, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open mission: expected 200, got %d: %v", resp.StatusCode, opened)
	}
	if opened["state"] != "open" {
		t.Fatalf("expected the mission to be open, got %v", opened["state"])
	}
	currentVersion, ok := opened["current_version"].(map[string]any)
	if !ok || currentVersion["contract_digest"] == nil || currentVersion["contract_digest"] == "" {
		t.Fatalf("expected an open mission to carry a contract digest, got %v", opened["current_version"])
	}

	// A second, unrelated organization must not be able to read any of this
	// by id, even though the ids are known and well-formed.
	stranger := &client{t: t, baseURL: server.URL, token: idp.sign(t, "stranger-1")}
	resp, strangerOrg := stranger.do(http.MethodPost, "/api/v1/organizations", map[string]string{"name": "Other Co"}, false)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create second organization: expected 201, got %d: %v", resp.StatusCode, strangerOrg)
	}
	stranger.orgID = strangerOrg["id"].(string)

	resp, body := stranger.do(http.MethodGet, "/api/v1/missions/"+missionID, nil, true)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected a stranger organization to get 404 for another org's mission, got %d: %v", resp.StatusCode, body)
	}
	resp, body = stranger.do(http.MethodGet, "/api/v1/campaigns/"+campaignID, nil, true)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected a stranger organization to get 404 for another org's campaign, got %d: %v", resp.StatusCode, body)
	}
}
