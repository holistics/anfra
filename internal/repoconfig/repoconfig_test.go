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

// A refused edit names the value to set, and changes nothing; a held lock
// refuses, as git's does.
func TestSetConfigStringRefusesSafely(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("org:\n  other: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := SetString(path, "org", "url", "https://bi.example.com/o/acme")
	if err == nil || !strings.Contains(err.Error(), "Set org.url to https://bi.example.com/o/acme yourself") {
		t.Errorf("err = %v, want the value to set by hand", err)
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
	if err := SetString(path, "org", "url", "https://bi.example.com/o/acme"); err == nil || !strings.Contains(err.Error(), "it's locked") {
		t.Errorf("err = %v, want the held lock refused", err)
	}
}

// aql.compiler picks the AQL compiler: legacy when unset, and any value but
// legacy or v2 is refused rather than read as one of them.
func TestUseV2Compiler(t *testing.T) {
	for name, tc := range map[string]struct {
		file    string
		want    bool
		wantErr bool
	}{
		"no file":  {"", false, false},
		"unset":    {"org:\n  url: https://bi.example.com/o/acme\n", false, false},
		"legacy":   {"aql:\n  compiler: legacy\n", false, false},
		"v2":       {"aql:\n  compiler: v2\n", true, false},
		"mistyped": {"aql:\n  compiler: V2\n", false, true},
	} {
		dir := t.TempDir()
		if tc.file != "" {
			if err := os.MkdirAll(filepath.Join(dir, ".anfra"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(Path(dir), []byte(tc.file), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		conf, err := Read(dir)
		if err != nil {
			t.Fatalf("%s: read: %v", name, err)
		}
		got, err := conf.UseV2Compiler()
		if (err != nil) != tc.wantErr {
			t.Fatalf("%s: err = %v, want error %v", name, err, tc.wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), "aql.compiler") {
			t.Errorf("%s: error %q does not name aql.compiler", name, err)
		}
		if got != tc.want {
			t.Errorf("%s: UseV2Compiler = %v, want %v", name, got, tc.want)
		}
	}
}
