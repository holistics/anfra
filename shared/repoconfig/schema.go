package repoconfig

import (
	"reflect"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
)

// Spec is one setting as anfra defines it: what it's called and does, the
// value anfra uses when nothing sets it, and the values it takes. The checks
// enforce what it says (checkValues reads Enum, Format and MaxItems, Diff reads
// Default), and JSONSchema publishes it, so what a client reads is what anfra
// refuses. Rules across settings, such as a default over the cap, aren't a
// setting's: Check holds them, and a client learns of them from its error.
type Spec struct {
	// Label names the setting for people: "Time zone".
	Label string
	// Description says what it does: one sentence with no end period, or
	// several, each with its own.
	Description string
	// Default is the value anfra uses when nothing sets it, as the file would
	// hold it; nil when there's none, such as no default cache duration.
	Default any
	// Enum is the values a string takes, or a list's items take.
	Enum []string
	// Format names the check a string's value, or a list's items, pass beyond
	// its type: FormatDuration, FormatTimezone, FormatOrigin. "" for none.
	Format string
	// MaxDuration is the longest a FormatDuration value may be; zero, no cap.
	MaxDuration time.Duration
	// MaxItems is the most items a list holds; zero, no limit.
	MaxItems int
	// Group is the mapping the setting is shown under, for people: by default
	// its key's first two names (query.default for
	// query.default.input.timezone), or its first for a key of two. Set for a
	// setting that belongs with others than its key's, as query.locked_defaults
	// with the defaults it locks.
	Group Key
}

// The formats a Spec names, each one check.
const (
	// FormatDuration is a Go duration: 30s, 10m, 2h.
	FormatDuration = "duration"
	// FormatTimezone is an IANA time zone that anfra knows: Asia/Ho_Chi_Minh.
	FormatTimezone = "timezone"
	// FormatOrigin is an https origin a content policy takes
	// (repoformat.CheckOrigin).
	FormatOrigin = "origin"
)

// Compiler versions, aql.compiler.version's values.
var compilerVersions = []string{"v1", "v2"}

// specs is every setting's Spec, by key. TestEverySettingHasASpec holds it to
// Keys.
var specs = map[Key]Spec{
	"org.url": {
		Label:       "Organization",
		Description: "The organization this folder publishes to, by its address. The organization itself ignores it.",
		Format:      "uri",
	},
	"apps.allow_origins": {
		Label:       "Allowed origins",
		Description: "Servers data apps may load code from and send requests to, beyond the SDK's own",
		Default:     []string{},
		Format:      FormatOrigin,
		MaxItems:    MaxOrigins,
	},
	PluginKey("inspect"): {
		Label:       "Inspect",
		Description: "A panel that shows what a data app runs: its queries, the data they return, and its structure",
		Default:     true,
	},
	PluginKey("lineage"): {
		Label:       "Lineage",
		Description: "Data apps can trace a field to the models and tables it comes from, and the panel draws where each number comes from",
		Default:     true,
	},
	PluginKey("view-underlying-data"): {
		Label:       "View underlying data",
		Description: "Readers can list the rows behind a point on a chart, from its menu",
		Default:     true,
	},
	PluginKey("render"): {
		Label:       "Render",
		Description: "Data apps can save a picture of themselves, or of a part, as a PNG or PDF file for the reader",
		Default:     true,
	},
	PluginKey("export"): {
		Label:       "Export",
		Description: "Data apps can save a query's whole result as a file for the reader",
		Default:     true,
	},
	"query.default.input.timezone": {
		Label:       "Time zone",
		Description: "For relative dates and date truncation, in a query that sets none",
		Default:     command.DefaultTimezone,
		Format:      FormatTimezone,
	},
	"query.default.input.weekStartDay": {
		Label:       "Week start day",
		Description: "The day weeks start on, in a query that sets none",
		Default:     command.DefaultWeekStartDay,
		Enum:        anfranode.WeekStartDays,
	},
	"query.default.cache.duration": {
		Label:       "Cache duration",
		Description: "How old a cached result a query accepts, and how long it keeps its own, in a query that sets none. Unset, such a query runs uncached.",
		Format:      FormatDuration,
		MaxDuration: querycache.MaxDuration,
	},
	"query.locked_defaults": {
		Label:       "Locked defaults",
		Description: "The defaults a query can't change",
		Default:     []string{},
		Enum:        command.LockablePaths,
		Group:       "query.default",
	},
	"query.cache.enabled": {
		Label:       "Query cache",
		Description: "Whether queries may use cached results",
		Default:     true,
	},
	"query.cache.max_duration": {
		Label:       "Longest cache duration",
		Description: "The longest a query's result may be cached",
		Default:     command.FormatDuration(querycache.MaxDuration),
		Format:      FormatDuration,
		MaxDuration: querycache.MaxDuration,
	},
	"aql.compiler.version": {
		Label:       "AQL compiler",
		Description: "The compiler queries and validation use. v2 is experimental.",
		Default:     compilerVersions[0],
		Enum:        compilerVersions,
	},
}

// groups are the names of the mappings settings sit in, for people.
var groups = map[Key]string{
	"org":                 "Publishing",
	"apps":                "Data apps",
	"apps.plugins":        "Plugins",
	"query":               "Queries",
	"query.default":       "Defaults",
	"query.default.input": "Input",
	"query.default.cache": "Cache",
	"query.cache":         "Cache",
	"aql":                 "AQL",
	"aql.compiler":        "Compiler",
}

// GroupOf is the mapping the setting k is shown under (Spec.Group).
func GroupOf(k Key) Key {
	if g := specs[k].Group; g != "" {
		return g
	}
	names := k.Segments()
	if len(names) > 2 {
		return Key(names[0] + "." + names[1])
	}
	return Key(names[0])
}

// SpecOf is the setting k's Spec, ok false when k names no setting.
func SpecOf(k Key) (Spec, bool) {
	s, ok := specs[k]
	return s, ok
}

// JSONSchema is the settings file's JSON Schema (2020-12): every setting's
// type, default, allowed values and description, from its Spec, under the
// mappings it sits in, none admitting a key it doesn't know. A format anfra
// checks that JSON Schema has no keyword for is x-anfra-format, with a pattern
// where one fits, so an editor checks what it can.
func JSONSchema() map[string]any {
	s := object(reflect.TypeFor[Config](), "")
	s["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	s["title"] = "anfra repo settings"
	s["description"] = "A repo's settings, .anfra/config.yml"
	return s
}

// object is the schema of t, a mapping, at key.
func object(t reflect.Type, key Key) map[string]any {
	props := map[string]any{}
	for f := range t.Fields() {
		name := yamlName(f)
		if name == "" {
			continue
		}
		k := key.child(name)
		if f.Type.Kind() == reflect.Struct {
			props[name] = object(f.Type, k)
			continue
		}
		props[name] = SettingSchema(k)
	}
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if title, ok := groups[key]; ok {
		s["title"] = title
	}
	return s
}

// SettingSchema is the JSON Schema of the setting k's value: nil when k names
// no setting.
func SettingSchema(k Key) map[string]any {
	var c Config
	f, ok := field(reflect.ValueOf(&c).Elem(), k)
	spec, known := specs[k]
	if !ok || !known {
		return nil
	}
	t := f.Type()
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	s := map[string]any{"title": spec.Label, "description": spec.Description, "x-anfra-group": GroupOf(k).String()}
	if spec.Default != nil {
		s["default"] = spec.Default
	}
	value := s
	switch t.Kind() {
	case reflect.Bool:
		s["type"] = "boolean"
	case reflect.Slice:
		value = map[string]any{"type": "string"}
		s["type"], s["items"] = "array", value
		if spec.MaxItems > 0 {
			s["maxItems"] = spec.MaxItems
		}
	default:
		s["type"] = "string"
	}
	if len(spec.Enum) > 0 {
		value["enum"] = spec.Enum
	}
	switch spec.Format {
	case "":
	case "uri":
		value["format"] = "uri"
	default:
		value["x-anfra-format"] = spec.Format
		if p, ok := patterns[spec.Format]; ok {
			value["pattern"] = p
		}
	}
	if spec.MaxDuration > 0 {
		value["x-anfra-max-duration"] = command.FormatDuration(spec.MaxDuration)
	}
	return s
}

// patterns are the JSON Schema patterns of the formats one fits: looser than
// the check, which has the last word, never stricter.
var patterns = map[string]string{
	FormatDuration: `^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$`,
	FormatOrigin:   `^https://[^/?#\s]+$`,
}
