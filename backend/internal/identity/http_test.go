package identity

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP_XForwardedFor(t *testing.T) {
	tests := []struct {
		name        string
		xff         string
		remoteAddr  string
		expectedIP  string
		description string
	}{
		{
			name:        "single IP in X-Forwarded-For",
			xff:         "192.0.2.1",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "192.0.2.1",
			description: "single IP should be used",
		},
		{
			name:        "multiple IPs - uses last",
			xff:         "1.2.3.4, 10.0.0.5",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "10.0.0.5",
			description: "last IP (from reverse proxy) should be used, not attacker prefix",
		},
		{
			name:        "attacker tries different prefixes - still same bucket",
			xff:         "9.9.9.9, 10.0.0.5",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "10.0.0.5",
			description: "attacker-controlled prefix should be ignored, last hop should be used",
		},
		{
			name:        "many IPs in chain",
			xff:         "1.1.1.1, 2.2.2.2, 3.3.3.3, 10.0.0.5",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "10.0.0.5",
			description: "last IP in chain should be used",
		},
		{
			name:        "whitespace handling",
			xff:         "1.2.3.4 , 10.0.0.5 ",
			remoteAddr:  "10.0.0.5:12345",
			expectedIP:  "10.0.0.5",
			description: "whitespace should be trimmed from last IP",
		},
		{
			name:        "no X-Forwarded-For uses RemoteAddr",
			xff:         "",
			remoteAddr:  "192.0.2.1:54321",
			expectedIP:  "192.0.2.1",
			description: "without header, IP from RemoteAddr should be used",
		},
		{
			name:        "no X-Forwarded-For with invalid RemoteAddr",
			xff:         "",
			remoteAddr:  "invalid",
			expectedIP:  "invalid",
			description: "invalid RemoteAddr should be returned as-is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}

			got := clientIP(req)
			if got != tt.expectedIP {
				t.Fatalf("got %q, want %q (%s)", got, tt.expectedIP, tt.description)
			}
		})
	}
}
