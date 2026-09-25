package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// fakeOAuth is a provider's token endpoint that checks the PKCE verifier
// against the challenge sent to the authorize URL, plus JSON API routes.
type fakeOAuth struct {
	t         *testing.T
	challenge string
	routes    map[string]any
	srv       *httptest.Server
}

func newFakeOAuth(t *testing.T, routes map[string]any) *fakeOAuth {
	f := &fakeOAuth{t: t, routes: routes}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","token_type":"bearer"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := f.routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// authorize reads the challenge off an authorize URL, as the provider would.
func (f *fakeOAuth) authorize(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("PKCE S256 missing: %s", raw)
	}
	f.challenge = q.Get("code_challenge")
	return q
}

func TestGitHub_IdentifiesByIDWithThePrimaryVerifiedEmail(t *testing.T) {
	f := newFakeOAuth(t, map[string]any{
		"/user": map[string]any{"id": 4242, "login": "octo"},
		"/user/emails": []map[string]any{
			{"email": "old@example.com", "primary": false, "verified": true},
			{"email": "octo@example.com", "primary": true, "verified": true},
		},
	})
	p := &GitHub{ClientID: "cid", ClientSecret: "sec", AuthURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token", APIURL: f.srv.URL}
	verifier := oauth2.GenerateVerifier()
	q := f.authorize(t, p.AuthCodeURL("st", verifier, "http://arena.test/cb"))
	if q.Get("state") != "st" || q.Get("client_id") != "cid" || q.Get("redirect_uri") != "http://arena.test/cb" || !strings.Contains(q.Get("scope"), "user:email") {
		t.Fatalf("authorize params: %v", q)
	}
	id, err := p.Identify(context.Background(), "good-code", verifier, "http://arena.test/cb")
	if err != nil {
		t.Fatal(err)
	}
	want := Identity{Provider: "github", Subject: "4242", Email: "octo@example.com", EmailVerified: true, Login: "octo"}
	if id != want {
		t.Fatalf("got %+v want %+v", id, want)
	}
}

func TestGitHub_NoPrimaryVerifiedEmailIsUnverified(t *testing.T) {
	f := newFakeOAuth(t, map[string]any{
		"/user":        map[string]any{"id": 1, "login": "x"},
		"/user/emails": []map[string]any{{"email": "x@example.com", "primary": true, "verified": false}},
	})
	p := &GitHub{ClientID: "c", ClientSecret: "s", AuthURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token", APIURL: f.srv.URL}
	v := oauth2.GenerateVerifier()
	f.authorize(t, p.AuthCodeURL("s", v, "http://arena.test/cb"))
	id, err := p.Identify(context.Background(), "good-code", v, "http://arena.test/cb")
	if err != nil {
		t.Fatal(err)
	}
	if id.EmailVerified {
		t.Fatalf("must not be verified: %+v", id)
	}
}

func TestGitHub_WrongVerifierFails(t *testing.T) {
	f := newFakeOAuth(t, map[string]any{"/user": map[string]any{"id": 1}})
	p := &GitHub{ClientID: "c", ClientSecret: "s", AuthURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token", APIURL: f.srv.URL}
	f.authorize(t, p.AuthCodeURL("s", oauth2.GenerateVerifier(), "http://arena.test/cb"))
	if _, err := p.Identify(context.Background(), "good-code", oauth2.GenerateVerifier(), "http://arena.test/cb"); err == nil {
		t.Fatal("a verifier that does not match the challenge must fail")
	}
}

func TestGoogle_IdentifiesBySub(t *testing.T) {
	f := newFakeOAuth(t, map[string]any{
		"/userinfo": map[string]any{"sub": "g-123", "email": "a@gmail.com", "email_verified": true},
	})
	p := &Google{ClientID: "c", ClientSecret: "s", AuthURL: f.srv.URL + "/authorize", TokenURL: f.srv.URL + "/token", UserInfoURL: f.srv.URL + "/userinfo"}
	v := oauth2.GenerateVerifier()
	q := f.authorize(t, p.AuthCodeURL("s", v, "http://arena.test/cb"))
	if !strings.Contains(q.Get("scope"), "openid") || !strings.Contains(q.Get("scope"), "email") {
		t.Fatalf("scope: %q", q.Get("scope"))
	}
	id, err := p.Identify(context.Background(), "good-code", v, "http://arena.test/cb")
	if err != nil {
		t.Fatal(err)
	}
	want := Identity{Provider: "google", Subject: "g-123", Email: "a@gmail.com", EmailVerified: true}
	if id != want {
		t.Fatalf("got %+v want %+v", id, want)
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"":                 "/app",
		"/app":             "/app",
		"/app/proofs/p_1":  "/app/proofs/p_1",
		"//evil.com":       "/app",
		"/\\evil.com":      "/app",
		"https://evil.com": "/app",
		"app":              "/app",
		"/app\r\nX: y":     "/app",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOAuthState_RoundTripsAndRejectsGarbage(t *testing.T) {
	st := oauthState{Provider: "github", State: "abc", Verifier: "ver", Next: "/app/x"}
	got, err := decodeState(encodeState(st))
	if err != nil || got != st {
		t.Fatalf("round trip: %v %+v", err, got)
	}
	for _, bad := range []string{"", "not base64!", base64.RawURLEncoding.EncodeToString([]byte("{}"))} {
		if _, err := decodeState(bad); err == nil {
			t.Errorf("decodeState(%q) must fail", bad)
		}
	}
}
