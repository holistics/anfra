package hostedorigin_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the real build validator and linker boundary, not a mutable package variable.
func TestHostedOriginBuildConfiguration(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.MkdirTemp(root, ".hosted-origin-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(fixture) })
	main := `package main
import (
 "fmt"
 "os"
 "github.com/holistics/anfra/internal/hostedorigin"
)
func main() {
 origin, err := hostedorigin.HostedOrigin()
 if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
 fmt.Println(origin)
}
`
	if err := os.WriteFile(filepath.Join(fixture, "main.go"), []byte(main), 0600); err != nil {
		t.Fatal(err)
	}
	build := func(output, target, origin string) {
		t.Helper()
		args := []string{"build", "-o", output}
		if origin != "" {
			args = append(args, "-ldflags", "-X github.com/holistics/anfra/internal/hostedorigin.embeddedOrigin="+origin)
		}
		cmd := exec.Command("go", append(args, target)...) //nolint:gosec // G204: fixed Go build tool and this test's generated program
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
	}
	run := func(binary, origin string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Env = append(os.Environ(), "ANFRA_HOSTED_ORIGIN="+origin)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	validator := filepath.Join(fixture, "validator")
	build(validator, "./scripts/hosted-origin", "")
	for _, tc := range []struct {
		name, raw, want string
		required, valid bool
	}{
		{name: "development missing", valid: true},
		{name: "release missing", required: true},
		{name: "canonical", raw: "HTTPS://BI.Example.COM:443/", want: "https://bi.example.com", required: true, valid: true},
		{name: "custom port", raw: "https://BI.example.com:8443/", want: "https://bi.example.com:8443", required: true, valid: true},
		{name: "ipv6", raw: "https://[::1]:443/", want: "https://[::1]", required: true, valid: true},
		{name: "development invalid", raw: "https://example.com/path"},
		{name: "http loopback", raw: "http://127.0.0.1", required: true},
		{name: "relative", raw: "example.com", required: true},
		{name: "userinfo", raw: "https://user@example.com", required: true},
		{name: "path", raw: "https://example.com/path", required: true},
		{name: "encoded slash", raw: "https://example.com/%2f", required: true},
		{name: "query", raw: "https://example.com?x=1", required: true},
		{name: "empty query", raw: "https://example.com?", required: true},
		{name: "fragment", raw: "https://example.com#x", required: true},
		{name: "empty fragment", raw: "https://example.com#", required: true},
		{name: "empty port", raw: "https://example.com:", required: true},
		{name: "bad port", raw: "https://example.com:65536", required: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			if tc.required {
				args = append(args, "--required")
			}
			out, err := run(validator, tc.raw, args...)
			if tc.valid {
				if err != nil || out != tc.want {
					t.Fatalf("got %q, %v; want %q", out, err, tc.want)
				}
			} else if err == nil || !strings.Contains(out, "ANFRA_HOSTED_ORIGIN") {
				t.Fatalf("invalid setting accepted or missing configuration error: %q, %v", out, err)
			}
		})
	}
	for _, tc := range []struct{ name, embedded, want string }{
		{"embedded", "HTTPS://BI.Example.COM:443/", "https://bi.example.com"},
		{"missing", "", "Provide an org URL."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := filepath.Join(fixture, tc.name)
			build(probe, fixture, tc.embedded)
			for _, runtimeOrigin := range []string{"", "https://different.example.com", "invalid"} {
				out, err := run(probe, runtimeOrigin)
				if out != tc.want || (err != nil) != (tc.embedded == "") {
					t.Fatalf("runtime setting %q: got %q, %v; want %q", runtimeOrigin, out, err, tc.want)
				}
			}
		})
	}
}
