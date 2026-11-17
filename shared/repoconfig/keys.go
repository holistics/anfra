package repoconfig

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Key is a setting's path in the settings: the names on the way to it, spelled
// as the file spells them, joined with ".", such as
// query.default.input.weekStartDay. No name in a key holds a ".", so the
// dotted form is exact, with no quoting, wherever a key is stored, sent or
// shown. A setting keyed by names users choose (none yet) refuses a name with
// a ".".
type Key string

// String is the key as people read it.
func (k Key) String() string { return string(k) }

// Segments are the key's names, in order.
func (k Key) Segments() []string { return strings.Split(string(k), ".") }

// Within reports whether k is prefix or a setting under it, name by name: so
// aql covers aql.compiler.version, and not aqlx.
func (k Key) Within(prefix Key) bool {
	return k == prefix || strings.HasPrefix(string(k), string(prefix)+".")
}

// Local reports whether k is about the folder rather than the repo, such as
// org.url, which says where a folder publishes: a platform ignores it.
func (k Key) Local() bool { return k.Within("org") }

// child is the key of name under k.
func (k Key) child(name string) Key {
	if k == "" {
		return Key(name)
	}
	return k + "." + Key(name)
}

// parent is the key k is under, or "" at the top level.
func (k Key) parent() Key {
	i := strings.LastIndexByte(string(k), '.')
	if i < 0 {
		return ""
	}
	return k[:i]
}

// last is k's own name.
func (k Key) last() string { return string(k[strings.LastIndexByte(string(k), '.')+1:]) }

// ParseKey reads a key, refusing one with an empty name.
func ParseKey(s string) (Key, error) {
	for _, name := range strings.Split(s, ".") {
		if name == "" {
			return "", fmt.Errorf("%q isn't a key. Write one such as query.cache.max_duration", s)
		}
	}
	return Key(s), nil
}

// Keys is every setting anfra knows, in Config's order.
func Keys() []Key {
	var keys []Key
	var walk func(t reflect.Type, prefix Key)
	walk = func(t reflect.Type, prefix Key) {
		for f := range t.Fields() {
			name := yamlName(f)
			if name == "" {
				continue
			}
			k := prefix.child(name)
			if f.Type.Kind() == reflect.Struct {
				walk(f.Type, k)
				continue
			}
			keys = append(keys, k)
		}
	}
	walk(reflect.TypeFor[Config](), "")
	return keys
}

// field is the setting k names in v, a Config: addressable when v is.
func field(v reflect.Value, k Key) (reflect.Value, bool) {
	if k == "" {
		return reflect.Value{}, false
	}
	names := k.Segments()
	for i, name := range names {
		if v.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		f, ok := yamlKeys(v.Type())[name]
		if !ok {
			return reflect.Value{}, false
		}
		v = v.FieldByIndex(f.Index)
		if v.Kind() == reflect.Struct && i == len(names)-1 {
			return reflect.Value{}, false // a mapping, not a setting
		}
	}
	return v, true
}

// LockedKey is the key a query.locked_defaults entry locks: its path under
// query.default, such as input.weekStartDay for
// query.default.input.weekStartDay.
func LockedKey(entry string) Key { return Key("query.default").child(entry) }

// UnknownKeyError is a key the settings have no such setting for. Near is the
// key meant, when one is close; Known are the settings beside it.
type UnknownKeyError struct {
	// File names the file the key is in, as messages name it.
	File string
	// Line is the key's line in File, or 0 when it isn't from a file.
	Line  int
	Key   Key
	Near  string
	Known []string
}

func (e *UnknownKeyError) Error() string {
	msg := fmt.Sprintf("%s isn't a setting.", e.Key)
	switch {
	case e.File != "" && e.Line > 0:
		msg = fmt.Sprintf("%s line %d: %s", e.File, e.Line, msg)
	case e.File != "":
		msg = e.File + ": " + msg
	}
	if e.Near != "" {
		return fmt.Sprintf("%s Did you mean %s?", msg, e.Near)
	}
	where := "at the top level"
	if p := e.Key.parent(); p != "" {
		where = "under " + p.String()
	}
	return fmt.Sprintf("%s The settings %s are %s. If a newer anfra added it, update anfra (`anfra update`).", msg, where, prose(e.Known))
}

// unknownKey is the error for k, which has no setting: t is the mapping it's
// in, so its keys are what k may have meant.
func unknownKey(file string, line int, k Key, t reflect.Type) *UnknownKeyError {
	known := yamlNames(t)
	return &UnknownKeyError{File: file, Line: line, Key: k, Near: didYouMean(k.last(), known), Known: known}
}

// AsUnknownKey is err's UnknownKeyError, if it is one.
func AsUnknownKey(err error) (*UnknownKeyError, bool) {
	var u *UnknownKeyError
	ok := errors.As(err, &u)
	return u, ok
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

// prose is names as a sentence lists them: "a", "a and b", or "a, b, and c".
func prose(names []string) string {
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
