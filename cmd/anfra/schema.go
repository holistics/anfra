package main

import (
	"encoding/json/jsontext"
	"iter"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/jsonkit"
)

// opAnnotation names a core command's op, for --schema to describe it as
// discovery does.
const opAnnotation = "anfra.op"

// resultTypes are the CLI-only commands' result types, for --schema.
var resultTypes = map[*cobra.Command]reflect.Type{}

// resultIs records T as cmd's result type, for --schema.
func resultIs[T any](cmd *cobra.Command) { resultTypes[cmd] = reflect.TypeFor[T]() }

// commandSchema is what --schema writes: a command's input and output, each a
// JSON Schema (2020-12), inlined as discovery inlines an op's
// (docs/designs/commands-and-api.md#cli-output). Each input field is a flag,
// --<field>, in kebab or snake case, unless its x-cli says otherwise, as an
// op's cli tag does: positional, and stdin when it's read from stdin unless
// given.
type commandSchema struct {
	Command string  `json:"command"`
	Input   ordered `json:"input"`
	Output  ordered `json:"output"`
}

// schemaOf is cmd's commandSchema: a core command's from its op, a CLI-only
// command's from its flags and its result type.
func schemaOf(cmd *cobra.Command) (commandSchema, error) {
	rt := app.NewRuntime()
	s := commandSchema{Command: cmd.CommandPath()}
	if name := cmd.Annotations[opAnnotation]; name != "" {
		c, _ := app.Find(name)
		for _, o := range app.NewRegistry().Ops() {
			if o.Meta().Name != command.OpName(name) {
				continue
			}
			in, err := rt.Inline(o.InSchema(rt))
			if err != nil {
				return s, err
			}
			out, err := rt.Inline(rt.Schema(o.OutType()))
			if err != nil {
				return s, err
			}
			argsOn(in, c.Args())
			s.Input, s.Output = orderSchema(in, nil, c.Args()), orderSchema(out, o.OutType(), nil)
			return s, nil
		}
	}
	s.Input, s.Output = orderSchema(flagsSchema(cmd), nil, nil), orderSchema(map[string]any{"type": "object"}, nil, nil)
	if t, ok := resultTypes[cmd]; ok {
		out, err := rt.Inline(rt.Schema(t))
		if err != nil {
			return s, err
		}
		s.Output = orderSchema(out, t, nil)
	}
	return s, nil
}

// argsOn marks each of args' properties of schema s that isn't a flag with
// x-cli, as its cli tag says: positional, stdin.
func argsOn(s map[string]any, args []command.Arg) {
	props, _ := s["properties"].(map[string]any)
	for _, a := range args {
		p, ok := props[a.Name].(map[string]any)
		if !ok || !a.Positional {
			continue
		}
		cli := []string{"positional"}
		if a.Stdin {
			cli = append(cli, "stdin")
		}
		p["x-cli"] = cli
	}
}

// positionalArg is a positional in a command's use line: [dir], <org URL>.
var positionalArg = regexp.MustCompile(`[\[<]+([^\]>]+)[\]>]+`)

// flagsSchema is a CLI-only command's input, from its flags and positionals:
// each by its name, snake_case, the flag being its kebab-case form, a
// positional marked x-cli. The
// root's flags, and hidden ones (former names), are left out.
func flagsSchema(cmd *cobra.Command) map[string]any {
	props := map[string]any{}
	_, rest, _ := strings.Cut(cmd.Use, " ")
	for _, m := range positionalArg.FindAllStringSubmatch(rest, -1) {
		name := strings.TrimSpace(m[1])
		props[snakeCase(name)] = map[string]any{"type": "string", "x-cli": []string{"positional"}}
	}
	cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		p := map[string]any{"description": f.Usage}
		switch f.Value.Type() {
		case "bool":
			p["type"] = "boolean"
		case "int", "int32", "int64", "uint", "uint64":
			p["type"] = "integer"
		case "stringSlice", "stringArray":
			p["type"], p["items"] = "array", map[string]any{"type": "string"}
		default:
			p["type"] = "string"
		}
		props[snakeCase(f.Name)] = p
	})
	return map[string]any{"type": "object", "additionalProperties": false, "properties": props}
}

// ordered is a JSON object written in its keys' order: a schema reads as its
// result does, rather than in a map's.
type ordered struct {
	keys   []string
	values map[string]any
}

func (o ordered) MarshalJSONTo(enc *jsontext.Encoder) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	for _, k := range o.keys {
		if err := enc.WriteToken(jsontext.String(k)); err != nil {
			return err
		}
		if err := jsonkit.MarshalEncode(enc, o.values[k]); err != nil {
			return err
		}
	}
	return enc.WriteToken(jsontext.EndObject)
}

// keywordOrder is the order a schema's keywords are written in: what it is,
// then its parts; the rest after, sorted.
var keywordOrder = []string{"type", "description", "x-cli", "enum", "default", "format", "required", "properties", "items", "additionalProperties"}

// orderSchema is schema s ordered: its keywords by keywordOrder, its
// properties by t's fields (a struct, through pointers and slices) or, for
// an input, by args, as the command takes them; any other after, sorted.
func orderSchema(s map[string]any, t reflect.Type, args []command.Arg) ordered {
	o := ordered{values: map[string]any{}}
	for _, k := range keysOrdered(s, keywordOrder) {
		v := s[k]
		switch k {
		case "properties":
			props, _ := v.(map[string]any)
			names := fieldNames(t)
			fieldTypes := fieldTypes(t)
			for _, a := range args {
				names = append(names, a.Name)
			}
			po := ordered{values: map[string]any{}}
			for _, name := range keysOrdered(props, names) {
				p, _ := props[name].(map[string]any)
				var sub []command.Arg
				for _, a := range args {
					if a.Name == name {
						sub = a.Fields
					}
				}
				po.keys, po.values[name] = append(po.keys, name), orderSchema(p, fieldTypes[name], sub)
			}
			v = po
		case "items":
			if p, ok := v.(map[string]any); ok {
				var elem reflect.Type
				if t = deref(t); t != nil && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
					elem = t.Elem()
				}
				v = orderSchema(p, elem, nil)
			}
		}
		o.keys, o.values[k] = append(o.keys, k), v
	}
	return o
}

// keysOrdered is m's keys: first those in order, as they come there, then
// the rest, sorted.
func keysOrdered[V any](m map[string]V, order []string) []string {
	var keys []string
	for _, k := range order {
		if _, ok := m[k]; ok && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// fieldNames are struct t's JSON names, in its fields' order, embedded ones
// inlined; none when t isn't a struct.
func fieldNames(t reflect.Type) []string {
	var names []string
	for name := range fieldsOf(t) {
		names = append(names, name)
	}
	return names
}

// fieldTypes are struct t's fields' types, by JSON name.
func fieldTypes(t reflect.Type) map[string]reflect.Type {
	types := map[string]reflect.Type{}
	for name, ft := range fieldsOf(t) {
		types[name] = ft
	}
	return types
}

// fieldsOf is struct t's fields by JSON name, in order, embedded ones
// inlined, as encoding/json sees them.
func fieldsOf(t reflect.Type) iter.Seq2[string, reflect.Type] {
	return func(yield func(string, reflect.Type) bool) {
		t = deref(t)
		if t == nil || t.Kind() != reflect.Struct {
			return
		}
		for i := range t.NumField() {
			f := t.Field(i)
			name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" || !f.IsExported() {
				continue
			}
			if f.Anonymous && name == "" || strings.Contains(opts, "inline") {
				for n, ft := range fieldsOf(f.Type) {
					if !yield(n, ft) {
						return
					}
				}
				continue
			}
			if name == "" {
				name = f.Name
			}
			if !yield(name, f.Type) {
				return
			}
		}
	}
}

func deref(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func snakeCase(name string) string {
	return strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(name))
}

// schemaInstead makes cmd and the commands under it write their schema, with
// --schema, rather than run: their args aren't checked, nothing runs.
func schemaInstead(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		schemaInstead(c)
	}
	if cmd.RunE == nil {
		return
	}
	run, validate := cmd.RunE, cmd.Args
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if !output.schema {
			return run(c, args)
		}
		s, err := schemaOf(c)
		if err != nil {
			return err
		}
		return writeStructured(c.OutOrStdout(), s)
	}
	if validate != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			if output.schema {
				return nil
			}
			return validate(c, args)
		}
	}
}
