package command

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apikit"
)

// ArgType is the type of a command argument, from its field's Go type.
type ArgType string

const (
	ArgString      ArgType = "string"       // a string field
	ArgBool        ArgType = "bool"         // a bool field
	ArgStringArray ArgType = "string_array" // a []string field
	ArgInt         ArgType = "int"          // an int field
	ArgObject      ArgType = "object"       // a struct, held by value; the CLI takes it as JSON, or by its fields
	ArgObjectArray ArgType = "object_array" // a slice of structs; the CLI takes it as JSON
)

// Arg is one of a command's args, parsed from a field of its In struct:
//
//	Dataset string `json:"dataset,omitempty" short:"d" doc:"the dataset an AQL query runs against"`
//
// In is also the op's input type, so its schema — validated by apikit, published
// in OpenAPI — is huma's reading of the same tags. Tags:
//
//	json      the name, the op input's field; the CLI flag is the name with _ as -.
//	          Without omitempty the arg is required (a string: non-blank); only a
//	          string arg can be. Required.
//	doc       the help text: the schema's description and the CLI flag's usage.
//	enum      the allowed values of a string arg, comma-separated.
//	default   a string arg's value when unset; one of its enum, if it has one.
//	group     a group name: exactly one arg of the group must be set. The schema
//	          says so as a oneOf (ArgsSchema).
//
// and the CLI's alone, which the schema does not see:
//
//	short     a one-letter CLI shorthand.
//	alias     other CLI flag names, comma-separated, folded into the name.
//	cli       "positional": the CLI takes it as trailing args (one string, or all of
//	          them for a []string); "stdin": the CLI reads it from piped stdin when
//	          unset. Comma-separated.
//
// An object arg's string, bool, int and []string fields are its Fields: the CLI
// sets each with a flag of its own, --<arg>.<field>, read from the field's json,
// doc and enum tags. A field holding an object, or a list of them, is set only
// through the arg's JSON. An object array arg is set only through its JSON.
//
// Fields without a json tag are not args, except an embedded struct, whose args
// are In's own (as huma and encoding/json flatten it too). An In with args
// implements huma.SchemaTransformer with ArgsSchema, so its groups and required
// strings are in its schema.
type Arg struct {
	Name       string
	Type       ArgType
	Usage      string
	Shorthand  string
	Aliases    []string
	Required   bool
	Enum       []string
	Default    string
	Group      string
	Positional bool
	Stdin      bool
	// Fields are an object arg's fields the CLI sets one by one (see Arg).
	Fields []Arg

	field []int // the field's index path in In
}

// Flag is the arg's CLI flag name (FlagName).
func (a Arg) Flag() string { return FlagName(a.Name) }

// FlagName is a field's CLI name: its name, snake_case or camelCase, in
// lowercase with hyphens, each level of a dotted path on its own:
// input.pagination.pageSize is input.pagination.page-size
// (docs/designs/commands-and-api.md#one-name-per-field).
func FlagName(name string) string {
	var b strings.Builder
	prev := rune(0)
	for _, r := range name {
		switch {
		case r == '_':
			b.WriteRune('-')
		case unicode.IsUpper(r):
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				b.WriteRune('-')
			}
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
		prev = r
	}
	return b.String()
}

// FlagKey is how the CLI matches a flag's name: ignoring case, _ and -. Two
// fields in one place whose keys are equal are refused, so a flag typed any of
// those ways reaches one field.
func FlagKey(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '-' {
			return -1
		}
		return unicode.ToLower(r)
	}, name)
}

func cloneArgs(args []Arg) []Arg {
	out := slices.Clone(args)
	for i := range out {
		out[i].Aliases = slices.Clone(out[i].Aliases)
		out[i].Enum = slices.Clone(out[i].Enum)
		out[i].field = slices.Clone(out[i].field)
		out[i].Fields = cloneArgs(out[i].Fields)
	}
	return out
}

// parseArgs reads In's args from its field tags, and refuses a malformed set:
// a type it cannot decode, a constraint on the wrong type, a repeated name, or
// a group of one.
func parseArgs(t reflect.Type) ([]Arg, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("In must be a struct, not %s", t)
	}
	var args []Arg
	names := map[string]bool{"help": true} // by FlagKey; help is every command's
	shorts := map[string]bool{}
	groups := map[string]int{}
	positional := 0
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous {
			if f.Type.Kind() != reflect.Struct {
				return nil, fmt.Errorf("field %s: only a struct can be embedded, not %s", f.Name, f.Type)
			}
			continue // its fields are visited themselves
		}
		tag, ok := f.Tag.Lookup("json")
		if !ok || tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		a := Arg{Name: name, Usage: f.Tag.Get("doc"), Shorthand: f.Tag.Get("short"), Default: f.Tag.Get("default"),
			Group: f.Tag.Get("group"), Required: !slices.Contains(strings.Split(opts, ","), "omitempty"), field: f.Index}
		switch {
		case f.Type == reflect.TypeFor[string]():
			a.Type = ArgString
		case f.Type == reflect.TypeFor[bool]():
			a.Type = ArgBool
		case f.Type == reflect.TypeFor[[]string]():
			a.Type = ArgStringArray
		case f.Type == reflect.TypeFor[int]():
			a.Type = ArgInt
		case f.Type.Kind() == reflect.Struct || f.Type.Kind() == reflect.Pointer && f.Type.Elem().Kind() == reflect.Struct:
			a.Type = ArgObject
			obj := f.Type
			if obj.Kind() == reflect.Pointer {
				obj = obj.Elem()
			}
			fields, err := objectFields(obj)
			if err != nil {
				return nil, fmt.Errorf("arg %s: %w", name, err)
			}
			a.Fields = fields
		case f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.Struct:
			a.Type = ArgObjectArray
		default:
			return nil, fmt.Errorf("arg %s: unsupported type %s", name, f.Type)
		}
		if v := f.Tag.Get("alias"); v != "" {
			a.Aliases = strings.Split(v, ",")
		}
		if v := f.Tag.Get("enum"); v != "" {
			a.Enum = strings.Split(v, ",")
		}
		for _, opt := range strings.Split(f.Tag.Get("cli"), ",") {
			switch opt {
			case "":
			case "positional":
				a.Positional = true
				positional++
			case "stdin":
				a.Stdin = true
			default:
				return nil, fmt.Errorf("arg %s: unknown cli option %q", name, opt)
			}
		}
		switch {
		case name == "" || a.Usage == "":
			return nil, fmt.Errorf("field %s: an arg needs a name and a usage", f.Name)
		case a.Positional && a.Type != ArgString && a.Type != ArgStringArray:
			return nil, fmt.Errorf("arg %s: only a string or []string arg can be positional", name)
		case (a.Required || len(a.Enum) > 0 || a.Default != "" || a.Stdin) && a.Type != ArgString:
			return nil, fmt.Errorf("arg %s: only a string arg can be required (no omitempty), closed, defaulted or read from stdin", name)
		case a.Required && a.Default != "":
			return nil, fmt.Errorf("arg %s: a required arg has no default; add omitempty", name)
		case a.Default != "" && len(a.Enum) > 0 && !slices.Contains(a.Enum, a.Default):
			return nil, fmt.Errorf("arg %s: default %q is not in its enum", name, a.Default)
		case a.Required && a.Group != "":
			return nil, fmt.Errorf("arg %s: an arg of a group is not required on its own", name)
		case len(a.Shorthand) > 1 || (a.Shorthand != "" && shorts[a.Shorthand]):
			return nil, fmt.Errorf("arg %s: shorthand %q is not one free letter", name, a.Shorthand)
		}
		for _, n := range append([]string{name}, a.Aliases...) {
			if names[FlagKey(n)] {
				return nil, fmt.Errorf("arg %s: the name %q is taken, or differs from one taken only by case, _ or -", name, n)
			}
			names[FlagKey(n)] = true
		}
		if a.Shorthand != "" {
			shorts[a.Shorthand] = true
		}
		if a.Group != "" {
			groups[a.Group]++
		}
		args = append(args, a)
	}
	if positional > 1 {
		return nil, errors.New("more than one positional arg")
	}
	for g, n := range groups {
		if n < 2 {
			return nil, fmt.Errorf("group %s has one arg: make it required instead", g)
		}
	}
	return args, nil
}

// objectFields are the fields of an object arg's struct the CLI sets one by one:
// its string, bool, int and []string fields, and an object's, at any depth (an
// object field holds its own Fields). A list of objects is left to the arg's
// JSON.
func objectFields(t reflect.Type) ([]Arg, error) {
	var fields []Arg
	keys := map[string]string{}
	for _, f := range reflect.VisibleFields(t) {
		tag, ok := f.Tag.Lookup("json")
		if f.Anonymous || !ok || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if other, taken := keys[FlagKey(name)]; taken {
			return nil, fmt.Errorf("fields %s and %s differ only by case, _ or -, so a flag can't tell them apart", other, name)
		}
		keys[FlagKey(name)] = name
		a := Arg{Name: name, Usage: f.Tag.Get("doc"), field: f.Index}
		switch obj := f.Type; {
		case obj == reflect.TypeFor[string]():
			a.Type = ArgString
		case obj == reflect.TypeFor[bool]():
			a.Type = ArgBool
		case obj == reflect.TypeFor[[]string]():
			a.Type = ArgStringArray
		case obj == reflect.TypeFor[int]():
			a.Type = ArgInt
		case obj.Kind() == reflect.Struct || obj.Kind() == reflect.Pointer && obj.Elem().Kind() == reflect.Struct:
			if obj.Kind() == reflect.Pointer {
				obj = obj.Elem()
			}
			nested, err := objectFields(obj)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", name, err)
			}
			if len(nested) == 0 {
				continue
			}
			a.Type, a.Fields = ArgObject, nested
			fields = append(fields, a)
			continue
		default:
			continue
		}
		if v := f.Tag.Get("enum"); v != "" {
			a.Enum = strings.Split(v, ",")
		}
		if name == "" || a.Usage == "" {
			return nil, fmt.Errorf("field %s: a field the CLI sets needs a name and a usage", f.Name)
		}
		fields = append(fields, a)
	}
	return fields, nil
}

// ArgsSchema completes an In's schema with what huma cannot read from its tags:
// each group as a oneOf over its args being set (apikit names the args to fix),
// and a required string's minimum length, so a blank one is refused as unset.
// Each In with args calls it from its TransformSchema.
func ArgsSchema[In any](s *huma.Schema) *huma.Schema {
	args, err := parseArgs(reflect.TypeFor[In]())
	if err != nil {
		panic(fmt.Sprintf("command: %s: %v", reflect.TypeFor[In](), err)) // Define refused it already
	}
	one := 1
	for _, a := range args {
		if a.Required && a.Type == ArgString && s.Properties[a.Name] != nil {
			s.Properties[a.Name].MinLength = &one
		}
	}
	groups := groupNames(args)
	switch len(groups) {
	case 0:
	case 1:
		s.OneOf = apikit.ExactlyOne(groupMembers(args, groups[0]))
	default:
		for _, g := range groups {
			s.AllOf = append(s.AllOf, &huma.Schema{OneOf: apikit.ExactlyOne(groupMembers(args, g))})
		}
	}
	return s
}

// applyDefaults sets each unset string arg with a default to it. The schema
// publishes defaults but validation does not apply them, so preparing the
// input does (command.prepare).
func applyDefaults(args []Arg, in any) {
	v := reflect.ValueOf(in).Elem()
	for _, a := range args {
		if f := v.FieldByIndex(a.field); a.Default != "" && strings.TrimSpace(f.String()) == "" {
			f.SetString(a.Default)
		}
	}
}

// ExactlyOne is args' groups, each as the names of its args, of which exactly
// one must be set.
func ExactlyOne(args []Arg) [][]string {
	var out [][]string
	for _, g := range groupNames(args) {
		out = append(out, groupMembers(args, g))
	}
	return out
}

// groupNames are the groups of specs, in the order their first arg appears.
func groupNames(specs []Arg) []string {
	var out []string
	for _, a := range specs {
		if a.Group != "" && !slices.Contains(out, a.Group) {
			out = append(out, a.Group)
		}
	}
	return out
}

func groupMembers(specs []Arg, group string) []string {
	var out []string
	for _, a := range specs {
		if a.Group == group {
			out = append(out, a.Name)
		}
	}
	return out
}
