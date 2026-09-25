package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

// Provider is one external sign-in: the URL to send the browser to, and the
// identity behind the code it comes back with. Both use PKCE (S256).
type Provider interface {
	AuthCodeURL(state, verifier, redirectURL string) string
	Identify(ctx context.Context, code, verifier, redirectURL string) (Identity, error)
}

// GitHub signs in with a GitHub OAuth App. Empty URLs mean github.com; tests
// point them at a fake.
type GitHub struct {
	ClientID, ClientSecret    string
	AuthURL, TokenURL, APIURL string
	HTTP                      *http.Client
}

func (g *GitHub) config(redirectURL string) *oauth2.Config {
	ep := endpoints.GitHub
	if g.AuthURL != "" {
		ep.AuthURL = g.AuthURL
	}
	if g.TokenURL != "" {
		ep.TokenURL = g.TokenURL
	}
	return &oauth2.Config{ClientID: g.ClientID, ClientSecret: g.ClientSecret, Endpoint: ep,
		RedirectURL: redirectURL, Scopes: []string{"read:user", "user:email"}}
}

func (g *GitHub) AuthCodeURL(state, verifier, redirectURL string) string {
	return g.config(redirectURL).AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

func (g *GitHub) Identify(ctx context.Context, code, verifier, redirectURL string) (Identity, error) {
	client, err := exchange(ctx, g.HTTP, g.config(redirectURL), code, verifier)
	if err != nil {
		return Identity{}, err
	}
	api := g.APIURL
	if api == "" {
		api = "https://api.github.com"
	}
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := getJSON(ctx, client, api+"/user", &user); err != nil {
		return Identity{}, err
	}
	if user.ID == 0 {
		return Identity{}, errors.New("github: user without an id")
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := getJSON(ctx, client, api+"/user/emails", &emails); err != nil {
		return Identity{}, err
	}
	id := Identity{Provider: "github", Subject: strconv.FormatInt(user.ID, 10), Login: user.Login}
	for _, e := range emails {
		if e.Primary && e.Verified {
			id.Email, id.EmailVerified = e.Email, true
		}
	}
	return id, nil
}

// Google signs in with OpenID Connect, reading the identity from userinfo
// with the access token (fetched directly from Google over TLS, so the
// id_token's signature does not need checking). Empty URLs mean Google's.
type Google struct {
	ClientID, ClientSecret         string
	AuthURL, TokenURL, UserInfoURL string
	HTTP                           *http.Client
}

func (g *Google) config(redirectURL string) *oauth2.Config {
	ep := endpoints.Google
	if g.AuthURL != "" {
		ep.AuthURL = g.AuthURL
	}
	if g.TokenURL != "" {
		ep.TokenURL = g.TokenURL
	}
	return &oauth2.Config{ClientID: g.ClientID, ClientSecret: g.ClientSecret, Endpoint: ep,
		RedirectURL: redirectURL, Scopes: []string{"openid", "email", "profile"}}
}

func (g *Google) AuthCodeURL(state, verifier, redirectURL string) string {
	return g.config(redirectURL).AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

func (g *Google) Identify(ctx context.Context, code, verifier, redirectURL string) (Identity, error) {
	client, err := exchange(ctx, g.HTTP, g.config(redirectURL), code, verifier)
	if err != nil {
		return Identity{}, err
	}
	u := g.UserInfoURL
	if u == "" {
		u = "https://openidconnect.googleapis.com/v1/userinfo"
	}
	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := getJSON(ctx, client, u, &info); err != nil {
		return Identity{}, err
	}
	if info.Sub == "" {
		return Identity{}, errors.New("google: userinfo without sub")
	}
	return Identity{Provider: "google", Subject: info.Sub, Email: info.Email, EmailVerified: info.EmailVerified}, nil
}

// exchange trades the code for a token and returns a client that sends it.
func exchange(ctx context.Context, hc *http.Client, cfg *oauth2.Config, code, verifier string) (*http.Client, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	return cfg.Client(ctx, tok), nil
}

func getJSON(ctx context.Context, c *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

// oauthState rides in the arena_oauth cookie between start and callback.
type oauthState struct {
	Provider string `json:"p"`
	State    string `json:"s"`
	Verifier string `json:"v"`
	Next     string `json:"n"`
}

func encodeState(st oauthState) string {
	b, _ := json.Marshal(st)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeState(v string) (oauthState, error) {
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return oauthState{}, err
	}
	var st oauthState
	if err := json.Unmarshal(b, &st); err != nil {
		return oauthState{}, err
	}
	if st.Provider == "" || st.State == "" || st.Verifier == "" {
		return oauthState{}, errors.New("identity: incomplete oauth state")
	}
	return st, nil
}

// safeNext keeps a post-sign-in redirect on this site: a path, never a
// scheme-relative or backslash URL a browser would read as another host.
// http.Redirect runs path.Clean on the target, and path.Clean does not
// treat '\' as a separator, so "/./\evil.com" cleans to "/\evil.com" —
// which a browser reads as "//evil.com". Reject any backslash outright
// rather than trying to out-think path.Clean's ".." handling.
func safeNext(next string) string {
	if len(next) > 512 {
		return "/app"
	}
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/app"
	}
	for i := 0; i < len(next); i++ {
		if b := next[i]; b == '\\' || b < 0x20 || b == 0x7f {
			return "/app"
		}
	}
	p := next
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if strings.HasPrefix(path.Clean(p), "//") {
		return "/app"
	}
	return next
}
