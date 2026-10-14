package repoconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Setting a value changes its bytes and nothing else: comments, order, other
// keys and quoting survive; a missing section is appended; what anfra will not
// guess at is refused with the line to write.
func TestEditConfigString(t *testing.T) {
	const url = "https://bi.example.com/o/acme"
	for name, tc := range map[string]struct{ in, want string }{
		"no file":          {"", "# This repo's anfra settings: committed, and free of credentials.\norg:\n  url: " + url + "\n"},
		"comments only":    {"# mine\n", "# mine\norg:\n  url: " + url + "\n"},
		"no section":       {"apps:\n    dir: dash # four spaces\n", "apps:\n    dir: dash # four spaces\norg:\n  url: " + url + "\n"},
		"no final newline": {"apps:\n  dir: dash", "apps:\n  dir: dash\norg:\n  url: " + url + "\n"},
		"plain": {"# top\norg:\n  url: https://old.example.com/o/x # where\nother: 1\n",
			"# top\norg:\n  url: " + url + " # where\nother: 1\n"},
		"double quoted":   {"org:\n    url: \"https://old.example.com/o/x\\u0020\" # q\n", "org:\n    url: " + url + " # q\n"},
		"flow section":    {"org: {}\n", ""},
		"single quotes":   {"org:\n  url: 'https://o.example.com/o/it''s'\n  more: x\n", "org:\n  url: " + url + "\n  more: x\n"},
		"after unicode":   {"# café\norg:\n  url: https://old.example.com/o/x\n", "# café\norg:\n  url: " + url + "\n"},
		"crlf":            {"org:\r\n  url: https://old.example.com/o/x\r\n", "org:\r\n  url: " + url + "\r\n"},
		"section, no key": {"org:\n  other: 1\n", ""},
		"null value":      {"org:\n  url:\n", ""},
		"multi-line":      {"org:\n  url: >\n    https://old\n", ""},
		"flow top level":  {"{org: {url: x}}\n", ""},
		"not a mapping":   {"- a\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := editString([]byte(tc.in), "org", "url", url)
			if tc.want == "" {
				if err == nil {
					t.Errorf("edited to %q, want a refusal", got)
				}
				return
			}
			if err != nil || string(got) != tc.want {
				t.Errorf("got %q, %v\nwant %q", got, err, tc.want)
			}
			if err == nil {
				if err := checkString(got, "org", "url", url); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

// A refused edit names the line to write, and changes nothing; a held lock
// refuses, as git's does.
func TestSetConfigStringRefusesSafely(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("org:\n  other: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := SetString(path, "org", "url", "https://bi.example.com/o/acme")
	if err == nil || !strings.Contains(err.Error(), "url: https://bi.example.com/o/acme") {
		t.Errorf("err = %v, want the line to write by hand", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "org:\n  other: 1\n" {
		t.Errorf("a refused edit changed the file: %q", b)
	}

	if err := os.WriteFile(path+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetString(path, "org", "url", "https://bi.example.com/o/acme"); err == nil || !strings.Contains(err.Error(), ".lock exists") {
		t.Errorf("err = %v, want the held lock refused", err)
	}
}
