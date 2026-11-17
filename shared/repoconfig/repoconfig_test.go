package repoconfig

import (
	"encoding/json/jsontext"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The settings are Config's fields that aren't mappings, as the design lists
// them (docs/designs/repos/config.md in anfra-cloud).
func TestKeys(t *testing.T) {
	var got []string
	for _, k := range Keys() {
		got = append(got, k.String())
	}
	want := []string{
		"org.url",
		"apps.allow_origins",
		"query.default.input.timezone",
		"query.default.input.weekStartDay",
		"query.default.cache.duration",
		"query.locked_defaults",
		"query.cache.enabled",
		"query.cache.max_duration",
		"aql.compiler.version",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Keys() = %v\nwant %v", got, want)
	}
}

// A key is its names joined with ".": a key covers those under it, name by
// name, and a key with an empty name is refused.
func TestKey(t *testing.T) {
	k := Key("query.cache.max_duration")
	if got := strings.Join(k.Segments(), "|"); got != "query|cache|max_duration" {
		t.Errorf("Segments() = %s", got)
	}
	for prefix, want := range map[Key]bool{"query": true, "query.cache": true, "query.cache.max_duration": true, "quer": false, "query.cache.max": false} {
		if got := k.Within(prefix); got != want {
			t.Errorf("%s within %s = %v, want %v", k, prefix, got, want)
		}
	}
	if !Key("org.url").Local() || Key("orgs.url").Local() {
		t.Error("Local() is wrong")
	}
	for _, bad := range []string{"", "a..b", "a.", ".a"} {
		if k, err := ParseKey(bad); err == nil {
			t.Errorf("ParseKey(%q) = %q, want an error", bad, k)
		}
	}
}

// A file's rows are what it names, in its order, a default included; a
// section with nothing under it sets nothing.
func TestParseRows(t *testing.T) {
	rows, err := ParseSettings(FilePath, []byte(`query:
  default:
    input:
      weekStartDay: monday
    cache:
      # duration: 10m
  locked_defaults: []
  cache:
    enabled: false
apps:
  allow_origins: [https://maps.example.com]
`))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	var order []string
	for _, r := range rows {
		got[r.Key.String()] = string(r.Value)
		order = append(order, r.Key.String())
	}
	want := map[string]string{
		"query.default.input.weekStartDay": `"monday"`,
		"query.locked_defaults":            `[]`,
		"query.cache.enabled":              `false`,
		"apps.allow_origins":               `["https://maps.example.com"]`,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("rows = %v\nwant %v", got, want)
	}
	if order[0] != "query.default.input.weekStartDay" || order[3] != "apps.allow_origins" {
		t.Errorf("rows aren't in the file's order: %v", order)
	}
}

// A setting with no value, one set twice, a value of the wrong kind and too
// many origins are refused, each naming the setting.
func TestParseRowsRefuses(t *testing.T) {
	many := "apps:\n  allow_origins:\n"
	for i := range MaxOrigins + 1 {
		many += fmt.Sprintf("    - https://o%d.example.com\n", i)
	}
	for name, tc := range map[string]struct{ yml, want string }{
		"no value":              {"query:\n  cache:\n    enabled:\n", "line 3: query.cache.enabled has no value. Set one, or remove the line"},
		"null":                  {"query:\n  cache:\n    enabled: ~\n", "query.cache.enabled has no value"},
		"set twice":             {"aql:\n  compiler:\n    version: v1\n    version: v2\n", "line 4: aql.compiler.version is set twice"},
		"not a bool":            {"query:\n  cache:\n    enabled: sometimes\n", "line 3: query.cache.enabled is a boolean, not a string"},
		"not a list":            {"apps:\n  allow_origins: https://maps.example.com\n", "apps.allow_origins is a list, not a string"},
		"a value for a section": {"query:\n  cache: off\n", "line 2: query.cache holds settings, not a value"},
		"not an origin":         {"apps:\n  allow_origins: [http://maps.example.com]\n", `"http://maps.example.com" isn't an origin`},
		"too many origins":      {many, "apps.allow_origins lists 21 origins. List at most 20"},
		"a list for a string":   {"aql:\n  compiler:\n    version: [v1]\n", "aql.compiler.version is a string, not a list"},
		"an empty item":         {"apps:\n  allow_origins: [https://a.example.com, ~]\n", "apps.allow_origins has an empty item in its list. Remove it"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSettings(FilePath, []byte(tc.yml)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v\nwant one containing %q", err, tc.want)
			}
		})
	}
}

// A key anfra doesn't know is an UnknownKeyError, which says what it's near
// and where it is, for a host to word its own message.
func TestUnknownKeyError(t *testing.T) {
	_, err := ParseSettings(FilePath, []byte("query:\n  default:\n    input:\n      firstMonth: march\n"))
	u, ok := AsUnknownKey(err)
	if !ok {
		t.Fatalf("err = %v, want an UnknownKeyError", err)
	}
	if u.Line != 4 || u.Key.String() != "query.default.input.firstMonth" || u.Near != "" || strings.Join(u.Known, ",") != "timezone,weekStartDay" {
		t.Errorf("got %+v", u)
	}
}

func setting(key, value string) Setting {
	k, err := ParseKey(key)
	if err != nil {
		panic(err)
	}
	return Setting{Key: k, Value: jsontext.Value(value)}
}

// Layers apply key by key, a later one winning, a list replaced whole; a row
// that names no setting or holds a value its setting doesn't take is reported
// and skipped, so its key keeps the value below it.
func TestLayer(t *testing.T) {
	repo := []Setting{
		setting("query.default.input.timezone", `"Asia/Ho_Chi_Minh"`),
		setting("query.cache.max_duration", `"24h"`),
		setting("apps.allow_origins", `["https://a.example.com","https://b.example.com"]`),
	}
	overrides := []Setting{
		setting("query.cache.max_duration", `"1h"`),
		setting("apps.allow_origins", `["https://c.example.com"]`),
		setting("query.default.input.weekStartDay", `"funday"`),
		setting("query.default.input.firstMonth", `"march"`),
		setting("query.cache", `{"enabled":false}`),
		setting("query.cache.enabled", `null`),
		setting("query.cache.enabled.x", `false`),
		setting("query.locked_defaults", `["input.timezone", null]`),
		setting("query.cache.max_duration", `5`),
	}
	c, broken := Layer(repo, overrides)
	if c.Query.Default.Input.Timezone != "Asia/Ho_Chi_Minh" || c.QueryCacheSettings().MaxDuration != time.Hour ||
		strings.Join(c.AllowedOrigins(), ",") != "https://c.example.com" || c.Query.Default.Input.WeekStartDay != "" || !c.QueryCacheEnabled() {
		t.Errorf("layered %+v", c)
	}
	var reasons []string
	for _, b := range broken {
		reasons = append(reasons, b.Key.String()+": "+b.Err.Error())
	}
	for i, want := range []string{
		`query.default.input.weekStartDay: query.default.input.weekStartDay is "funday"`,
		"query.default.input.firstMonth: query.default.input.firstMonth isn't a setting.",
		"query.cache: query.cache holds settings, not a value",
		"query.cache.enabled: query.cache.enabled is null. Set a value, or leave the setting out",
		"query.cache.enabled.x: query.cache.enabled is a boolean, so it has nothing under it. Use the key query.cache.enabled",
		"query.locked_defaults: query.locked_defaults has an empty item in its list",
		"query.cache.max_duration: query.cache.max_duration is a string, not a number",
	} {
		if i >= len(reasons) || !strings.HasPrefix(reasons[i], want) {
			t.Errorf("broken = %q\nwant [%d] starting %q", reasons, i, want)
		}
	}
}

// Check refuses settings that contradict each other once layered, though each
// row was fine alone.
func TestCheckLayered(t *testing.T) {
	c, broken := Layer(
		[]Setting{setting("query.default.cache.duration", `"2h"`)},
		[]Setting{setting("query.cache.max_duration", `"1h"`)},
	)
	if len(broken) > 0 {
		t.Fatal(broken)
	}
	if err := c.Check(); err == nil || !strings.Contains(err.Error(), "more than query.cache.max_duration (1h)") {
		t.Errorf("Check() = %v", err)
	}
}

// Diff is what changes the settings' meaning: not comments, not a default
// written out, not a duration spelled another way, and never org.url.
func TestDiff(t *testing.T) {
	parse := func(yml string) Config {
		t.Helper()
		c, err := Parse(FilePath, []byte(yml))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	none := parse("")
	initDefaults := parse(`# This repo's anfra settings
org:
  url: https://bi.example.com/o/acme
query:
  default:
    input:
      timezone: Etc/UTC
      weekStartDay: monday
    cache:
      # duration: 10m
  locked_defaults: []
  cache:
    enabled: true
    max_duration: 168h
aql:
  compiler:
    version: v1
`)
	if d := Diff(none, initDefaults); len(d) > 0 {
		t.Errorf("init's defaults differ from no file in %v", d)
	}
	a := parse("query:\n  default:\n    cache:\n      duration: 60m\n")
	b := parse("query:\n  default:\n    cache:\n      duration: 1h\n  locked_defaults: [cache.duration]\n")
	var got []string
	for _, k := range Diff(a, b) {
		got = append(got, k.String())
	}
	if strings.Join(got, ",") != "query.locked_defaults" {
		t.Errorf("Diff = %v, want query.locked_defaults only", got)
	}
}

func TestLockedKey(t *testing.T) {
	if k := LockedKey("input.weekStartDay"); k.String() != "query.default.input.weekStartDay" {
		t.Errorf("LockedKey = %s", k)
	}
}

// Get is a setting's value as a file holds it, nil unset; Equal compares two
// by what they mean.
func TestGetAndEqual(t *testing.T) {
	c, err := Parse(FilePath, []byte("query:\n  cache:\n    enabled: false\n    max_duration: 60m\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(c.Get("query.cache.max_duration")); got != `"60m"` {
		t.Errorf("Get = %s", got)
	}
	if got := c.Get("query.default.input.timezone"); got != nil {
		t.Errorf("Get of an unset setting = %s, want nil", got)
	}
	for _, tc := range []struct {
		key  Key
		a, b string
		want bool
	}{
		{"query.cache.max_duration", `"60m"`, `"1h"`, true},
		{"query.cache.max_duration", `"60m"`, `"2h"`, false},
		{"query.default.input.weekStartDay", `"monday"`, ``, true},
		{"query.cache.enabled", `false`, ``, false},
		{"query.cache.enabled", `"nope"`, `true`, false},
	} {
		var a, b jsontext.Value
		if tc.a != "" {
			a = jsontext.Value(tc.a)
		}
		if tc.b != "" {
			b = jsontext.Value(tc.b)
		}
		if got := Equal(tc.key, a, b); got != tc.want {
			t.Errorf("Equal(%s, %s, %s) = %v, want %v", tc.key, tc.a, tc.b, got, tc.want)
		}
	}
}
