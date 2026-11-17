package repoconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// read writes yml as a repo's settings file and reads it.
func read(t *testing.T, yml string) (Config, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".anfra"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	return Read(dir)
}

// Every setting reads back; unset, each has anfra's default.
func TestReadQuerySettings(t *testing.T) {
	c, err := read(t, `query:
  default:
    input:
      timezone: Asia/Ho_Chi_Minh
      weekStartDay: sunday
    cache:
      duration: 10m
  locked_defaults: [input.weekStartDay]
  cache:
    enabled: false
    max_duration: 1h
`)
	if err != nil {
		t.Fatal(err)
	}
	if c.QueryCacheEnabled() || c.QueryCacheSettings().MaxDuration != time.Hour || c.QueryDefaults().CacheDuration != 10*time.Minute ||
		c.Query.Default.Input.Timezone != "Asia/Ho_Chi_Minh" || c.Query.Default.Input.WeekStartDay != "sunday" {
		t.Errorf("read %+v", c.Query)
	}

	for _, yml := range []string{"", "# only a comment\n"} {
		c, err := read(t, yml)
		if err != nil || !c.QueryCacheEnabled() || c.QueryCacheSettings().MaxDuration != 0 || c.QueryDefaults().CacheDuration != 0 {
			t.Errorf("%q: %+v, %v; want the defaults", yml, c.Query, err)
		}
	}
}

// A key anfra doesn't know is refused, with its line and the key meant, or the
// keys there are; so is a value it doesn't take, or settings that contradict
// each other. Each error names the setting.
func TestReadRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ yml, want string }{
		"snake_case under input": {"query:\n  default:\n    input:\n      week_start_day: monday\n",
			".anfra/config.yml line 4: query.default.input.week_start_day isn't a setting. Did you mean weekStartDay?"},
		"camelCase outside input": {"query:\n  cache:\n    maxDuration: 1h\n", "query.cache.maxDuration isn't a setting. Did you mean max_duration?"},
		"a typo":                  {"query:\n  cache:\n    enabeld: false\n", "line 3: query.cache.enabeld isn't a setting. Did you mean enabled?"},
		"no default for it": {"query:\n  default:\n    input:\n      pagination: {page: 1}\n",
			"query.default.input.pagination isn't a setting. The settings under query.default.input are timezone and weekStartDay. If a newer anfra added it, update anfra (`anfra update`)."},
		"an unknown section": {"queries: {}\n", "The settings at the top level are org, apps, and query."},
		"the old key":        {"cache:\n  query:\n    enabled: false\n", "line 1: cache isn't a setting."},
		"not a duration":     {"query:\n  default:\n    cache:\n      duration: 1 hour\n", `.anfra/config.yml: query.default.cache.duration is "1 hour", which isn't a duration.`},
		"a negative default": {"query:\n  default:\n    cache:\n      duration: -1m\n", "is -1m, which is negative"},
		"over the cap":       {"query:\n  default:\n    cache:\n      duration: 2h\n  cache:\n    max_duration: 1h\n", "is 2h, more than query.cache.max_duration (1h). Lower the default or raise the cap"},
		"over 7 days":        {"query:\n  default:\n    cache:\n      duration: 200h\n", "more than query.cache.max_duration (168h, by default)"},
		"a cap of 0s":        {"query:\n  cache:\n    max_duration: 0s\n", "To turn the query cache off, set query.cache.enabled to false"},
		"a cap over 7 days":  {"query:\n  cache:\n    max_duration: 200h\n", "anfra caches a result for at most 168h (7 days)"},
		"a time zone":        {"query:\n  default:\n    input:\n      timezone: Mars/Olympus\n", `"Mars/Olympus", which isn't an IANA time zone`},
		"the local zone":     {"query:\n  default:\n    input:\n      timezone: Local\n", "isn't an IANA time zone"},
		"a week start day":   {"query:\n  default:\n    input:\n      weekStartDay: Monday\n", `is "Monday". Did you mean "monday"?`},
		"not a day":          {"query:\n  default:\n    input:\n      weekStartDay: weekend\n", `is "weekend". Use one of: "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", or "sunday"`},
		"a lock on nothing set": {"query:\n  locked_defaults: [input.timezone]\n",
			"names input.timezone, which query.default doesn't set. Set query.default.input.timezone, or remove it from query.locked_defaults"},
		"a lock on an object": {"query:\n  default:\n    input:\n      timezone: Etc/UTC\n  locked_defaults: [input]\n",
			"names input, which has fields that can't have a default. Lock its fields one by one, such as input.timezone and input.weekStartDay"},
		"a lock on no default": {"query:\n  locked_defaults: [input.pagination]\n",
			"names input.pagination, which isn't a default. The defaults are input.timezone, input.weekStartDay, and cache.duration"},
		"a lock on a cache turned off": {"query:\n  default:\n    cache:\n      duration: 5m\n  locked_defaults: [cache.duration]\n  cache:\n    enabled: false\n",
			"locks cache.duration, but query.cache.enabled is false"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := read(t, tc.yml); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v\nwant one containing %q", err, tc.want)
			}
		})
	}
}
