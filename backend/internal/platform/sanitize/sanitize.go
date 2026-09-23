package sanitize

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	redacted = "[redacted]"
)

// The connector applies the same rules before sending; the server repeats
// them because it must not trust its clients with what gets published.
var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

	secretPatterns = []*regexp.Regexp{
		// JWTs.
		regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}(?:\.[A-Za-z0-9_-]*)?`),
		// Vendor-prefixed keys: sk-…, ak_… (our own), ghp_…, xoxb-…, AKIA_… and so on.
		regexp.MustCompile(`(?i)\b(?:sk|pk|ak|ghp|gho|ghs|ghu|ghr|github_pat|xox[abprs]|AKIA|ASIA)[-_][A-Za-z0-9_\-]{8,}`),
		// Raw AWS access key ids.
		regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
		// Authorization headers.
		regexp.MustCompile(`(?i)\bbearer\s+\S+`),
		// key=value and key: value where the key names a secret.
		regexp.MustCompile(`(?i)\b[a-z0-9_]*(?:token|secret|password|passwd|api[_-]?key|access[_-]?key|private[_-]?key)[a-z0-9_]*\s*[:=]\s*\S+`),
		// Long hex strings (hashes, keys) - requires at least one digit to avoid false positives.
		regexp.MustCompile(`\b[A-Fa-f0-9]*[0-9][A-Fa-f0-9]{31,}\b`),
		// PEM blocks: the header and everything after it.
		regexp.MustCompile(`-----BEGIN [A-Z ]+-----.*`),
	}

	// longTokenRe finds base64-ish blobs. Ordinary long file paths match it
	// too, so a candidate is redacted only when it mixes letters and digits.
	longTokenRe = regexp.MustCompile(`[A-Za-z0-9+/_\-]{40,}={0,2}`)
)

func looksLikeToken(s string) bool {
	var digit, letter bool
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			letter = true
		}
	}
	return digit && letter
}

// CleanText prepares free text from a client for storage and display: it
// strips ANSI sequences and control characters, collapses whitespace,
// redacts anything shaped like a secret and truncates to maxRunes.
// Redaction runs before truncation so a secret cut by the limit cannot
// leak its head. An empty result means the text should be dropped.
func CleanText(s string, maxRunes int) string {
	s = strings.ToValidUTF8(s, "")
	s = ansiRe.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t' || r == '\r' || r == '\v' || r == '\f':
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, redacted)
	}
	s = longTokenRe.ReplaceAllStringFunc(s, func(m string) string {
		if looksLikeToken(m) {
			return redacted
		}
		return m
	})
	if utf8.RuneCountInString(s) > maxRunes {
		s = string([]rune(s)[:maxRunes])
	}
	return strings.TrimSpace(s)
}

// CleanLog is CleanText for multi-line output: ANSI stripped, secrets
// redacted per line, newlines kept, and only the last maxBytes returned
// (the end of a log is where the failure is).
func CleanLog(s string, maxBytes int) string {
	s = strings.ToValidUTF8(s, "")
	s = ansiRe.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		line = strings.Map(func(r rune) rune {
			if r == '\t' {
				return r
			}
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return -1
			}
			return r
		}, line)
		for _, re := range secretPatterns {
			line = re.ReplaceAllString(line, redacted)
		}
		lines[i] = longTokenRe.ReplaceAllStringFunc(line, func(m string) string {
			if looksLikeToken(m) {
				return redacted
			}
			return m
		})
	}
	s = strings.Join(lines, "\n")
	if len(s) > maxBytes {
		s = s[len(s)-maxBytes:]
		s = strings.ToValidUTF8(s, "")
	}
	return s
}
