package agents

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateRunes(t *testing.T) {
	long := strings.Repeat("модель-", 20) // multibyte, well over 80 runes
	got := truncateRunes(long, 80)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 80 {
		t.Fatalf("got %d runes, valid=%v", utf8.RuneCountInString(got), utf8.ValidString(got))
	}
	if truncateRunes("gpt-5", 80) != "gpt-5" {
		t.Fatal("short string must be unchanged")
	}
	cut := truncateRunes(strings.Repeat("a", 79)+"é"+"tail", 80)
	if !strings.HasSuffix(cut, "é") || !utf8.ValidString(cut) {
		t.Fatalf("split a rune: %q", cut)
	}
}
