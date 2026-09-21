package attempts_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"tolerance/internal/attempts"
)

func TestCleanText(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	cases := []struct {
		name      string
		in        string
		want      string   // exact expected output when non-empty
		mustHave  []string // substrings the output must contain
		mustNot   []string // substrings the output must not contain
		wantEmpty bool
	}{
		{name: "plain progress text is kept", in: "Editing src/plan.ts", want: "Editing src/plan.ts"},
		{name: "anthropic style key assignment", in: "export ANTHROPIC_API_KEY=sk-ant-abc123def456",
			mustHave: []string{"[redacted]"}, mustNot: []string{"sk-ant", "abc123def456"}},
		{name: "openai style key", in: "using sk-proj-AbCdEfGhIjKlMnOp now",
			mustHave: []string{"[redacted]"}, mustNot: []string{"AbCdEfGhIjKlMnOp"}},
		{name: "github token", in: "push with ghp_1234567890abcdefghij",
			mustHave: []string{"[redacted]"}, mustNot: []string{"1234567890abcdefghij"}},
		{name: "aws access key id", in: "key AKIA_ABCDEFGH12345678 found",
			mustHave: []string{"[redacted]"}, mustNot: []string{"ABCDEFGH12345678"}},
		{name: "authorization bearer header", in: "Authorization: Bearer " + jwt,
			mustHave: []string{"[redacted]"}, mustNot: []string{"eyJhbGci", "dozjgNry"}},
		{name: "short bearer token", in: "curl -H 'Bearer abc123' x",
			mustHave: []string{"[redacted]"}, mustNot: []string{"abc123"}},
		{name: "password assignment", in: "password=hunter2 in config",
			mustHave: []string{"[redacted]"}, mustNot: []string{"hunter2"}},
		{name: "token colon value", in: "token: s3cr3t-value",
			mustHave: []string{"[redacted]"}, mustNot: []string{"s3cr3t-value"}},
		{name: "40 hex chars (a commit sha or a secret)", in: "commit 3f9a2b1c0d5e6f708192a3b4c5d6e7f801234567 done",
			mustHave: []string{"[redacted]"}, mustNot: []string{"3f9a2b1c0d5e6f70"}},
		{name: "long base64 blob", in: "blob dGhpcyBpcyBhIHZlcnkgbG9uZyBiYXNlNjQgc2VjcmV0IHZhbHVlIQ== end",
			mustHave: []string{"[redacted]"}, mustNot: []string{"dGhpcyBpcyBhIHZlcnkg"}},
		{name: "private key header", in: "-----BEGIN RSA PRIVATE KEY----- MIIEow",
			mustHave: []string{"[redacted]"}, mustNot: []string{"BEGIN RSA"}},
		{name: "long path without digits is not a secret", in: "Read src/components/dashboard/widgets/analytics/charts/BarChart",
			want: "Read src/components/dashboard/widgets/analytics/charts/BarChart"},
		{name: "ansi colours are stripped", in: "\x1b[31mred\x1b[0m alert", want: "red alert"},
		{name: "control characters and newlines collapse", in: "line one\n\tline\x00 two\r", want: "line one line two"},
		{name: "bell and escape are removed", in: "a\x07b\x1bc", want: "abc"},
		{name: "whitespace is trimmed", in: "   hello   world  ", want: "hello world"},
		{name: "only control characters", in: "\x00\x01\x02", wantEmpty: true},
		{name: "empty", in: "", wantEmpty: true},
		{name: "unicode is kept", in: "Строим маршрут ✓", want: "Строим маршрут ✓"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := attempts.CleanText(c.in)
			if c.wantEmpty {
				if got != "" {
					t.Fatalf("want empty, got %q", got)
				}
				return
			}
			if c.want != "" && got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			for _, s := range c.mustHave {
				if !strings.Contains(got, s) {
					t.Errorf("output %q must contain %q", got, s)
				}
			}
			for _, s := range c.mustNot {
				if strings.Contains(got, s) {
					t.Errorf("output %q must not contain %q", got, s)
				}
			}
		})
	}
}

func TestCleanText_TruncatesToTwoHundredRunes(t *testing.T) {
	got := attempts.CleanText(strings.Repeat("ж", 500))
	if n := utf8.RuneCountInString(got); n != 200 {
		t.Fatalf("want 200 runes, got %d", n)
	}
	if !utf8.ValidString(got) {
		t.Fatal("truncation must not split a rune")
	}
	got = attempts.CleanText(strings.Repeat("word ", 100))
	if utf8.RuneCountInString(got) > 200 {
		t.Fatalf("too long: %d", utf8.RuneCountInString(got))
	}
}

func TestCleanText_RedactionSurvivesTruncation(t *testing.T) {
	// A secret that straddles the 200-rune cut must not leak its head.
	in := strings.Repeat("x ", 97) + "sk-ant-abcdefghijklmnop123456"
	got := attempts.CleanText(in)
	if strings.Contains(got, "sk-ant") || strings.Contains(got, "abcdef") {
		t.Fatalf("secret leaked through truncation: %q", got)
	}
}
