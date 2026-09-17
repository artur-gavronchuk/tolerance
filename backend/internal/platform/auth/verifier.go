// Package auth verifies OIDC access tokens against a provider's published
// JWKS. It does not perform the OIDC discovery dance (issuer and JWKS URL
// are supplied by configuration) and it does not manage login itself; the
// SPA talks to the provider directly and only ever hands this API a bearer
// token to check.
package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// Claims is the subset of an access token's claims the rest of the system
// needs. Email and Name are best-effort: many providers only put them on
// the ID token, not the access token, and slice 1 does not call userinfo to
// backfill them.
type Claims struct {
	Issuer  string
	Subject string
	Email   string
	Name    string
}

// Fetcher fetches a JWKS document. jwk.Fetch satisfies this; tests supply a
// fake so verification can be tested without a real HTTP round trip.
type Fetcher func(ctx context.Context, url string) (jwk.Set, error)

// Verifier checks a bearer token's signature against a cached JWKS and its
// iss/aud/exp against the expected values.
type Verifier struct {
	issuer   string
	audience string
	jwksURL  string
	ttl      time.Duration
	fetch    Fetcher

	mu       sync.Mutex
	cached   jwk.Set
	cachedAt time.Time
}

// NewVerifier builds a Verifier that fetches from jwksURL, refetching at
// most once per ttl. A ttl of zero uses a 10 minute default.
func NewVerifier(issuer, audience, jwksURL string, ttl time.Duration) *Verifier {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Verifier{issuer: issuer, audience: audience, jwksURL: jwksURL, ttl: ttl,
		fetch: func(ctx context.Context, url string) (jwk.Set, error) { return jwk.Fetch(ctx, url) }}
}

func (v *Verifier) keySet(ctx context.Context) (jwk.Set, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cached != nil && time.Since(v.cachedAt) < v.ttl {
		return v.cached, nil
	}
	set, err := v.fetch(ctx, v.jwksURL)
	if err != nil {
		if v.cached != nil {
			// Serving a stale key set beats rejecting every request during
			// a provider blip; rotation still lands within one ttl window.
			return v.cached, nil
		}
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	v.cached, v.cachedAt = set, time.Now()
	return set, nil
}

// Verify checks rawToken's signature, issuer, audience and expiry, and
// returns the claims the rest of the system needs. It never accepts an
// unsigned or "alg: none" token: jwt.Parse requires a key set match.
func (v *Verifier) Verify(ctx context.Context, rawToken string) (Claims, error) {
	set, err := v.keySet(ctx)
	if err != nil {
		return Claims{}, err
	}
	token, err := jwt.Parse([]byte(rawToken),
		jwt.WithKeySet(set),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("verify token: %w", err)
	}
	subject, ok := token.Subject()
	if !ok || subject == "" {
		return Claims{}, fmt.Errorf("verify token: missing subject claim")
	}
	issuer, _ := token.Issuer()
	claims := Claims{Issuer: issuer, Subject: subject}
	_ = token.Get("email", &claims.Email)
	_ = token.Get("name", &claims.Name)
	return claims, nil
}
