package app

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apikit"
)

// ArgType is the type of a command argument, from its field's Go type.
type ArgType string

const (
	ArgString      ArgType = "string"       // a string field
	ArgBool        ArgType = "bool"         // a bool field
	ArgStringArray ArgType = "string_array" // a []string field
)

// Arg is one of a command's args, parsed from a field of its In struct:
//
//	Dataset string `json:"dataset,omitempty" short:"d" group:"target" doc:"the dataset to query"`
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
//	          says so as a oneOf (argsSchema).
//
// and the CLI's alone, which the schema does not see:
//
//	short     a one-letter CLI shorthand.
//	alias     other CLI flag names, comma-separated, folded into the name.
//	cli       "positional": the CLI takes it as trailing args (one string, or all of
//	          them for a []string); "stdin": the CLI reads it from piped stdin when
//	          unset. Comma-separated.
//
// Fields without a json tag are not args. An In with args implements
// huma.SchemaTransformer with argsSchema, so its groups and required strings are
// in its schema.
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

	field int // the field's index in In
}

// Flag is the arg's CLI flag name: its name with _ as -.
func (a Arg) Flag() string { return strings.ReplaceAll(a.Name, "_", "-") }

func cloneArgs(args []Arg) []Arg {
	out := slices.Clone(args)
	for i := range out {
		out[i].Aliases = slices.Clone(out[i].Aliases)
		out[i].Enum = slices.Clone(out[i].Enum)
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
	names := map[string]bool{"help": true} // help is every command's
	shorts := map[string]bool{}
	groups := map[string]int{}
	positional := 0
	for i := range t.NumField() {
		f := t.Field(i)
		tag, ok := f.Tag.Lookup("json")
		if !ok || tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		a := Arg{Name: name, Usage: f.Tag.Get("doc"), Shorthand: f.Tag.Get("short"), Default: f.Tag.Get("default"),
			Group: f.Tag.Get("group"), Required: !slices.Contains(strings.Split(opts, ","), "omitempty"), field: i}
		switch f.Type {
		case reflect.TypeFor[string]():
			a.Type = ArgString
		case reflect.TypeFor[bool]():
			a.Type = ArgBool
		case reflect.TypeFor[[]string]():
			a.Type = ArgStringArray
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
			if names[n] {
				return nil, fmt.Errorf("arg %s: the name %q is taken", name, n)
			}
			names[n] = true
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

// argsSchema completes an In's schema with what huma cannot read from its tags:
// each group as a oneOf over its args being set (apikit names the args to fix),
// and a required string's minimum length, so a blank one is refused as unset.
// Each In with args calls it from its TransformSchema.
func argsSchema[In any](s *huma.Schema) *huma.Schema {
	args, err := parseArgs(reflect.TypeFor[In]())
	if err != nil {
		panic(fmt.Sprintf("app: %s: %v", reflect.TypeFor[In](), err)) // Define refused it already
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
// publishes defaults but validation does not apply them, so the handler does.
func applyDefaults(args []Arg, in any) {
	v := reflect.ValueOf(in).Elem()
	for _, a := range args {
		if f := v.Field(a.field); a.Default != "" && strings.TrimSpace(f.String()) == "" {
			f.SetString(a.Default)
		}
	}
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
