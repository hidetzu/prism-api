package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClientIP covers every extraction path the helper supports, with
// particular attention to IPv6 edge cases that were flagged as untested
// in the v0.2.0 release review.
func TestClientIP(t *testing.T) {
	cases := []struct {
		name       string
		xff        string // X-Forwarded-For header; empty means header not set
		remoteAddr string
		want       string
	}{
		// X-Forwarded-For path (preferred when present)
		{
			name:       "xff single ipv4",
			xff:        "203.0.113.5",
			remoteAddr: "192.0.2.1:80",
			want:       "203.0.113.5",
		},
		{
			name:       "xff single ipv6",
			xff:        "2001:db8::1",
			remoteAddr: "192.0.2.1:80",
			want:       "2001:db8::1",
		},
		{
			name:       "xff multi ipv4",
			xff:        "203.0.113.5, 10.0.0.1",
			remoteAddr: "192.0.2.1:80",
			want:       "203.0.113.5",
		},
		{
			name:       "xff multi ipv6",
			xff:        "2001:db8::1, 10.0.0.1",
			remoteAddr: "192.0.2.1:80",
			want:       "2001:db8::1",
		},
		{
			name:       "xff leading and trailing space",
			xff:        " 203.0.113.5 ",
			remoteAddr: "192.0.2.1:80",
			want:       "203.0.113.5",
		},
		{
			name:       "xff no space after comma",
			xff:        "203.0.113.5,10.0.0.1",
			remoteAddr: "192.0.2.1:80",
			want:       "203.0.113.5",
		},

		// RemoteAddr fallback path (no XFF header)
		{
			name:       "remoteaddr ipv4 with port",
			remoteAddr: "203.0.113.5:1234",
			want:       "203.0.113.5",
		},
		{
			name:       "remoteaddr ipv6 bracketed with port",
			remoteAddr: "[2001:db8::1]:1234",
			want:       "2001:db8::1",
		},
		{
			name:       "remoteaddr bare ipv4 without port falls through",
			remoteAddr: "203.0.113.5",
			want:       "203.0.113.5",
		},
		{
			name:       "remoteaddr bare ipv6 without port falls through",
			remoteAddr: "2001:db8::1",
			want:       "2001:db8::1",
		},
		{
			name:       "remoteaddr empty",
			remoteAddr: "",
			want:       "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			req.RemoteAddr = tc.remoteAddr
			if got := clientIP(req); got != tc.want {
				t.Errorf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}
