package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"tolerance/internal/platform/dbtest"
	"tolerance/internal/platform/ratelimit"
)

// stubIdentifyProvider is a provider whose Identify always succeeds, so the
// callback reaches its final, post-sign-in redirect.
type stubIdentifyProvider struct{}

func (stubIdentifyProvider) AuthCodeURL(state, verifier, redirectURL string) string { return "" }
func (stubIdentifyProvider) Identify(context.Context, string, string, string) (Identity, error) {
	return Identity{Provider: "github", Subject: "cb-1", Email: "cb@example.com", EmailVerified: true}, nil
}

// TestOAuthCallback_FinalRedirectRejectsAForgedBackslashNext defends the
// callback's own final redirect, not just oauthStart: the arena_oauth
// cookie is not signed, so nothing stops a forged cookie carrying a
// malicious Next from reaching this code path directly.
func TestOAuthCallback_FinalRedirectRejectsAForgedBackslashNext(t *testing.T) {
	d := dbtest.New(t)
	s := NewService(d.AppPool, nil)
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, s, ratelimit.New(nil), AuthConfig{
		Providers: map[string]Provider{"github": stubIdentifyProvider{}},
		PublicURL: "https://tolerance.test/",
	})

	cookie := encodeState(oauthState{Provider: "github", State: "st-1", Verifier: "v-1", Next: "/./\\evil.com"})
	req := httptest.NewRequest("GET", "/api/v1/auth/github/callback?code=c&state=st-1", nil)
	req.AddCookie(&http.Cookie{Name: OAuthCookie, Value: cookie})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/app" {
		t.Fatalf("Location = %q, want /app (forged Next must not survive to the redirect)", loc)
	}
}
