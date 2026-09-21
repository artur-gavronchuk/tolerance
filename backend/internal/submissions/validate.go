package submissions

import (
	"math"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"tolerance/internal/platform/httpx"
)

const maxURLLen = 2048

var (
	repoRe = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/?$`)
	shaRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)

	cgnat = netip.MustParsePrefix("100.64.0.0/10")

	blockedSuffixes = []string{".internal", ".local", ".localdomain", ".lan", ".home.arpa", ".intranet", ".corp"}
)

func invalid(msg, path, code string) error {
	return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", msg, path, code)
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// Validate checks the result before anything is stored. The URL checks
// only reject the obviously wrong early; the real protection against
// internal targets is the checker's network guard, which decides on the
// address the connection would actually use.
func (in Input) Validate(allowLoopback bool) error {
	if n := runeLen(strings.TrimSpace(in.Summary)); n < 20 || runeLen(in.Summary) > 2000 {
		return invalid("summary must be 20-2000 characters", "summary", "invalid")
	}
	if code := checkPreviewURL(in.PreviewURL, allowLoopback); code != "" {
		return invalid("preview_url must be a public https URL ("+code+")", "preview_url", code)
	}
	if in.RepoURL != "" && (len(in.RepoURL) > maxURLLen || !repoRe.MatchString(in.RepoURL)) {
		return invalid("repo_url must look like https://github.com/{owner}/{repo}", "repo_url", "invalid")
	}
	if in.CommitSHA != "" {
		if !shaRe.MatchString(in.CommitSHA) {
			return invalid("commit_sha must be 40 lowercase hex characters", "commit_sha", "invalid")
		}
		if in.RepoURL == "" {
			return invalid("commit_sha needs a repo_url", "commit_sha", "needs_repo_url")
		}
	}
	if runeLen(in.Notes) > 2000 {
		return invalid("notes must be at most 2000 characters", "notes", "too_long")
	}
	if c := in.Cost; c != nil {
		if math.IsNaN(c.USD) || math.IsInf(c.USD, 0) || c.USD < 0 || c.USD > 10000 {
			return invalid("cost.usd must be between 0 and 10000", "cost.usd", "out_of_range")
		}
		if s := strings.TrimSpace(c.Source); s == "" || runeLen(c.Source) > 40 {
			return invalid("cost.source must name where the figure comes from (1-40 characters)", "cost.source", "invalid")
		}
	}
	return nil
}

// checkPreviewURL returns "" when the URL is acceptable, otherwise a short
// code naming the problem.
func checkPreviewURL(raw string, allowLoopback bool) string {
	if len(raw) > maxURLLen {
		return "too_long"
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "invalid"
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return "invalid"
	}
	if u.User != nil {
		return "credentials"
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return "not_https"
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "invalid"
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.Zone() != "" {
			return "invalid"
		}
		addr = addr.Unmap()
		if addr.IsLoopback() {
			if allowLoopback {
				return ""
			}
			return "private_address"
		}
		if addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() ||
			addr.IsUnspecified() || cgnat.Contains(addr) {
			return "private_address"
		}
		if scheme != "https" {
			return "not_https"
		}
		return ""
	}

	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		if allowLoopback {
			return ""
		}
		return "blocked_host"
	}
	if !strings.Contains(host, ".") || host == "metadata" {
		return "blocked_host" // single-label names are internal
	}
	for _, s := range blockedSuffixes {
		if strings.HasSuffix(host, s) {
			return "blocked_host"
		}
	}
	// A browser reads 0x7f.0.0.1 or 127.1 as an IPv4 address; no real TLD is numeric.
	last := host[strings.LastIndex(host, ".")+1:]
	if last == "" || strings.HasPrefix(last, "0x") || strings.Trim(last, "0123456789") == "" {
		return "blocked_host"
	}
	if scheme != "https" {
		return "not_https"
	}
	return ""
}
