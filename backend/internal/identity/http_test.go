package identity

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP_TrustProxyGatesTheRealIPHeader(t *testing.T) {
	tests := []struct {
		name        string
		trustProxy  bool
		realIP      string
		remoteAddr  string
		expectedIP  string
		description string
	}{
		{
			name:        "trusted proxy sets X-Real-IP",
			trustProxy:  true,
			realIP:      "192.0.2.1",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "192.0.2.1",
			description: "the proxy's resolved client address should be used",
		},
		{
			name:        "trusted proxy but header absent falls back to RemoteAddr",
			trustProxy:  true,
			realIP:      "",
			remoteAddr:  "192.0.2.1:54321",
			expectedIP:  "192.0.2.1",
			description: "without the header, RemoteAddr is used",
		},
		{
			name:        "header ignored when the proxy is not trusted",
			trustProxy:  false,
			realIP:      "9.9.9.9",
			remoteAddr:  "192.0.2.1:54321",
			expectedIP:  "192.0.2.1",
			description: "a client-controlled header must never be trusted without ARENA_TRUST_PROXY",
		},
		{
			name:        "no header, no trust, uses RemoteAddr",
			trustProxy:  false,
			realIP:      "",
			remoteAddr:  "192.0.2.1:54321",
			expectedIP:  "192.0.2.1",
			description: "without header, IP from RemoteAddr should be used",
		},
		{
			name:        "invalid RemoteAddr returned as-is",
			trustProxy:  false,
			realIP:      "",
			remoteAddr:  "invalid",
			expectedIP:  "invalid",
			description: "invalid RemoteAddr should be returned as-is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}

			got := clientIP(req, tt.trustProxy)
			if got != tt.expectedIP {
				t.Fatalf("got %q, want %q (%s)", got, tt.expectedIP, tt.description)
			}
		})
	}
}
