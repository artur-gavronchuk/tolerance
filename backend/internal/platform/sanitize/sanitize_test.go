package sanitize

import (
	"strings"
	"testing"
)

func TestCleanText_RedactsSecretsStripsANSIAndTruncates(t *testing.T) {
	in := "\x1b[32mok\x1b[0m token=abc123def456ghi789 and sk-ABCDEFGHIJKLMNOP123 done"
	got := CleanText(in, 200)
	if strings.Contains(got, "\x1b") || strings.Contains(got, "abc123def456") || strings.Contains(got, "sk-ABCDEF") {
		t.Fatalf("not sanitized: %q", got)
	}
	if !strings.HasPrefix(got, "ok ") {
		t.Fatalf("expected ANSI stripped, got %q", got)
	}
	long := CleanText(strings.Repeat("a", 500), 200)
	if len([]rune(long)) != 200 {
		t.Fatalf("expected truncation to 200 runes, got %d", len([]rune(long)))
	}
}

func TestCleanLog_KeepsLinesRedactsAndKeepsTail(t *testing.T) {
	in := "line1\nAuthorization: Bearer eyJabcdefghij.abcdefghijklmnop.sig\nline3\n"
	got := CleanLog(in, 1<<20)
	if strings.Count(got, "\n") != 3 || strings.Contains(got, "eyJabcdefghij") {
		t.Fatalf("unexpected: %q", got)
	}
	tail := CleanLog(strings.Repeat("x", 100)+"\nEND", 10)
	if !strings.HasSuffix(tail, "END") || len(tail) > 10 {
		t.Fatalf("expected the last 10 bytes, got %q", tail)
	}
}

func TestCleanText_RedactsHexSecretWithDigitAnywhere(t *testing.T) {
	// Regression test: digit in the middle, not at position 0 — this is the case
	// where the first fix attempt (position-0-only regex) silently failed to redact.
	secret := "abcdefabcdefabcdefabcdef1abcdefa" // 32 hex chars with digit at position 24 (middle-ish)
	if len(secret) != 32 {
		t.Fatalf("test setup: secret must be exactly 32 chars, got %d", len(secret))
	}
	got := CleanText("prefix "+secret+" suffix", 200)
	if strings.Contains(got, secret) {
		t.Fatalf("32-char hex secret with a digit not at position 0 was not redacted: %q", got)
	}
}
