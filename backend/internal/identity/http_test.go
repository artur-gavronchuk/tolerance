package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"tolerance/internal/platform/ratelimit"
)

func TestClientIP_XForwardedFor(t *testing.T) {
	tests := []struct {
		name        string
		xff         string
		remoteAddr  string
		expectedIP  string
		description string
	}{
		{
			name:        "single IP in X-Forwarded-For",
			xff:         "192.0.2.1",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "192.0.2.1",
			description: "single IP should be used",
		},
		{
			name:        "multiple IPs - uses last",
			xff:         "1.2.3.4, 10.0.0.5",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "10.0.0.5",
			description: "last IP (from reverse proxy) should be used, not attacker prefix",
		},
		{
			name:        "attacker tries different prefixes - still same bucket",
			xff:         "9.9.9.9, 10.0.0.5",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "10.0.0.5",
			description: "attacker-controlled prefix should be ignored, last hop should be used",
		},
		{
			name:        "many IPs in chain",
			xff:         "1.1.1.1, 2.2.2.2, 3.3.3.3, 10.0.0.5",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "10.0.0.5",
			description: "last IP in chain should be used",
		},
		{
			name:        "whitespace handling",
			xff:         "1.2.3.4 , 10.0.0.5 ",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "10.0.0.5",
			description: "whitespace should be trimmed from last IP",
		},
		{
			name:        "no X-Forwarded-For uses RemoteAddr",
			xff:         "",
			remoteAddr:  "192.0.2.1:54321",
			expectedIP:  "192.0.2.1",
			description: "without header, IP from RemoteAddr should be used",
		},
		{
			name:        "no X-Forwarded-For with invalid RemoteAddr",
			xff:         "",
			remoteAddr:  "invalid",
			expectedIP:  "invalid",
			description: "invalid RemoteAddr should be returned as-is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/auth/dev", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}

			got := clientIP(req)
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
