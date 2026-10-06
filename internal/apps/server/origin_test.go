package server

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestShellOrigin(t *testing.T) {
	cases := []struct {
		name, proto string
		tls         bool
		want        string
	}{
		{"plain", "", false, "http://demo.example.com"},
		{"direct TLS", "", true, "https://demo.example.com"},
		{"TLS-terminating proxy", "https", false, "https://demo.example.com"},
		{"proxy chain", "https, http", false, "https://demo.example.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/_anfra/data-apps/x.html", nil)
			r.Host = "demo.example.com"
			if c.proto != "" {
				r.Header.Set("X-Forwarded-Proto", c.proto)
			}
			if c.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if got := shellOrigin(r); got != c.want {
				t.Fatalf("shellOrigin = %q, want %q", got, c.want)
			}
		})
	}
}
