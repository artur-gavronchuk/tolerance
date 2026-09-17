package identity_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"tolerance/internal/identity"
	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/dbtest"
)

const testIssuer = "https://id.example/realms/forge"
const testAudience = "forge-api"

// testIdP serves a real JWKS document over HTTP and can sign tokens against
// it, so Middleware is exercised against the same fetch path production
// uses, not a stub.
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
	if err := priv.Set(jwk.AlgorithmKey, jwa.RS256()); err != nil {
		t.Fatalf("set alg: %v", err)
	}
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
	token, err := jwt.NewBuilder().Issuer(testIssuer).Audience([]string{testAudience}).
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

func TestMiddleware_RejectsMissingOrInvalidToken(t *testing.T) {
	d := dbtest.New(t)
	idp := newTestIdP(t)
	verifier := auth.NewVerifier(testIssuer, testAudience, idp.server.URL, time.Minute)
	service := identity.NewService(d.AppPool)
	handler := identity.Middleware(verifier, service)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no token, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	r.Header.Set("Authorization", "Bearer not-a-real-token")
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with a garbage token, got %d", w.Code)
	}
}

func TestMiddleware_ForbidsAnOrganizationTheUserIsNotAMemberOf(t *testing.T) {
	d := dbtest.New(t)
	idp := newTestIdP(t)
	verifier := auth.NewVerifier(testIssuer, testAudience, idp.server.URL, time.Minute)
	service := identity.NewService(d.AppPool)

	// The token's subject resolves to a user with no memberships anywhere.
	token := idp.sign(t, "no-memberships")
	// Arrange an organization owned by someone else, so it exists to be
	// denied, not merely absent.
	owner, err := service.ResolveUser(context.Background(), auth.Claims{Issuer: testIssuer, Subject: "owner"})
	if err != nil {
		t.Fatalf("resolve owner: %v", err)
	}
	org, err := service.CreateOrganizationWithOwner(context.Background(), owner.ID, "Acme")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	var reached bool
	handler := identity.Middleware(verifier, service)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Organization-Id", org.ID)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-member, got %d", w.Code)
	}
	if reached {
		t.Fatalf("expected the inner handler not to run for a forbidden request")
	}
}

func TestMiddleware_AttachesActorForAnActiveMember(t *testing.T) {
	d := dbtest.New(t)
	idp := newTestIdP(t)
	verifier := auth.NewVerifier(testIssuer, testAudience, idp.server.URL, time.Minute)
	service := identity.NewService(d.AppPool)

	token := idp.sign(t, "owner")
	owner, err := service.ResolveUser(context.Background(), auth.Claims{Issuer: testIssuer, Subject: "owner"})
	if err != nil {
		t.Fatalf("resolve owner: %v", err)
	}
	org, err := service.CreateOrganizationWithOwner(context.Background(), owner.ID, "Acme")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	var gotActor identity.Actor
	handler := identity.Middleware(verifier, service)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActor = identity.MustFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Organization-Id", org.ID)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for an active member, got %d", w.Code)
	}
	if gotActor.OrganizationID != org.ID || gotActor.Role != "owner" || gotActor.UserID != owner.ID {
		t.Fatalf("unexpected actor: %+v", gotActor)
	}
}
