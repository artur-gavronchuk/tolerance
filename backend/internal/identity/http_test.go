package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/ratelimit"
)

func TestClientIP_TrustProxyGatesTheRealIPHeader(t *testing.T) {
	tests := []struct {
		name        string
		trustProxy  bool
		realIP      string
		remoteAddr  string
		expectedIP  string
		description string
	}{
		{
			name:        "trusted proxy sets X-Real-IP",
			trustProxy:  true,
			realIP:      "192.0.2.1",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "192.0.2.1",
			description: "the proxy's resolved client address should be used",
		},
		{
			name:        "trusted proxy but header absent falls back to RemoteAddr",
			trustProxy:  true,
			realIP:      "",
			remoteAddr:  "192.0.2.1:54321",
			expectedIP:  "192.0.2.1",
			description: "without the header, RemoteAddr is used",
		},
		{
			name:        "header ignored when the proxy is not trusted",
			trustProxy:  false,
			realIP:      "9.9.9.9",
			remoteAddr:  "192.0.2.1:54321",
			expectedIP:  "192.0.2.1",
			description: "a client-controlled header must never be trusted without ARENA_TRUST_PROXY",
		},
		{
			name:        "no header, no trust, uses RemoteAddr",
			trustProxy:  false,
			realIP:      "",
			remoteAddr:  "192.0.2.1:54321",
			expectedIP:  "192.0.2.1",
			description: "without header, IP from RemoteAddr should be used",
		},
		{
			name:        "invalid RemoteAddr returned as-is",
			trustProxy:  false,
			realIP:      "",
			remoteAddr:  "invalid",
			expectedIP:  "invalid",
			description: "invalid RemoteAddr should be returned as-is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/auth/dev", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}

			got := clientIP(req, tt.trustProxy)
			if got != tt.expectedIP {
				t.Fatalf("got %q, want %q (%s)", got, tt.expectedIP, tt.description)
			}
		})
	}
}

type stubProvider struct{ authURL string }

func (p stubProvider) AuthCodeURL(state, verifier, redirectURL string) string {
	v := url.Values{"state": {state}, "redirect_uri": {redirectURL}, "code_challenge_method": {"S256"}}
	return p.authURL + "?" + v.Encode()
}
func (stubProvider) Identify(context.Context, string, string, string) (Identity, error) {
	return Identity{}, errors.New("stub: not reached in these tests")
}

func authMux() *http.ServeMux {
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, NewService(nil, nil), ratelimit.New(nil), AuthConfig{
		Providers: map[string]Provider{"github": stubProvider{authURL: "https://gh.test/authorize"}},
		PublicURL: "https://tolerance.test/",
	})
	return mux
}

func serve(mux *http.ServeMux, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestOAuthStart_RedirectsWithStateAndSetsTheCookie(t *testing.T) {
	rec := serve(authMux(), httptest.NewRequest("GET", "/api/v1/auth/github/start?next=//evil.com", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Host != "gh.test" || loc.Query().Get("redirect_uri") != "https://tolerance.test/api/v1/auth/github/callback" {
		t.Fatalf("location %s", loc)
	}
	var c *http.Cookie
	for _, k := range rec.Result().Cookies() {
		if k.Name == OAuthCookie {
			c = k
		}
	}
	if c == nil || !c.HttpOnly || c.Path != "/api/v1/auth/" || c.MaxAge != 600 {
		t.Fatalf("cookie %+v", c)
	}
	st, err := decodeState(c.Value)
	if err != nil || st.State != loc.Query().Get("state") || st.Provider != "github" || st.Next != "/app" {
		t.Fatalf("state %+v %v", st, err)
	}
}

func TestOAuthStart_RejectsABackslashNextEvenWhenPathCleanWouldTurnItIntoADoubleSlash(t *testing.T) {
	// /./\evil.com: path.Clean does not treat '\' as a separator, so
	// http.Redirect's own path.Clean call turns this into "/\evil.com",
	// which a browser reads as "//evil.com" (an open redirect).
	rec := serve(authMux(), httptest.NewRequest("GET", "/api/v1/auth/github/start?next=%2F.%2F%5Cevil.com", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	var c *http.Cookie
	for _, k := range rec.Result().Cookies() {
		if k.Name == OAuthCookie {
			c = k
		}
	}
	if c == nil {
		t.Fatal("no oauth cookie set")
	}
	st, err := decodeState(c.Value)
	if err != nil || st.Next != "/app" {
		t.Fatalf("state %+v %v, want Next /app", st, err)
	}
}

func TestOAuthStart_UnknownProviderIs404(t *testing.T) {
	if rec := serve(authMux(), httptest.NewRequest("GET", "/api/v1/auth/google/start", nil)); rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestOAuthCallback_Failures(t *testing.T) {
	good := encodeState(oauthState{Provider: "github", State: "s1", Verifier: "v1", Next: "/app"})
	other := encodeState(oauthState{Provider: "google", State: "s1", Verifier: "v1", Next: "/app"})
	for name, tc := range map[string]struct {
		query, cookie, want string
	}{
		"provider error": {"error=access_denied&state=s1", good, "oauth_denied"},
		"no cookie":      {"code=c&state=s1", "", "oauth_state"},
		"wrong state":    {"code=c&state=s2", good, "oauth_state"},
		"other provider": {"code=c&state=s1", other, "oauth_state"},
		"garbage cookie": {"code=c&state=s1", "%%%", "oauth_state"},
		"identify fails": {"code=c&state=s1", good, "oauth_failed"},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/auth/github/callback?"+tc.query, nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: OAuthCookie, Value: tc.cookie})
			}
			rec := serve(authMux(), req)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login?error="+tc.want {
				t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
			}
			for _, k := range rec.Result().Cookies() {
				if k.Name == SessionCookie {
					t.Fatal("no session may be set on failure")
				}
			}
		})
	}
}

// ErrEmailUnverified used to be a shared *httpx.Problem; httpx.WriteError
// mutates a matched Problem's RequestID field in place, so every caller
// across every request would race on and clobber the same package-level
// value. It must be a plain sentinel that errors.As does not match.
func TestErrEmailUnverified_IsNotAnHTTPXProblem(t *testing.T) {
	var p *httpx.Problem
	if errors.As(ErrEmailUnverified, &p) {
		t.Fatal("ErrEmailUnverified must not be a *httpx.Problem: httpx.WriteError would mutate this shared value's RequestID on every request that reaches it")
	}
	if !errors.Is(ErrEmailUnverified, ErrEmailUnverified) {
		t.Fatal("errors.Is(ErrEmailUnverified, ErrEmailUnverified) must still hold")
	}
}

func TestDevSignIn_RejectsADisplayNameAddress(t *testing.T) {
	// mail.ParseAddress happily accepts "Name <a@b.c>"; without an exact
	// match against addr.Address this would sign in as a mangled email
	// ("name <a@b.c>", after NormalizeEmail) instead of being rejected.
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, NewService(nil, nil), ratelimit.New(nil), AuthConfig{DevLogin: true})
	body := strings.NewReader(`{"email":"Name <a@b.c>"}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/dev", body)
	rec := serve(mux, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterAuthRoutes_DevLoginOffIs404(t *testing.T) {
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, NewService(nil, nil), ratelimit.New(nil), AuthConfig{})
	if rec := serve(mux, httptest.NewRequest("POST", "/api/v1/auth/dev", nil)); rec.Code != http.StatusNotFound {
		t.Fatalf("dev login status %d", rec.Code)
	}
	rec := serve(mux, httptest.NewRequest("GET", "/api/v1/auth/providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("providers status %d", rec.Code)
	}
	var out struct {
		Providers []string `json:"providers"`
		DevLogin  bool     `json:"dev_login"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Providers == nil || len(out.Providers) != 0 || out.DevLogin {
		t.Fatalf("providers body %+v", out)
	}
}
