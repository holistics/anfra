package decode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/validation"
	"github.com/holistics/anfra/shared/apperr"
)

// huma's validation errors carry no machine code, report a missing property at its
// parent, and read tersely. This file turns each into a Violation: our code, the
// caller's field path, and a message written for an agent to act on. The message
// never repeats the path: a client shows it under the field, and a batch rewrites
// only `field`.
//
// It matches against huma's own message constants, never ad hoc substrings: each rule
// is compiled from the constant, so it matches exactly what huma emits. A huma upgrade
// that rewords a constant still matches; one that removes a constant fails to compile;
// and the golden table in violations_test.go pins every message we produce.
//
// Values are never echoed back, except where they are codes the caller chose from a
// closed set (enum, const). A failed minLength on a password must not return the
// password.

type rule struct {
	format string
	code   string
	// render builds the message. field is the caller's path ("" at the root); args
	// are the format's arguments as huma filled them in.
	render func(field string, args []string, value any) string
	re     *regexp.Regexp
}

var rules = compile([]rule{
	{format: validation.MsgExpectedRequiredProperty, code: "required",
		render: func(string, []string, any) string { return "Required." }},
	{format: validation.MsgUnexpectedProperty, code: "unknown",
		render: func(string, []string, any) string {
			return "Not a field of this input. Field names are snake_case and case-sensitive."
		}},
	{format: validation.MsgExpectedOneOf, code: "invalid",
		render: func(f string, a []string, v any) string {
			return must(f, fmt.Sprintf("be one of: %s; got %s.", a[0], show(v)))
		}},
	{format: validation.MsgExpectedConst, code: "invalid",
		render: func(f string, a []string, v any) string {
			return must(f, fmt.Sprintf("be %s; got %s.", a[0], show(v)))
		}},

	{format: validation.MsgExpectedMinLength, code: "too_short",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("be at least %s %s long.", a[0], plural(a[0], "character")))
		}},
	{format: validation.MsgExpectedMaxLength, code: "too_long",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("be at most %s %s long.", a[0], plural(a[0], "character")))
		}},
	{format: validation.MsgExpectedMinItems, code: "too_short",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("have at least %s %s.", a[0], plural(a[0], "item")))
		}},
	{format: validation.MsgExpectedMaxItems, code: "too_long",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("have at most %s %s.", a[0], plural(a[0], "item")))
		}},
	{format: validation.MsgExpectedArrayItemsUnique, code: "invalid",
		render: func(f string, _ []string, _ any) string { return must(f, "not contain duplicates.") }},

	// Number bounds use "invalid": error_handling.md's generic set has no
	// too_small/too_large, and a client handles them as any invalid field.
	{format: validation.MsgExpectedMinimumNumber, code: "invalid",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("be at least %s.", a[0]))
		}},
	{format: validation.MsgExpectedMaximumNumber, code: "invalid",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("be at most %s.", a[0]))
		}},
	{format: validation.MsgExpectedExclusiveMinimumNumber, code: "invalid",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("be greater than %s.", a[0]))
		}},
	{format: validation.MsgExpectedExclusiveMaximumNumber, code: "invalid",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("be less than %s.", a[0]))
		}},
	{format: validation.MsgExpectedNumberBeMultipleOf, code: "invalid",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("be a multiple of %s.", a[0]))
		}},

	{format: validation.MsgExpectedString, code: "invalid", render: typeRule("a string")},
	{format: validation.MsgExpectedInteger, code: "invalid", render: typeRule("an integer")},
	{format: validation.MsgExpectedNumber, code: "invalid", render: typeRule("a number")},
	{format: validation.MsgExpectedBoolean, code: "invalid", render: typeRule("true or false")},
	{format: validation.MsgExpectedArray, code: "invalid", render: typeRule("a list")},
	{format: validation.MsgExpectedObject, code: "invalid", render: typeRule("an object")},

	{format: validation.MsgExpectedRFC5322Email, code: "invalid", render: formatRule("an email address")},
	{format: validation.MsgExpectedRFC5322EmailBare, code: "invalid", render: formatRule("a bare email address, with no display name")},
	{format: validation.MsgExpectedRFC3339DateTime, code: "invalid", render: formatRule("an RFC 3339 date-time, e.g. 2026-10-02T09:30:00Z")},
	{format: validation.MsgExpectedRFC3339Date, code: "invalid", render: formatRule("an RFC 3339 date, e.g. 2026-10-02")},
	{format: validation.MsgExpectedRFC4122UUID, code: "invalid", render: formatRule("a UUID")},
	{format: validation.MsgExpectedRFC3986URI, code: "invalid", render: formatRule("a URI")},
	{format: validation.MsgExpectedMatchPattern, code: "invalid",
		render: func(f string, a []string, _ any) string {
			return must(f, fmt.Sprintf("match the pattern %s.", a[0]))
		}},
})

// compile turns each rule's format into an anchored pattern: literal text quoted,
// each verb a capture group.
func compile(rs []rule) []rule {
	verb := regexp.MustCompile(`%[dvs]`)
	for i := range rs {
		parts := verb.Split(rs[i].format, -1)
		verbs := verb.FindAllString(rs[i].format, -1)
		var b strings.Builder
		b.WriteString("^")
		for j, p := range parts {
			b.WriteString(regexp.QuoteMeta(p))
			if j < len(verbs) {
				if verbs[j] == "%d" {
					b.WriteString(`(-?\d+)`)
				} else {
					b.WriteString(`(.*)`)
				}
			}
		}
		b.WriteString("$")
		rs[i].re = regexp.MustCompile(b.String())
	}
	return rs
}

// violations maps huma's errors. groups are the input's exactly-one groups
// (ExactlyOne): huma reports a group set wrong as the whole input failing to
// match one schema, so those errors are replaced by one violation per field to
// fix, worked out from the input itself.
func violations(errs []error, groups [][]string, input any) []apperr.Violation {
	out := make([]apperr.Violation, 0, len(errs))
	for _, err := range errs {
		d, ok := err.(*huma.ErrorDetail)
		if !ok {
			out = append(out, apperr.Violation{Code: "invalid", Message: err.Error()})
			continue
		}
		if len(groups) > 0 && d.Location == "" && strings.HasPrefix(d.Message, oneOfPrefix) {
			continue
		}
		out = append(out, violation(d))
	}
	if obj, ok := input.(map[string]any); ok {
		for _, g := range groups {
			out = append(out, exactlyOne(g, obj)...)
		}
	}
	return out
}

// oneOfPrefix begins both of huma's messages for a value matching none, or more
// than one, of a oneOf's schemas. Only the first is a constant.
var oneOfPrefix = strings.TrimSuffix(validation.MsgExpectedMatchExactlyOneSchema, "none")

// ExactlyOne is the schema of a group of fields of which exactly one must be set,
// for an object's schema to carry in OneOf (or, for several groups, one per AllOf
// entry): a oneOf over each field being required. A JSON Schema reader sees the
// rule; apikit reads the group back from it, to say which fields to set.
func ExactlyOne(fields []string) []*huma.Schema {
	subs := make([]*huma.Schema, len(fields))
	for i, f := range fields {
		// huma checks only the required properties a schema lists, so the field is
		// listed too, as anything: its own schema is the object's.
		subs[i] = &huma.Schema{Type: huma.TypeObject, Required: []string{f}, Properties: map[string]*huma.Schema{f: {}}}
	}
	return subs
}

// exactlyOneGroups reads back the groups ExactlyOne wrote into s.
func exactlyOneGroups(reg huma.Registry, s *huma.Schema) [][]string {
	for s != nil && s.Ref != "" {
		s = reg.SchemaFromRef(s.Ref)
	}
	if s == nil {
		return nil
	}
	var groups [][]string
	for _, oneOf := range append([]*huma.Schema{s}, s.AllOf...) {
		if g, ok := groupOf(oneOf.OneOf); ok {
			groups = append(groups, g)
		}
	}
	return groups
}

func groupOf(subs []*huma.Schema) ([]string, bool) {
	if len(subs) == 0 {
		return nil, false
	}
	g := make([]string, len(subs))
	for i, sub := range subs {
		if len(sub.Required) != 1 || len(sub.Properties) != 1 || sub.Properties[sub.Required[0]] == nil {
			return nil, false
		}
		g[i] = sub.Required[0]
	}
	return g, true
}

// exactlyOne is what is wrong with group in obj: none set, at the group's first
// field, or more than one, at each after the first.
func exactlyOne(group []string, obj map[string]any) []apperr.Violation {
	var set []string
	for _, f := range group {
		if _, ok := obj[f]; ok {
			set = append(set, f)
		}
	}
	list := strings.Join(group, ", ")
	switch {
	case len(set) == 0:
		return []apperr.Violation{{Field: group[0], Code: "required", Message: "Set exactly one of: " + list + "."}}
	case len(set) > 1:
		out := make([]apperr.Violation, 0, len(set)-1)
		for _, f := range set[1:] {
			out = append(out, apperr.Violation{Field: f, Code: "invalid", Message: "Set only one of: " + list + "."})
		}
		return out
	}
	return nil
}

func violation(d *huma.ErrorDetail) apperr.Violation {
	for _, r := range rules {
		m := r.re.FindStringSubmatch(d.Message)
		if m == nil {
			continue
		}
		field, args := d.Location, m[1:]
		if r.format == validation.MsgExpectedRequiredProperty {
			// Reported at the parent; the missing property is named in the message.
			field = join(field, args[0])
		}
		return apperr.Violation{Field: field, Code: r.code, Message: r.render(field, args, d.Value)}
	}
	// Unmatched: degrade, never drop. huma's text, with the field in front.
	return apperr.Violation{Field: d.Location, Code: "invalid", Message: sentence(d.Message)}
}

func join(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

// must completes "must <rest>" as a sentence. A violation's message never contains its
// field path — `field` says where — so it reads "Must …", except at the root, where
// there is no field to show it under, and it reads "The input must …".
func must(field, rest string) string {
	if field == "" {
		return "The input must " + rest
	}
	return "Must " + rest
}

func sentence(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func typeRule(want string) func(string, []string, any) string {
	return func(f string, _ []string, v any) string {
		return must(f, fmt.Sprintf("be %s; got %s.", want, kind(v)))
	}
}

func formatRule(want string) func(string, []string, any) string {
	return func(f string, _ []string, _ any) string { return must(f, fmt.Sprintf("be %s.", want)) }
}

// kind names a JSON value's type without echoing the value.
func kind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case float64, int, int64, json.Number:
		return "a number"
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	}
	return "a value of the wrong type"
}

// show echoes a value from a closed set (enum, const), which is never a secret.
func show(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return kind(v)
	}
	return string(b)
}

func plural(n, word string) string {
	if n == "1" {
		return word
	}
	return word + "s"
}
