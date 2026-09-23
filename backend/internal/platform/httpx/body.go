package httpx

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// ReadBody reads the request body up to a fixed limit, returning a Problem
// (413) rather than a generic error when it is exceeded.
func ReadBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		return nil, New(http.StatusRequestEntityTooLarge, "body_too_large", "Request body exceeds 1 MiB")
	}
	return raw, nil
}

// ReadBodyLimit is ReadBody with a caller-chosen limit, for the few routes
// that legitimately carry more than 1 MiB (the checker's evidence upload).
func ReadBodyLimit(w http.ResponseWriter, r *http.Request, max int64) ([]byte, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, max))
	if err != nil {
		return nil, New(http.StatusRequestEntityTooLarge, "body_too_large", fmt.Sprintf("Request body exceeds %d MiB", max>>20))
	}
	return raw, nil
}

// ValidText reports whether value, once trimmed, has length in [min, max]
// bytes. Used for the free-text fields (names, briefs, reasons) that appear
// across nearly every module's create/update commands.
func ValidText(value string, min, max int) bool {
	trimmed := strings.TrimSpace(value)
	return len(trimmed) >= min && len(value) <= max
}
