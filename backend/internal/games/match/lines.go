package match

import (
	"bufio"
	"sync"
)

// maxLineBytes is the longest protocol line delivered whole; a longer physical line is truncated to this
// length (the remainder up to the next "\n" is read and discarded, not buffered) so a bot that floods
// stdout without newlines cannot grow a reader's memory unboundedly. The truncated line is still sent on
// — parsing it as a protocol message will fail, which counts as noise like any other malformed line.
const maxLineBytes = 64 * 1024

// readLine reads one newline-terminated line from r, dropping the trailing "\n" and, for CRLF-terminated
// output, the preceding "\r" too. It returns a truncated line (see maxLineBytes) rather than growing
// without bound. A final line with no trailing newline is still returned once, before the error (usually
// io.EOF) that ended the read; after that, err is returned with a nil line.
func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	discarding := false
	for {
		b, err := r.ReadByte()
		if err != nil {
			if len(buf) > 0 {
				return trimCR(buf), nil
			}
			return nil, err
		}
		if b == '\n' {
			return trimCR(buf), nil
		}
		if discarding {
			continue
		}
		if len(buf) >= max {
			discarding = true
			continue
		}
		buf = append(buf, b)
	}
}

func trimCR(line []byte) []byte {
	if n := len(line); n > 0 && line[n-1] == '\r' {
		return line[:n-1]
	}
	return line
}

// tailWriter is an io.Writer that keeps only the last max bytes written to it. Safe for concurrent use: a
// process's stderr is copied into it from one goroutine while Stderr() may be read from another once the
// match is over.
type tailWriter struct {
	max int

	mu  sync.Mutex
	buf []byte
}

func newTailWriter(max int) *tailWriter {
	return &tailWriter{max: max}
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.max {
		w.buf = w.buf[len(w.buf)-w.max:]
	}
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}
