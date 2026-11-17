package repoconfig

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/holistics/anfra/shared/jsonkit"
)

// Check refuses settings no repo can run with: a value anfra doesn't take, or
// settings that contradict each other, such as a default cache duration over
// the cap. Each error names the setting and how to fix it. A host checks the
// settings in effect with it, after Layer.
func (c Config) Check() error {
	if err := c.checkValues(); err != nil {
		return err
	}
	if err := c.QueryCacheSettings().CheckDefault(c.QueryDefaults()); err != nil {
		return err
	}
	return c.checkLocks()
}

// checkValues refuses a value anfra doesn't take, each judged alone, so a
// Config with one setting set fails only for that setting.
func (c Config) checkValues() error {
	if v := c.AQL.Compiler.Version; v != "" && v != "v1" && v != "v2" {
		return fmt.Errorf("aql.compiler.version is %q. Use v1 or v2", v)
	}
	q := c.Query
	if s := q.Cache.MaxDuration; s != "" {
		d, err := time.ParseDuration(s)
		switch {
		case err != nil:
			return notDuration("query.cache.max_duration", s)
		case d == 0: // unset, to the engine: a setting says it, so it means none
			return fmt.Errorf("query.cache.max_duration is %s. Use a duration from 1s to 168h (7 days). To turn the query cache off, set query.cache.enabled to false", s)
		}
	}
	if s := q.Default.Cache.Duration; s != "" {
		if _, err := time.ParseDuration(s); err != nil {
			return notDuration("query.default.cache.duration", s)
		}
	}
	if err := c.QueryCacheSettings().Check(); err != nil {
		return err
	}
	if err := c.QueryDefaults().Check(); err != nil {
		return err
	}
	return checkOrigins(c.Apps.AllowOrigins)
}

// checkLocks refuses a lock on a default query.default doesn't set, since the
// settings say each value they lock; QueryDefaults.Check refused a path that
// isn't a default.
func (c Config) checkLocks() error {
	for _, path := range c.Query.LockedDefaults {
		f, ok := field(reflect.ValueOf(c), LockedKey(path))
		if !ok || f.IsZero() {
			return fmt.Errorf("query.locked_defaults names %s, which query.default doesn't set. Set query.default.%s, or remove it from query.locked_defaults", path, path)
		}
		if path == "cache.duration" && !c.QueryCacheEnabled() {
			return errors.New("query.locked_defaults locks cache.duration, but query.cache.enabled is false. Remove the lock, or turn the cache on")
		}
	}
	return nil
}

func notDuration(setting, s string) error {
	return fmt.Errorf("%s is %q, which isn't a duration. Use one such as 30s, 10m or 2h", setting, s)
}

// Diff is the settings whose values differ between a and b, in Keys' order,
// leaving out the folder's own (Key.Local). It compares what each setting
// means, not how it's written: a setting at anfra's default equals one not
// set, and 60m equals 1h. A list's order counts.
func Diff(a, b Config) []Key {
	var out []Key
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	for _, k := range Keys() {
		if k.Local() {
			continue
		}
		fa, _ := field(va, k)
		fb, _ := field(vb, k)
		if !reflect.DeepEqual(meaning(k, fa), meaning(k, fb)) {
			out = append(out, k)
		}
	}
	return out
}

// meaning is what f, the setting k, means: its value with anfra's default in
// place of unset, durations as durations, and an empty list as none.
func meaning(k Key, f reflect.Value) any {
	switch k.String() {
	case "query.default.input.timezone":
		return orDefault(f.String(), "Etc/UTC")
	case "query.default.input.weekStartDay":
		return orDefault(f.String(), "monday")
	case "query.default.cache.duration":
		d, _ := time.ParseDuration(f.String())
		return d
	case "query.cache.max_duration":
		d, _ := time.ParseDuration(orDefault(f.String(), "168h"))
		return d
	case "query.cache.enabled":
		return f.IsNil() || f.Elem().Bool()
	case "aql.compiler.version":
		return orDefault(f.String(), "v1")
	}
	if f.Kind() == reflect.Slice {
		if f.Len() == 0 {
			return []string(nil)
		}
		return slices.Clone(f.Interface().([]string))
	}
	return f.Interface()
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// Get is k's value in c, as JSON, as a file would hold it: nil when c leaves it
// unset, which is anfra's default, or when k names no setting.
func (c Config) Get(k Key) jsontext.Value {
	f, ok := field(reflect.ValueOf(c), k)
	if !ok || f.IsZero() {
		return nil
	}
	if f.Kind() == reflect.Pointer {
		f = f.Elem()
	}
	b, err := jsonkit.Marshal(f.Interface())
	if err != nil {
		return nil
	}
	return b
}

// Equal reports whether a and b, two values of the setting k as JSON, mean the
// same (Diff): nil is unset, anfra's default. A value k doesn't take equals
// nothing but itself.
func Equal(k Key, a, b jsontext.Value) bool {
	if string(a) == string(b) {
		return true
	}
	one := func(v jsontext.Value) (Config, bool) {
		if v == nil {
			return Config{}, true
		}
		c, broken := Layer([]Setting{{Key: k, Value: v}})
		return c, len(broken) == 0
	}
	ca, okA := one(a)
	cb, okB := one(b)
	return okA && okB && !slices.Contains(Diff(ca, cb), k)
}
