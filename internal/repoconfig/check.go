package repoconfig

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/didyoumean"
)

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
	if s := q.Cache.MaxDuration; s != "" {
		d, err := time.ParseDuration(s)
		switch {
		case err != nil:
			return notDuration("query.cache.max_duration", s)
		case d == 0: // unset, to the engine: the file says it, so it means none
			return fmt.Errorf(RelPath+": query.cache.max_duration is %s. Use a duration from 1s to 168h (7 days). To turn the query cache off, set query.cache.enabled to false", s)
		}
	}
	if s := q.Default.Cache.Duration; s != "" {
		if _, err := time.ParseDuration(s); err != nil {
			return notDuration("query.default.cache.duration", s)
		}
	}
	cache, defaults := c.QueryCacheSettings(), c.QueryDefaults()
	for _, err := range []error{cache.Check(), defaults.Check(), cache.CheckDefault(defaults)} {
		if err != nil {
			return fmt.Errorf("%s: %w", RelPath, err)
		}
	}
	return c.checkLocks()
}

// checkLocks refuses a lock on a default query.default doesn't set, since
// the file says each value it locks; QueryDefaults.Check refused a path that
// isn't a default.
func (c Config) checkLocks() error {
	set := defaultValues(reflect.ValueOf(c.Query.Default), "")
	for _, path := range c.Query.LockedDefaults {
		if set[path] == "" {
			return fmt.Errorf(RelPath+": query.locked_defaults names %s, which query.default doesn't set. Set query.default.%s, or remove it from query.locked_defaults", path, path)
		}
		if path == "cache.duration" && !c.QueryCacheEnabled() {
			return fmt.Errorf(RelPath + ": query.locked_defaults locks cache.duration, but query.cache.enabled is false. Remove the lock, or turn the cache on")
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

func notDuration(setting, s string) error {
	return fmt.Errorf("%s: %s is %q, which isn't a duration. Use one such as 30s, 10m or 2h", RelPath, setting, s)
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
