package repoconfig

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/shared/didyoumean"
)

// weekStartDays are the values input.week_start_day takes.
var weekStartDays = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

// checkKeys refuses the first key in n that t, the struct n decodes into, has
// no field for, naming its line and path, and the key meant when one is near.
// The struct is the allowlist: a setting exists once its field does.
func checkKeys(n *yaml.Node, t reflect.Type, path string) error {
	if n.Kind != yaml.MappingNode || t.Kind() != reflect.Struct {
		return nil // not a mapping, or a value: Decode says what's wrong with it
	}
	keys := yamlKeys(t)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		at := join(path, k.Value)
		field, ok := keys[k.Value]
		if !ok {
			return unknownKey(k, at, path, t)
		}
		ft := field.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if err := checkKeys(v, ft, at); err != nil {
			return err
		}
	}
	return nil
}

func unknownKey(k *yaml.Node, at, parent string, t reflect.Type) error {
	msg := fmt.Sprintf("%s line %d: %s isn't a setting.", RelPath, k.Line, at)
	names := yamlNames(t)
	if near := didyoumean.Find(k.Value, names); near != "" {
		return fmt.Errorf("%s Did you mean %s?", msg, near)
	}
	where := "at the top level"
	if parent != "" {
		where = "under " + parent
	}
	return fmt.Errorf("%s The settings %s are %s. If a newer anfra added it, update anfra (`anfra update`).", msg, where, list(names))
}

// yamlKeys is t's fields by their YAML key.
func yamlKeys(t reflect.Type) map[string]reflect.StructField {
	keys := map[string]reflect.StructField{}
	for f := range t.Fields() {
		if name := yamlName(f); name != "" {
			keys[name] = f
		}
	}
	return keys
}

// yamlNames is t's YAML keys, in the struct's order.
func yamlNames(t reflect.Type) []string {
	var names []string
	for f := range t.Fields() {
		if name := yamlName(f); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func yamlName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
	if name == "-" || !f.IsExported() {
		return ""
	}
	return name
}

// check refuses values anfra doesn't take, and settings that contradict each
// other, each error naming the setting and how to fix it.
func (c Config) check() error {
	q := c.Query
	maxDuration := querycache.MaxDuration
	if s := q.Cache.MaxDuration; s != "" {
		d, err := time.ParseDuration(s)
		switch {
		case err != nil:
			return notDuration("query.cache.max_duration", s)
		case d <= 0:
			return fmt.Errorf("query.cache.max_duration in %s is %s. Use a duration from 1s to 168h (7 days). To turn the query cache off, set query.cache.enabled to false", RelPath, s)
		case d > querycache.MaxDuration:
			return fmt.Errorf("query.cache.max_duration in %s is %s, but anfra caches a result for at most 168h (7 days). Use 168h or less", RelPath, s)
		}
		maxDuration = d
	}
	if s := q.Default.Cache.Duration; s != "" {
		d, err := time.ParseDuration(s)
		switch {
		case err != nil:
			return notDuration("query.default.cache.duration", s)
		case d < 0:
			return fmt.Errorf("query.default.cache.duration in %s is %s, which is negative. Use 0s or more", RelPath, s)
		case d > maxDuration:
			return fmt.Errorf("query.default.cache.duration in %s is %s, more than query.cache.max_duration (%s). Lower the default or raise the cap", RelPath, s, maxDurationShown(q.Cache.MaxDuration))
		}
	}
	if s := q.Default.Input.Timezone; s != "" {
		if _, err := time.LoadLocation(s); err != nil || s == "Local" {
			return fmt.Errorf("query.default.input.timezone in %s is %q, which isn't an IANA time zone. Use one such as Asia/Ho_Chi_Minh or Etc/UTC", RelPath, s)
		}
	}
	if s := q.Default.Input.WeekStartDay; s != "" && !slices.Contains(weekStartDays, s) {
		return fmt.Errorf("query.default.input.week_start_day in %s is %q. Use one of %s", RelPath, s, list(weekStartDays))
	}
	return c.checkLocks()
}

// checkLocks refuses a locked default that query.default doesn't set, or can't.
func (c Config) checkLocks() error {
	set := defaultValues(reflect.ValueOf(c.Query.Default), "")
	for _, path := range c.Query.LockedDefaults {
		value, isDefault := set[path]
		switch {
		case isDefault && value != "":
		case isDefault:
			return fmt.Errorf("query.locked_defaults in %s names %s, which query.default doesn't set. Set query.default.%s, or remove it from query.locked_defaults", RelPath, path, path)
		case isObject(path):
			return fmt.Errorf("query.locked_defaults in %s names %s, which has fields that can't have a default. Lock its fields one by one, such as %s", RelPath, path, list(fieldsUnder(path)))
		default:
			return fmt.Errorf("query.locked_defaults in %s names %s, which isn't a default. The defaults are %s", RelPath, path, list(defaultPaths()))
		}
		if path == "cache.duration" && !c.QueryCacheEnabled() {
			return fmt.Errorf("query.locked_defaults in %s locks cache.duration, but query.cache.enabled is false. Remove the lock, or turn the cache on", RelPath)
		}
	}
	return nil
}

// defaultValues is each default's path in v, a query.default struct, and its
// value: "" when unset.
func defaultValues(v reflect.Value, prefix string) map[string]string {
	out := map[string]string{}
	for i := range v.NumField() {
		name := yamlName(v.Type().Field(i))
		if name == "" {
			continue
		}
		f := v.Field(i)
		if f.Kind() == reflect.Struct {
			for k, s := range defaultValues(f, join(prefix, name)) {
				out[k] = s
			}
			continue
		}
		out[join(prefix, name)] = f.String()
	}
	return out
}

// defaultPaths is every default's path, in the struct's order.
func defaultPaths() []string { return leafPaths(reflect.TypeOf(Config{}.Query.Default), "") }

func leafPaths(t reflect.Type, prefix string) []string {
	var out []string
	for f := range t.Fields() {
		name := yamlName(f)
		if name == "" {
			continue
		}
		if f.Type.Kind() == reflect.Struct {
			out = append(out, leafPaths(f.Type, join(prefix, name))...)
			continue
		}
		out = append(out, join(prefix, name))
	}
	return out
}

// isObject reports whether path names an object of query.default, such as input.
func isObject(path string) bool { return len(fieldsUnder(path)) > 0 }

// fieldsUnder is the defaults under the object at path.
func fieldsUnder(path string) []string {
	var out []string
	for _, p := range defaultPaths() {
		if strings.HasPrefix(p, path+".") {
			out = append(out, p)
		}
	}
	return out
}

func notDuration(setting, s string) error {
	return fmt.Errorf("%s in %s is %q, which isn't a duration. Use one such as 30s, 10m or 2h", setting, RelPath, s)
}

func maxDurationShown(s string) string {
	if s == "" {
		return "168h, by default"
	}
	return s
}

func join(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// list is names as prose: "a", "a and b", or "a, b, and c".
func list(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
}
