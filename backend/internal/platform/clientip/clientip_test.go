package clientip

import (
	"net/http/httptest"
	"testing"
)

func TestFromRequest(t *testing.T) {
	tests := []struct {
		name       string
		trustProxy bool
		realIP     string
		remoteAddr string
		want       string
	}{
		{"trusted, header present", true, "192.0.2.1", "10.0.0.5:12345", "192.0.2.1"},
		{"trusted, header absent falls back", true, "", "192.0.2.1:54321", "192.0.2.1"},
		{"trusted, header unparseable falls back", true, "not-an-ip", "192.0.2.1:54321", "192.0.2.1"},
		{"untrusted, header ignored even if present", false, "9.9.9.9", "192.0.2.1:54321", "192.0.2.1"},
		{"invalid RemoteAddr returned as-is", false, "", "invalid", "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/x", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}
			if got := FromRequest(req, tt.trustProxy); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
