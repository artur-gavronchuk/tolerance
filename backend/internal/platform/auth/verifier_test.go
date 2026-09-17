package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"tolerance/internal/platform/auth"
)

const testIssuer = "https://id.example/realms/forge"
const testAudience = "forge-api"

// issuer bundles a signing key and its published public key set, so tests
// can sign tokens and verify them without any real network round trip.
type issuer struct {
	private jwk.Key
	public  jwk.Set
}

func newIssuer(t *testing.T) *issuer {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	priv, err := jwk.Import(raw)
	if err != nil {
		t.Fatalf("import private key: %v", err)
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
	if err := priv.Get(jwk.KeyIDKey, &kid); err != nil {
		t.Fatalf("read kid: %v", err)
	}
	if err := pub.Set(jwk.KeyIDKey, kid); err != nil {
		t.Fatalf("set public kid: %v", err)
	}
	if err := pub.Set(jwk.AlgorithmKey, jwa.RS256()); err != nil {
		t.Fatalf("set public alg: %v", err)
	}

	set := jwk.NewSet()
	if err := set.AddKey(pub); err != nil {
		t.Fatalf("add public key to set: %v", err)
	}
	return &issuer{private: priv, public: set}
}

func (iss *issuer) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	b := jwt.NewBuilder().Issuer(testIssuer).Audience([]string{testAudience}).
		Subject("user-1").IssuedAt(time.Now()).Expiration(time.Now().Add(time.Hour))
	for k, v := range claims {
		b = b.Claim(k, v)
	}
	token, err := b.Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), iss.private))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

func (iss *issuer) fetcher() auth.Fetcher {
	return func(ctx context.Context, url string) (jwk.Set, error) { return iss.public, nil }
}

func newVerifierForTest(t *testing.T, iss *issuer) *auth.Verifier {
	t.Helper()
	v := auth.NewVerifier(testIssuer, testAudience, "https://id.example/jwks", time.Minute)
	auth.SetFetcherForTest(v, iss.fetcher())
	return v
}

func TestVerify_AcceptsAValidlySignedToken(t *testing.T) {
	iss := newIssuer(t)
	v := newVerifierForTest(t, iss)
	token := iss.sign(t, map[string]any{"email": "dev@example.com", "name": "Dev"})

	claims, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("expected a validly signed token to verify, got: %v", err)
	}
	if claims.Subject != "user-1" || claims.Issuer != testIssuer {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.Email != "dev@example.com" {
		t.Fatalf("expected email claim to be carried through, got %q", claims.Email)
	}
}

func TestVerify_RejectsATokenSignedByAnUnknownKey(t *testing.T) {
	trusted := newIssuer(t)
	attacker := newIssuer(t) // a different keypair, not in the trusted JWKS
	v := newVerifierForTest(t, trusted)
	token := attacker.sign(t, nil)

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatalf("expected a token signed by an untrusted key to be rejected")
	}
}

func TestVerify_RejectsAnExpiredToken(t *testing.T) {
	iss := newIssuer(t)
	v := newVerifierForTest(t, iss)
	token, err := jwt.NewBuilder().Issuer(testIssuer).Audience([]string{testAudience}).
		Subject("user-1").Expiration(time.Now().Add(-time.Hour)).Build()
	if err != nil {
		t.Fatalf("build expired token: %v", err)
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), iss.private))
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}

	if _, err := v.Verify(context.Background(), string(signed)); err == nil {
		t.Fatalf("expected an expired token to be rejected")
	}
}

func TestVerify_RejectsWrongAudience(t *testing.T) {
	iss := newIssuer(t)
	v := newVerifierForTest(t, iss)
	token, err := jwt.NewBuilder().Issuer(testIssuer).Audience([]string{"some-other-api"}).
		Subject("user-1").Expiration(time.Now().Add(time.Hour)).Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), iss.private))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := v.Verify(context.Background(), string(signed)); err == nil {
		t.Fatalf("expected a token for a different audience to be rejected")
	}
}
