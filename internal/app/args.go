package app

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apperr"
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
//	Dataset string `arg:"dataset" short:"d" group:"target" usage:"the dataset to query"`
//
// Tags:
//
//	arg       the name: the /call key; the CLI flag is the name with _ as -. Required.
//	usage     the help text.
//	short     a one-letter CLI shorthand.
//	alias     other names, comma-separated: extra CLI flags and /call keys, folded into
//	          the name.
//	required  "true": a string arg that must be set (non-blank).
//	enum      the allowed values of a string arg, comma-separated.
//	default   a string arg's value when unset; one of its enum, if it has one.
//	group     a group name: exactly one arg of the group must be set.
//	cli       "positional": the CLI takes it as trailing args (one string, or all of
//	          them for a []string); "stdin": the CLI reads it from piped stdin when
//	          unset. Comma-separated.
//
// Fields without an arg tag are not args.
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
		name, ok := f.Tag.Lookup("arg")
		if !ok {
			continue
		}
		a := Arg{Name: name, Usage: f.Tag.Get("usage"), Shorthand: f.Tag.Get("short"), Default: f.Tag.Get("default"),
			Group: f.Tag.Get("group"), Required: f.Tag.Get("required") == "true", field: i}
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
			return nil, fmt.Errorf("arg %s: only a string arg can be required, closed, defaulted or read from stdin", name)
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

// decode reads args — a /call body's, or the CLI's flags — into an In, and
// refuses what the command does not take: unknown args, values of the wrong
// type, required ones left unset, values outside an enum, and groups with not
// exactly one set. Every refusal is in one invalid_args error, a violation each.
//
// Aliases are folded into their names first, so a command never sees one.
// help is every command's, and ignored here.
func decode[In any](command string, specs []Arg, args map[string]any) (In, error) {
	var in In
	v := reflect.ValueOf(&in).Elem()
	known := map[string]bool{"help": true}
	vals := map[string]any{}
	for _, a := range specs {
		known[a.Name] = true
		vals[a.Name] = args[a.Name]
		for _, alias := range a.Aliases {
			known[alias] = true
			if av, ok := args[alias]; ok && isZero(vals[a.Name]) {
				vals[a.Name] = av
			}
		}
	}

	var bad apperr.Violations
	var msgs []string
	refuse := func(field, code, msg string) {
		bad = append(bad, apperr.Violation{Field: field, Code: code, Message: msg})
		msgs = append(msgs, msg)
	}

	var unknown []string
	for k := range args {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		bad = append(bad, apperr.Violation{Field: k, Code: "unknown", Message: "Not an arg of " + command + "."})
	}
	if len(unknown) > 0 {
		msgs = append(msgs, fmt.Sprintf(`unknown arg(s) %s for command %q; send {"command": %q, "help": true} for its args`,
			strings.Join(unknown, ", "), command, command))
	}

	set := map[string][]string{} // group -> its args that are set
	for _, a := range specs {
		f := v.Field(a.field)
		raw := vals[a.Name]
		switch a.Type {
		case ArgString:
			s, ok := raw.(string)
			if raw != nil && !ok {
				refuse(a.Name, "invalid", a.Name+" must be a string")
				continue
			}
			if strings.TrimSpace(s) == "" {
				s = a.Default
			}
			switch {
			case a.Required && strings.TrimSpace(s) == "":
				refuse(a.Name, "required", a.Name+" is required")
			case s != "" && len(a.Enum) > 0 && !slices.Contains(a.Enum, s):
				refuse(a.Name, "invalid", fmt.Sprintf("%s must be one of: %s", a.Name, strings.Join(a.Enum, ", ")))
			}
			f.SetString(s)
		case ArgBool:
			b, ok := raw.(bool)
			if raw != nil && !ok {
				refuse(a.Name, "invalid", a.Name+" must be true or false")
				continue
			}
			f.SetBool(b)
		case ArgStringArray:
			ss, ok := stringList(raw)
			if !ok {
				refuse(a.Name, "invalid", a.Name+" must be a list of strings")
				continue
			}
			f.Set(reflect.ValueOf(ss))
		}
		if a.Group != "" && !isZero(f.Interface()) {
			set[a.Group] = append(set[a.Group], a.Name)
		}
	}
	for _, g := range groupNames(specs) {
		members := groupMembers(specs, g)
		switch n := len(set[g]); {
		case n == 0:
			refuse(members[0], "required", "one of "+strings.Join(members, ", ")+" is required")
		case n > 1:
			refuse(set[g][1], "invalid", "only one of "+strings.Join(set[g], ", ")+" may be set")
		}
	}

	if len(bad) > 0 {
		return in, apperr.NewWith(errcode.InvalidArgs, strings.Join(msgs, "; "), bad)
	}
	return in, nil
}

// stringList reads a list of strings: []string from the CLI, []any from JSON.
func stringList(raw any) ([]string, bool) {
	switch x := raw.(type) {
	case nil:
		return nil, true
	case []string:
		return x, true
	case []any:
		out := make([]string, len(x))
		for i, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

func isZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case bool:
		return !x
	case []string:
		return len(x) == 0
	case []any:
		return len(x) == 0
	}
	return false
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
