package repoconfig

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/shared/jsonkit"
)

// Every setting has a Spec, and every Spec a setting: the schema can't leave
// one out, or describe one that isn't there.
func TestEverySettingHasASpec(t *testing.T) {
	for _, k := range Keys() {
		if _, ok := specs[k]; !ok {
			t.Errorf("%s has no Spec", k)
		}
	}
	for k := range specs {
		if !slices.Contains(Keys(), k) {
			t.Errorf("Spec for %s, which isn't a setting", k)
		}
	}
}

// A Spec's default is a value its setting takes: laid over nothing, it checks.
func TestSpecDefaultsAreValid(t *testing.T) {
	for k, s := range specs {
		if s.Default == nil {
			continue
		}
		b, err := jsonkit.Marshal(s.Default)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkSetting(Setting{Key: k, Value: b}); err != nil {
			t.Errorf("%s's default %s: %v", k, b, err)
		}
	}
}

// The file's schema nests settings under their mappings, refuses keys it
// doesn't know, and carries each setting's type, default and allowed values.
func TestJSONSchema(t *testing.T) {
	s := JSONSchema()
	b, err := jsonkit.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := jsonkit.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	at := func(path ...string) map[string]any {
		t.Helper()
		node := doc
		for _, p := range path {
			props, _ := node["properties"].(map[string]any)
			node, _ = props[p].(map[string]any)
			if node == nil {
				t.Fatalf("no %s in the schema", strings.Join(path, "."))
			}
		}
		return node
	}
	if doc["additionalProperties"] != false || at("query", "cache")["additionalProperties"] != false {
		t.Error("a mapping admits keys it doesn't know")
	}
	day := at("query", "default", "input", "weekStartDay")
	if day["type"] != "string" || day["default"] != "monday" || len(day["enum"].([]any)) != 7 {
		t.Errorf("weekStartDay: %v", day)
	}
	if c := at("query", "cache", "max_duration"); c["default"] != "168h" || c["x-anfra-format"] != FormatDuration {
		t.Errorf("max_duration: %v", c)
	}
	if e := at("query", "cache", "enabled"); e["type"] != "boolean" || e["default"] != true {
		t.Errorf("enabled: %v", e)
	}
	origins := at("apps", "allow_origins")
	items, _ := origins["items"].(map[string]any)
	if origins["type"] != "array" || origins["maxItems"] != float64(MaxOrigins) || items["x-anfra-format"] != FormatOrigin {
		t.Errorf("allow_origins: %v", origins)
	}
	for _, name := range Plugins {
		e := at("apps", "plugins", name, "enabled")
		if e["type"] != "boolean" || e["default"] != true || e["title"] == "" || e["description"] == "" {
			t.Errorf("apps.plugins.%s.enabled: %v", name, e)
		}
	}
	if _, ok := at("apps", "plugins")["properties"].(map[string]any)["core"]; ok {
		t.Error("core has a key, but it's always installed")
	}
	if at("aql")["title"] != "AQL" {
		t.Errorf("aql's title: %v", at("aql")["title"])
	}
	if SettingSchema("query.cache") != nil || SettingSchema("query.cache.ttl") != nil {
		t.Error("a schema for something that isn't a setting")
	}
}

// A format's pattern is never stricter than its check: what the check takes,
// the pattern takes too.
func TestPatternsAreLooserThanChecks(t *testing.T) {
	duration := regexp.MustCompile(patterns[FormatDuration])
	for _, s := range []string{"30s", "10m", "1h30m", "1.5h", "500ms", "168h"} {
		if _, err := time.ParseDuration(s); err != nil {
			t.Fatal(err)
		}
		if !duration.MatchString(s) {
			t.Errorf("the duration pattern refuses %s, which anfra takes", s)
		}
	}
	origin := regexp.MustCompile(patterns[FormatOrigin])
	for _, s := range []string{"https://cdn.example.com", "https://cdn.example.com:8443"} {
		if !origin.MatchString(s) {
			t.Errorf("the origin pattern refuses %s, which anfra takes", s)
		}
	}
}

// A setting shows under its key's first two names, or the group its Spec sets:
// the locks with the defaults they lock.
func TestGroupOf(t *testing.T) {
	for k, want := range map[Key]Key{
		"query.default.input.timezone": "query.default",
		"query.default.cache.duration": "query.default",
		"query.locked_defaults":        "query.default",
		"query.cache.enabled":          "query.cache",
		"aql.compiler.version":         "aql.compiler",
		"apps.allow_origins":           "apps",
		"apps.plugins.render.enabled":  "apps.plugins",
	} {
		if got := GroupOf(k); got != want {
			t.Errorf("GroupOf(%s) = %s, want %s", k, got, want)
		}
	}
	for k := range specs {
		if g := GroupOf(k); groups[g] == "" {
			t.Errorf("%s shows under %s, which has no title", k, g)
		}
	}
}
