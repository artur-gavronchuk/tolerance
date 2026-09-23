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
