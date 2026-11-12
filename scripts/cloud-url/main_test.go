package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The repo's own manifest.yml is what a release embeds, so CI checks it before a release does.
func TestReleaseManifestIsValid(t *testing.T) {
	if _, _, err := readManifest("../../manifest.yml"); err != nil {
		t.Fatal(err)
	}
}

func TestReadManifest(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, servers, email, wantError string
	}{
		{name: "none", manifest: "version: 1.0.0\n"},
		{
			name: "available and upcoming",
			manifest: `cloud_servers:
  - {id: us, label: United States, url: "https://US.example.com/"}
  - {id: eu, label: Europe}
cloud_support_email: support@example.com
`,
			servers: "United States=https://us.example.com;Europe", email: "support@example.com",
		},
		{name: "a misspelled field", manifest: "cloud_servers:\n  - {id: us, label: US, uri: https://us.example.com}\n", wantError: `unknown field, "uri"`},
		{name: "no id", manifest: "cloud_servers:\n  - {label: US, url: https://us.example.com}\n", wantError: "needs an id"},
		{name: "a repeated id", manifest: "cloud_servers:\n  - {id: us, label: US, url: https://us.example.com}\n  - {id: us, label: Europe}\n", wantError: `the id "us", which an earlier server already has`},
		{name: "no label", manifest: "cloud_servers:\n  - {id: us, url: https://us.example.com}\n", wantError: "needs a label"},
		{name: "a separator in a label", manifest: "cloud_servers:\n  - {id: us, label: US;EU, url: https://us.example.com}\n", wantError: `";" or an "="`},
		{name: "an invalid url", manifest: "cloud_servers:\n  - {id: us, label: US, url: https://us.example.com/path}\n", wantError: "cloud_servers: server 1:"},
		{name: "an invalid email", manifest: "cloud_support_email: Support <support@example.com>\n", wantError: "cloud_support_email:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.yml")
			if err := os.WriteFile(path, []byte(tc.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			servers, email, err := readManifest(path)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil || servers != tc.servers || email != tc.email {
				t.Fatalf("read (%q, %q, %v), want (%q, %q)", servers, email, err, tc.servers, tc.email)
			}
		})
	}
}
