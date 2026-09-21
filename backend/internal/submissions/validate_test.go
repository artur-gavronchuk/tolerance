package submissions_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"tolerance/internal/platform/httpx"
	"tolerance/internal/submissions"
)

const goodSummary = "Static planner: greedy nearest-feasible scheduling with remove and replace."

func ok() submissions.Input {
	return submissions.Input{Summary: goodSummary, PreviewURL: "https://a.github.io/planner/"}
}

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) || p.Code != "invalid_body" || len(p.Fields) == 0 {
		t.Fatalf("want invalid_body with a field, got %v", err)
	}
	return p.Fields[0].Path
}

func TestValidate_PreviewURL(t *testing.T) {
	cases := []struct {
		url           string
		ok            bool // without the loopback flag
		okWithLoopack bool
	}{
		{"https://a.github.io/x/", true, true},
		{"https://example.com", true, true},
		{"https://example.com:8443/app?x=1#top", true, true},
		{"https://Sub.Example.COM/", true, true},
		{"http://evil.example.com", false, false},
		{"ftp://example.com/x", false, false},
		{"//example.com/x", false, false},
		{"example.com", false, false},
		{"", false, false},
		{"https://", false, false},
		{"https://user:pw@example.com", false, false},
		{"https://user@example.com", false, false},
		{"https://10.0.0.1", false, false},
		{"https://172.16.5.4/x", false, false},
		{"https://192.168.1.1", false, false},
		{"https://169.254.169.254/latest/meta-data/", false, false},
		{"https://100.64.0.1", false, false},
		{"https://0.0.0.0", false, false},
		{"https://224.0.0.1", false, false},
		{"https://[::1]", false, true},
		{"https://[::]", false, false},
		{"https://[fd00::1]", false, false},
		{"https://[fe80::1]", false, false},
		{"https://[::ffff:10.0.0.1]", false, false},
		{"https://[::ffff:127.0.0.1]", false, true},
		{"https://127.0.0.1", false, true},
		{"http://127.0.0.1:4173", false, true},
		{"http://localhost:3000", false, true},
		{"https://localhost", false, true},
		{"http://foo.localhost:3000", false, true},
		{"http://10.0.0.1:3000", false, false},
		{"https://metadata.google.internal/", false, false},
		{"https://service.internal", false, false},
		{"https://printer.local", false, false},
		{"https://intranet", false, false},
		{"https://2130706433/", false, false},
		{"https://0x7f.0.0.1/", false, false},
		{"https://127.1/", false, false},
		{"https://example.com/" + strings.Repeat("a", 2100), false, false},
		{"https://exa mple.com", false, false},
	}
	for _, c := range cases {
		for _, allow := range []bool{false, true} {
			want := c.ok
			if allow {
				want = c.okWithLoopack
			}
			in := ok()
			in.PreviewURL = c.url
			err := in.Validate(allow)
			if want && err != nil {
				t.Errorf("%q (loopback=%v) must be accepted: %v", c.url, allow, err)
			}
			if !want {
				if err == nil {
					t.Errorf("%q (loopback=%v) must be rejected", c.url, allow)
				} else if got := fieldOf(t, err); got != "preview_url" {
					t.Errorf("%q: error on %q, want preview_url", c.url, got)
				}
			}
		}
	}
}

func TestValidate_Fields(t *testing.T) {
	sha := strings.Repeat("a", 40)
	cases := []struct {
		name  string
		edit  func(*submissions.Input)
		field string // "" means accepted
	}{
		{"minimal input", func(i *submissions.Input) {}, ""},
		{"summary too short", func(i *submissions.Input) { i.Summary = "too short" }, "summary"},
		{"summary blank padding", func(i *submissions.Input) { i.Summary = strings.Repeat(" ", 30) }, "summary"},
		{"summary too long", func(i *submissions.Input) { i.Summary = strings.Repeat("x", 2001) }, "summary"},
		{"summary max in runes", func(i *submissions.Input) { i.Summary = strings.Repeat("ж", 2000) }, ""},
		{"summary over max in runes", func(i *submissions.Input) { i.Summary = strings.Repeat("ж", 2001) }, "summary"},
		{"repo and sha", func(i *submissions.Input) { i.RepoURL = "https://github.com/owner/repo"; i.CommitSHA = sha }, ""},
		{"repo trailing slash", func(i *submissions.Input) { i.RepoURL = "https://github.com/owner/my.repo-1/" }, ""},
		{"repo not github", func(i *submissions.Input) { i.RepoURL = "https://gitlab.com/owner/repo" }, "repo_url"},
		{"repo is a pull request", func(i *submissions.Input) { i.RepoURL = "https://github.com/o/r/pull/1" }, "repo_url"},
		{"repo http", func(i *submissions.Input) { i.RepoURL = "http://github.com/owner/repo" }, "repo_url"},
		{"repo lookalike host", func(i *submissions.Input) { i.RepoURL = "https://github.com.evil.io/o/r" }, "repo_url"},
		{"sha without repo", func(i *submissions.Input) { i.CommitSHA = sha }, "commit_sha"},
		{"sha too short", func(i *submissions.Input) { i.RepoURL = "https://github.com/o/r"; i.CommitSHA = "abc123" }, "commit_sha"},
		{"sha uppercase", func(i *submissions.Input) {
			i.RepoURL = "https://github.com/o/r"
			i.CommitSHA = strings.Repeat("A", 40)
		}, "commit_sha"},
		{"notes at limit", func(i *submissions.Input) { i.Notes = strings.Repeat("n", 2000) }, ""},
		{"notes too long", func(i *submissions.Input) { i.Notes = strings.Repeat("n", 2001) }, "notes"},
		{"cost ok", func(i *submissions.Input) { i.Cost = &submissions.Cost{USD: 1.42, Source: "claude-code-cli"} }, ""},
		{"cost zero", func(i *submissions.Input) { i.Cost = &submissions.Cost{USD: 0, Source: "agent-reported"} }, ""},
		{"cost negative", func(i *submissions.Input) { i.Cost = &submissions.Cost{USD: -1, Source: "x"} }, "cost.usd"},
		{"cost huge", func(i *submissions.Input) { i.Cost = &submissions.Cost{USD: 10001, Source: "x"} }, "cost.usd"},
		{"cost NaN", func(i *submissions.Input) { i.Cost = &submissions.Cost{USD: math.NaN(), Source: "x"} }, "cost.usd"},
		{"cost infinite", func(i *submissions.Input) { i.Cost = &submissions.Cost{USD: math.Inf(1), Source: "x"} }, "cost.usd"},
		{"cost without source", func(i *submissions.Input) { i.Cost = &submissions.Cost{USD: 1} }, "cost.source"},
		{"cost source too long", func(i *submissions.Input) { i.Cost = &submissions.Cost{USD: 1, Source: strings.Repeat("s", 41)} }, "cost.source"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := ok()
			c.edit(&in)
			err := in.Validate(false)
			if c.field == "" {
				if err != nil {
					t.Fatalf("must be accepted: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("must be rejected on %q", c.field)
			}
			if got := fieldOf(t, err); got != c.field {
				t.Fatalf("rejected on %q, want %q", got, c.field)
			}
		})
	}
}
