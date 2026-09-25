package match

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestReadLineDeliversEmptyLines guards against a real bug: readLine used to return a nil line for both
// "no more input" and "the line was empty", so readStdout's `if line != nil` check silently dropped blank
// stdout lines — a bot flooding bare newlines never reached Lines(), never counted toward noise, and
// never tripped MaxNoise. An empty line must come back as a non-nil, zero-length slice with a nil error,
// distinguishable from true end of input (nil line, non-nil err).
func TestReadLineDeliversEmptyLines(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\nhello\n"))

	line, err := readLine(r, maxLineBytes)
	if err != nil {
		t.Fatalf("first readLine: unexpected err %v", err)
	}
	if line == nil {
		t.Fatalf("first line = nil, want a non-nil empty line")
	}
	if len(line) != 0 {
		t.Fatalf("first line = %q, want empty", line)
	}

	line, err = readLine(r, maxLineBytes)
	if err != nil {
		t.Fatalf("second readLine: unexpected err %v", err)
	}
	if string(line) != "hello" {
		t.Fatalf("second line = %q, want %q", line, "hello")
	}

	line, err = readLine(r, maxLineBytes)
	if line != nil {
		t.Fatalf("third line = %q, want nil once input is exhausted", line)
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("third err = %v, want io.EOF", err)
	}
}
