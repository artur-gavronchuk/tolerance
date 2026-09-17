// Package httpx provides the shared HTTP plumbing every module's handlers
// use: problem responses, request ids, and a strict JSON decoder. Nothing
// here knows about any specific domain module.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Problem is the error body shape used across the whole API.
type Problem struct {
	Status    int          `json:"-"`
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id,omitempty"`
	Fields    []FieldError `json:"fields,omitempty"`
}

type FieldError struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

func (p *Problem) Error() string { return p.Message }

func New(status int, code, message string) *Problem {
	return &Problem{Status: status, Code: code, Message: message}
}

func WithField(status int, code, message, path, fieldCode string) *Problem {
	return &Problem{Status: status, Code: code, Message: message, Fields: []FieldError{{Path: path, Code: fieldCode}}}
}

func Unauthenticated(message string) *Problem {
	return New(http.StatusUnauthorized, "unauthenticated", message)
}
func Forbidden(message string) *Problem { return New(http.StatusForbidden, "forbidden", message) }
func NotFound() *Problem {
	return New(http.StatusNotFound, "not_found", "Not found")
}
func StateConflict(message string) *Problem {
	return New(http.StatusConflict, "state_conflict", message)
}
func InvalidBody(message string) *Problem {
	return New(http.StatusUnprocessableEntity, "invalid_body", message)
}
func Internal() *Problem {
	return New(http.StatusInternalServerError, "internal_error", "Internal error")
}

// WriteError writes err as a problem response. Errors that are not *Problem
// are logged by the caller and reported to the client as internal_error,
// never leaking their text.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var p *Problem
	if !errors.As(err, &p) {
		p = Internal()
	}
	p.RequestID = RequestID(r.Context())
	Respond(w, p.Status, p)
}

type requestIDKey struct{}

// WithRequestID middleware assigns a request id used for log correlation.
// It does not depend on any specific id format; callers may supply their
// own generator.
func WithRequestID(gen func() string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = gen()
			}
			w.Header().Set("X-Request-Id", id)
			next.ServeHTTP(w, r.WithContext(withRequestID(r.Context(), id)))
		})
	}
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the id assigned to this request, or "" if none.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func Respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Decode reads exactly one JSON object into dst, rejecting unknown fields
// and trailing data. It never accepts a stream of JSON values.
func Decode(raw []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return InvalidBody("Invalid JSON or unknown field")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return InvalidBody("Expected a single JSON object")
	}
	return nil
}
