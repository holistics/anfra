// Package appstep defines the steps a request goes through: what a step is, and
// how it describes itself.
//
// A step is a formal operation: defined once with a name and whether it is
// public, then run wherever the operation runs (see apptracing). It feeds two
// things at once — a span in the trace, and context on any error that passes
// through it — so the operation trail is written once. There is no way to add
// informal, ad hoc context: context first means formalising the step.
//
// A public step carries a fragment of user-facing text, and appears in the
// context of an error's response. A private step appears only in the log and
// the trace. Making a step public later is a change to its definition, never to
// the code that runs it.
//
// Two types, one per concept: a Def is a step's definition, made once by Define;
// a Step is one occurrence — a Def and the parameters it ran with. "Step" always
// means an occurrence, as in "the steps an error passed through".
//
// Every step belongs to a namespace, one per module, and is named by it
// everywhere — the log, the trace and the context of a response:
// anfra_cloud.import_dashboard. A library's public steps reach its host's
// clients as they are, so the qualified name keeps two modules' steps apart
// without either knowing the other. apperr names its codes by the same
// namespaces.
//
// This package knows nothing of errors, responses or tracing. A Step describes
// itself (String and Logged for the log and the trace, Describe for a user); apperr decides how that
// appears in a response, and apptracing runs steps and attaches them to errors.
package appstep

import (
	"errors"
	"fmt"
	"iter"
	"regexp"
	"strconv"
	"strings"
)

// Namespace is a module's namespace, for its steps and its error codes (see
// apperr): a name is unique within it. Only DefineNamespace makes one.
type Namespace struct{ name string }

var (
	namespaces    = map[string]bool{}
	namespaceName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// DefineNamespace declares a module's namespace. Call it once per module, from a
// package-level var. A name is snake_case, as step and code names are —
// anfra_cloud — so a qualified name splits at its first dot; another name, or
// a duplicate, panics at startup.
func DefineNamespace(name string) Namespace {
	if !namespaceName.MatchString(name) {
		panic("appstep: namespace " + strconv.Quote(name) + " is not snake_case")
	}
	if namespaces[name] {
		panic("appstep: duplicate namespace " + name)
	}
	namespaces[name] = true
	return Namespace{name}
}

// String is the namespace's name. The zero Namespace has none.
func (ns Namespace) String() string { return ns.name }

type definition struct {
	name      string          // qualified: namespace.name
	template  string          // user-facing fragment; empty for a private step
	sensitive map[string]bool // parameter keys kept out of telemetry
}

// Def is a step's definition: a handle to it. Only Define makes one.
type Def struct{ d *definition }

// String is the step's qualified name.
func (d Def) String() string { return d.d.name }

// config is what the options gather for Define to check and build a definition from.
// It lives only while Define runs, so the definition holds finished state only.
type config struct {
	template  string
	sensitive []string
}

// Option configures a step at its definition.
type Option func(*config)

// Public makes a step appear in the context of an error's response, as
// template filled from the step's parameters: a lowercase verb phrase naming
// the attempt, with no punctuation — "import dashboard {name}". It names what
// was being done, never why it failed.
func Public(template string) Option {
	return func(c *config) { c.template = template }
}

var defined = map[string]Def{}

// Sensitive declares parameters, by key, whose values must not reach telemetry
// — an email, a name a person typed. Every run of the step redacts them from the
// log form and the trace, so no call site can forget; they may still appear in
// the user-facing description, since they came from the user.
func Sensitive(keys ...string) Option {
	return func(c *config) { c.sensitive = append(c.sensitive, keys...) }
}

// Define declares a step of ns. Call it from a package-level var, once per
// name; a duplicate name in ns, a public template that does not parse, or an
// empty or repeated Sensitive key panics at startup. Without Public, the step
// is private.
//
// Names are unique because a public step's qualified name reaches clients, as
// the step of a context entry, and a client localising its text keys on it.
func Define(ns Namespace, name string, opts ...Option) Def {
	if ns.name == "" {
		panic("appstep: step " + name + " has no namespace")
	}
	if name == "" {
		panic("appstep: empty step name")
	}
	name = ns.name + "." + name
	if _, dup := defined[name]; dup {
		panic("appstep: duplicate step " + name)
	}
	var c config
	for _, o := range opts {
		o(&c)
	}
	if c.template != "" {
		if err := checkTemplate(c.template); err != nil {
			panic("appstep: step " + name + ": " + err.Error())
		}
	}
	d := &definition{name: name, template: c.template, sensitive: make(map[string]bool, len(c.sensitive))}
	for _, k := range c.sensitive {
		if k == "" || d.sensitive[k] {
			panic("appstep: step " + name + ": empty or repeated sensitive key " + strconv.Quote(k))
		}
		d.sensitive[k] = true
	}
	def := Def{d}
	defined[name] = def
	return def
}

type param struct {
	key   string
	value any
}

// Step is one occurrence of a step: its definition and the parameters it ran
// with. It is what an error holds for each step it passed through.
type Step struct {
	def    Def
	params []param
}

// NewStep is an occurrence of def with kv, key/value pairs as in slog. Which of
// them are sensitive is the definition's declaration (Sensitive), not the
// caller's. A malformed pair is kept, visibly, under a bad key, rather than
// panicking inside a request.
func NewStep(def Def, kv ...any) Step {
	return Step{def: def, params: pairs(kv)}
}

// redacted stands in for a sensitive value in the log form, so an operator can
// see the parameter was there without seeing it.
const redacted = "[redacted]"

// Name is the step's qualified name.
func (s Step) Name() string { return s.def.d.name }

// Logged yields the parameters as the log and the trace see them, in the order
// given, with sensitive values redacted.
func (s Step) Logged() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for _, p := range s.params {
			v := any(redacted)
			if !s.def.d.sensitive[p.key] {
				v = p.value
			}
			if !yield(p.key, v) {
				return
			}
		}
	}
}

// String is the log form, with sensitive values redacted:
// anfra_cloud.import_dashboard(name=Sales, owner=[redacted]).
func (s Step) String() string {
	if len(s.params) == 0 {
		return s.Name()
	}
	var parts []string
	for k, v := range s.Logged() {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return s.Name() + "(" + strings.Join(parts, ", ") + ")"
}

// Description is how a step describes itself to a user: its public template,
// filled from the parameters it ran with.
type Description struct {
	Name   string         // the step's qualified name; stable, since a localising client keys on it
	Params map[string]any // only the parameters the template names; nil if none
	Text   string         // the filled template: "import dashboard 'Sales'"
}

// Describe returns s's description, or false when its definition is private, or
// when its template names a parameter s was not given: never shown half-filled.
// String values are quoted in single quotes; others render with %v.
func (s Step) Describe() (Description, bool) {
	tmpl := s.def.d.template
	if tmpl == "" {
		return Description{}, false
	}
	values := make(map[string]any, len(s.params))
	for _, p := range s.params {
		values[p.key] = p.value
	}
	used := map[string]any{}
	missing := false
	text := placeholder.ReplaceAllStringFunc(tmpl, func(m string) string {
		key := m[1 : len(m)-1]
		v, ok := values[key]
		if !ok {
			missing = true
			return m
		}
		used[key] = v
		if str, ok := v.(string); ok {
			return "'" + str + "'"
		}
		return fmt.Sprint(v)
	})
	if missing {
		return Description{}, false
	}
	d := Description{Name: s.def.d.name, Text: text}
	if len(used) > 0 {
		d.Params = used
	}
	return d, true
}

var placeholder = regexp.MustCompile(`\{([a-z_][a-z0-9_]*)\}`)

// checkTemplate rejects a step template that is empty or has a brace that is
// not a well-formed placeholder, which would otherwise reach callers literally.
func checkTemplate(t string) error {
	if t == "" {
		return errors.New("empty template")
	}
	if strings.ContainsAny(placeholder.ReplaceAllString(t, ""), "{}") {
		return fmt.Errorf("malformed placeholder in %q", t)
	}
	return nil
}

// badKey stands in for a missing or non-string key, as in slog.
const badKey = "!BADKEY"

// pairs reads kv the way slog does: a string followed by a value is a pair; a
// non-string, or a string with nothing after it, is a value under badKey.
func pairs(kv []any) []param {
	var out []param
	for len(kv) > 0 {
		key, ok := kv[0].(string)
		if !ok || len(kv) == 1 {
			out = append(out, param{key: badKey, value: kv[0]})
			kv = kv[1:]
			continue
		}
		out = append(out, param{key: key, value: kv[1]})
		kv = kv[2:]
	}
	return out
}
