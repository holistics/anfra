package repoconfig

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/didyoumean"
	"github.com/holistics/anfra/shared/jsonkit"
)

// Setting is one setting's value, as one layer sets it: its key, and its value
// as JSON, as the file would hold it.
type Setting struct {
	Key   Key
	Value jsontext.Value
}

// BrokenSetting is a setting Layer didn't apply, and why: a key anfra doesn't
// know, or a value that key doesn't take.
type BrokenSetting struct {
	Setting
	Err error
}

// Parse reads a settings file's content: what it sets, laid over anfra's
// defaults, and checked as a whole. file names it in errors, as
// .anfra/config.yml. Empty content, or comments only, is anfra's defaults.
func Parse(file string, content []byte) (Config, error) {
	set, err := ParseSettings(file, content)
	if err != nil {
		return Config{}, err
	}
	c, broken := Layer(set)
	if len(broken) > 0 { // ParseSettings checked each, so this is a bug
		return Config{}, fmt.Errorf("%s: %w", file, broken[0].Err)
	}
	if err := c.Check(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", file, err)
	}
	return c, nil
}

// ParseSettings is what a settings file's content sets, one per setting it
// names, in the file's order, each value checked on its own. It refuses a key
// anfra doesn't know (an UnknownKeyError), a setting with no value, one set
// twice, and a value its setting doesn't take. It doesn't check how the
// settings go together: Layer them, then Check.
func ParseSettings(file string, content []byte) ([]Setting, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	if len(doc.Content) == 0 { // empty, or comments only
		return nil, nil
	}
	var settings []Setting
	if err := collect(file, doc.Content[0], reflect.TypeFor[Config](), "", &settings); err != nil {
		return nil, err
	}
	for _, r := range settings {
		if err := checkSetting(r); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	}
	return settings, nil
}

// collect appends the settings n sets to settings: n is the mapping for t, at
// prefix.
func collect(file string, n *yaml.Node, t reflect.Type, prefix Key, settings *[]Setting) error {
	if n.Tag == "!!null" {
		return nil // a section with nothing under it, or only comments
	}
	if n.Kind != yaml.MappingNode {
		where := "The file"
		if prefix != "" {
			where = prefix.String()
		}
		return fmt.Errorf("%s line %d: %s holds settings, not a value. Write its settings under it, such as %s", file, n.Line, where, yamlNames(t)[0])
	}
	fields := yamlKeys(t)
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		key := prefix.child(k.Value)
		f, ok := fields[k.Value]
		if !ok {
			return unknownKey(file, k.Line, key, t)
		}
		if seen[k.Value] {
			return fmt.Errorf("%s line %d: %s is set twice. Keep one", file, k.Line, key)
		}
		seen[k.Value] = true
		if f.Type.Kind() == reflect.Struct {
			if err := collect(file, v, f.Type, key, settings); err != nil {
				return err
			}
			continue
		}
		if v.Tag == "!!null" {
			return fmt.Errorf("%s line %d: %s has no value. Set one, or remove the line", file, k.Line, key)
		}
		value, err := decodeYAML(v, f.Type)
		if err != nil {
			return fmt.Errorf("%s line %d: %s %w", file, v.Line, key, err)
		}
		*settings = append(*settings, Setting{Key: key, Value: value})
	}
	return nil
}

// decodeYAML is n's value as t, a setting's type, written as JSON. Its error
// follows the setting's key: "is a boolean, not a list".
func decodeYAML(n *yaml.Node, t reflect.Type) (jsontext.Value, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if n.Kind == yaml.SequenceNode {
		for _, item := range n.Content {
			if item.Tag == "!!null" {
				return nil, errEmptyItem
			}
		}
	}
	v := reflect.New(t)
	if err := n.Decode(v.Interface()); err != nil {
		return nil, fmt.Errorf("is %s, not %s", typeName(t), yamlKind(n))
	}
	b, err := jsonkit.Marshal(v.Elem().Interface())
	if err != nil {
		return nil, err
	}
	return b, nil
}

var errEmptyItem = errors.New("has an empty item in its list. Remove it")

// typeName is what a setting of type t is, as a message names it: a boolean,
// a list, or a string.
func typeName(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return "a boolean"
	case reflect.Slice:
		return "a list"
	}
	return "a string"
}

// yamlKind is what n is, as a message names it.
func yamlKind(n *yaml.Node) string {
	switch n.Kind {
	case yaml.SequenceNode:
		return "a list"
	case yaml.MappingNode:
		return "an object"
	}
	switch n.Tag {
	case "!!bool":
		return "a boolean"
	case "!!int", "!!float":
		return "a number"
	}
	return "a string"
}

// jsonKind is what v is, as a message names it.
func jsonKind(v jsontext.Value) string {
	switch v.Kind() {
	case 't', 'f':
		return "a boolean"
	case '0':
		return "a number"
	case '[':
		return "a list"
	case '{':
		return "an object"
	}
	return "a string"
}

// Layer is anfra's defaults with each list of settings applied over them, key by
// key, a later list winning: the repo's file, then a member's own file; or the
// repo's settings, then a platform's overrides. A setting whose key names none,
// or holds a value its setting doesn't take, is returned as broken and not
// applied, so its key keeps its value from the layers below. Layer doesn't
// check how the settings go together: Check the result.
func Layer(layers ...[]Setting) (Config, []BrokenSetting) {
	var c Config
	var broken []BrokenSetting
	for _, layer := range layers {
		for _, r := range layer {
			if err := checkSetting(r); err != nil {
				broken = append(broken, BrokenSetting{Setting: r, Err: err})
				continue
			}
			f, _ := field(reflect.ValueOf(&c).Elem(), r.Key)
			_ = setJSON(f, r.Value) // checkSetting decoded it
		}
	}
	return c, broken
}

// checkSetting refuses a setting whose key names none, or whose value isn't one
// its setting takes, judged alone: a Config with only that setting set.
func checkSetting(r Setting) error {
	var c Config
	f, ok := field(reflect.ValueOf(&c).Elem(), r.Key)
	if !ok {
		return unknownSettingKey(r.Key)
	}
	if err := setJSON(f, r.Value); err != nil {
		return fmt.Errorf("%s %w", r.Key, err)
	}
	return c.checkValues()
}

// unknownSettingKey is the error for a key that names no setting.
func unknownSettingKey(k Key) error {
	if _, err := ParseKey(string(k)); err != nil {
		return err
	}
	t := reflect.TypeFor[Config]()
	var at Key
	names := k.Segments()
	for _, name := range names[:len(names)-1] {
		at = at.child(name)
		f, ok := yamlKeys(t)[name]
		if !ok {
			return unknownKey("", 0, at, t)
		}
		if f.Type.Kind() != reflect.Struct {
			return fmt.Errorf("%s is %s, so it has nothing under it. Use the key %s", at, typeName(f.Type), at)
		}
		t = f.Type
	}
	if f, ok := yamlKeys(t)[k.last()]; ok && f.Type.Kind() == reflect.Struct {
		return fmt.Errorf("%s holds settings, not a value. Set its settings one by one, such as %s.%s", k, k, yamlNames(f.Type)[0])
	}
	return unknownKey("", 0, k, t)
}

// setJSON sets f, a setting, to value. JSON null is refused, in a list too: no
// setting takes it, and an unset setting is one a layer leaves out, not a null one.
// Its error follows the setting's key.
func setJSON(f reflect.Value, value jsontext.Value) error {
	if value.Kind() == 'n' {
		return errors.New("is null. Set a value, or leave the setting out")
	}
	t := f.Type()
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if value.Kind() == '[' && t.Kind() == reflect.Slice {
		var items []any
		if err := jsonkit.Unmarshal(value, &items); err == nil && slices.Contains(items, nil) {
			return errEmptyItem
		}
	}
	v := reflect.New(t)
	if err := jsonkit.Unmarshal(value, v.Interface()); err != nil {
		return fmt.Errorf("is %s, not %s", typeName(t), jsonKind(value))
	}
	if f.Kind() == reflect.Pointer {
		f.Set(v)
	} else {
		f.Set(v.Elem())
	}
	return nil
}

func didYouMean(typed string, names []string) string { return didyoumean.Find(typed, names) }
