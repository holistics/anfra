package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeHTTPAddr(t *testing.T) {
	cases := map[string]string{
		"8080":           "127.0.0.1:8080",
		":8080":          "127.0.0.1:8080",
		":0":             "127.0.0.1:0",
		"0.0.0.0:8080":   "0.0.0.0:8080",
		"localhost:8080": "localhost:8080",
		"[::1]:8080":     "[::1]:8080",
	}
	for in, want := range cases {
		got, err := normalizeHTTPAddr(in)
		if err != nil {
			t.Errorf("normalizeHTTPAddr(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeHTTPAddr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeHTTPAddrRejectsInvalid(t *testing.T) {
	for _, in := range []string{"", "localhost", "127.0.0.1:", "a:b:c"} {
		if got, err := normalizeHTTPAddr(in); err == nil {
			t.Errorf("normalizeHTTPAddr(%q) = %q, want error", in, got)
		}
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true,
		"LOCALHOST": true,
		"127.0.0.1": true,
		"127.1.2.3": true,
		"::1":       true,
		"0.0.0.0":   false,
		"10.0.0.5":  false,
		"evil.com":  false,
	} {
		if got := isLoopbackHost(host); got != want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}

// guardedStatus sends one request through httpGuard (bound to boundAddr) and
// returns the status code; the inner handler answers 200.
func guardedStatus(t *testing.T, boundAddr, method, path, host, contentType string) int {
	t.Helper()
	bound, err := net.ResolveTCPAddr("tcp", boundAddr)
	if err != nil {
		t.Fatalf("resolve %q: %v", boundAddr, err)
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(method, path, strings.NewReader(`{"command":"version"}`))
	req.Host = host
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	httpGuard(bound, ok).ServeHTTP(rec, req)
	return rec.Code
}

func TestHTTPGuardContentType(t *testing.T) {
	const bound = "127.0.0.1:8080"
	cases := []struct {
		contentType string
		want        int
	}{
		{"application/json", http.StatusOK},
		{"application/json; charset=utf-8", http.StatusOK},
		{"", http.StatusUnsupportedMediaType},
		{"text/plain", http.StatusUnsupportedMediaType}, // a CORS "simple" request
		{"application/x-www-form-urlencoded", http.StatusUnsupportedMediaType},
	}
	for _, c := range cases {
		if got := guardedStatus(t, bound, http.MethodPost, "/call", "127.0.0.1:8080", c.contentType); got != c.want {
			t.Errorf("POST /call Content-Type %q: status %d, want %d", c.contentType, got, c.want)
		}
	}
	// Only POST /call needs JSON; /health is a plain GET.
	if got := guardedStatus(t, bound, http.MethodGet, "/health", "127.0.0.1:8080", ""); got != http.StatusOK {
		t.Errorf("GET /health: status %d, want 200", got)
	}
}

func TestHTTPGuardHost(t *testing.T) {
	cases := []struct {
		bound, host string
		want        int
	}{
		{"127.0.0.1:8080", "127.0.0.1:8080", http.StatusOK},
		{"127.0.0.1:8080", "localhost:8080", http.StatusOK},
		{"127.0.0.1:8080", "[::1]:8080", http.StatusOK},
		{"127.0.0.1:8080", "localhost", http.StatusOK},
		{"127.0.0.1:8080", "evil.example:8080", http.StatusForbidden}, // DNS rebinding
		{"127.0.0.1:8080", "10.0.0.5:8080", http.StatusForbidden},
		{"10.0.0.5:8080", "10.0.0.5:8080", http.StatusOK}, // a specific bound IP is allowed
		{"10.0.0.5:8080", "localhost:8080", http.StatusOK},
		{"10.0.0.5:8080", "evil.example:8080", http.StatusForbidden},
		{"0.0.0.0:8080", "172.20.1.2:8080", http.StatusOK}, // wildcard bind skips the check
		{"0.0.0.0:8080", "evil.example:8080", http.StatusOK},
	}
	for _, c := range cases {
		for _, req := range []struct{ method, path, ct string }{
			{http.MethodPost, "/call", "application/json"},
			{http.MethodGet, "/health", ""},
		} {
			if got := guardedStatus(t, c.bound, req.method, req.path, c.host, req.ct); got != c.want {
				t.Errorf("bound %s, %s %s Host %q: status %d, want %d", c.bound, req.method, req.path, c.host, got, c.want)
			}
		}
	}
}

func TestRunServeFailsFastWhenHTTPPortTaken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	// No sidecar binaries: if serve started the sidecars before binding, the error
	// would be about anfra-node instead of the port.
	t.Setenv("ANFRA_NODE_BIN", "")
	t.Setenv("ANFRA_CANAL_QUERY_BIN", "")

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()

	err = runServe(context.Background(), taken.Addr().String())
	if err == nil || !strings.Contains(err.Error(), "listen on --http") {
		t.Fatalf("runServe error = %v, want a --http listen error", err)
	}
}
