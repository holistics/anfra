package apikit

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"

	"github.com/holistics/anfra/shared/apikit/internal/decode"
)

// Registry is the set of ops one server serves: called with the same Call
// (what its transports pass for each call), and admitted into the same Caller
// (whom a call is for). The composition root makes one and passes it to each
// module, which registers its ops with Register.
type Registry[Call, Caller any] struct {
	ns       apperr.Namespace
	params   func(Call) []any
	executor Executor[Caller]
	ops      []Op[Call]
	byName   map[string]Op[Call]
	groups   map[string]string // group name → its one-line summary
}

// RegistryConfig is what a registry needs of its host.
type RegistryConfig[Call, Caller any] struct {
	// Namespace is the host's: each op's private step is op.<name> in it.
	Namespace apperr.Namespace
	// StepParams are an op step's parameters for a call — who called, in which
	// org — for its log entry and span. Nil: none.
	StepParams func(call Call) []any
	// Executor executes the runs of the registry's ops, mounted ones included.
	// Nil: each run is executed now, in its call.
	Executor Executor[Caller]
}

// NewRegistry returns an empty registry.
func NewRegistry[Call, Caller any](c RegistryConfig[Call, Caller]) *Registry[Call, Caller] {
	if c.Namespace.String() == "" {
		panic("apikit: a registry needs its host's namespace")
	}
	return &Registry[Call, Caller]{ns: c.Namespace, params: c.StepParams, executor: c.Executor,
		byName: map[string]Op[Call]{}, groups: map[string]string{}}
}

// OpSet is a registry's ops as its transports serve them, whatever its Caller:
// what HTTP, MCP and Discovery take, and Mount mounts from.
type OpSet[Call any] interface {
	// Ops are the ops, in registration order.
	Ops() []Op[Call]
	// Lookup is the op named name.
	Lookup(name string) (Op[Call], bool)
	// GroupSummary is a declared group's one-line summary; "" for none.
	GroupSummary(group string) string
}

// Group declares a group of ops — the first segment of their names — with its
// one-line summary: the index level of discovery. An op is registered only into
// a declared group, so the index never shows a blank. Declaring a group twice
// panics.
func (r *Registry[Call, Caller]) Group(name, summary string) {
	switch _, dup := r.groups[name]; {
	case dup:
		panic(fmt.Sprintf("group %q: declared twice", name))
	case name == "" || strings.Contains(name, ".") || summary == "":
		panic(fmt.Sprintf("group %q: a group is one name segment, with a summary", name))
	}
	r.groups[name] = summary
}

// GroupOf is the group an op name belongs to: its first segment.
func GroupOf(op string) string {
	group, _, _ := strings.Cut(op, ".")
	return group
}

// add holds o; a name the registry already holds panics. It is unexported so
// that Register, which checks the declaration, is the only way in.
func (r *Registry[Call, Caller]) add(o Op[Call]) {
	n := o.Meta().Name
	if _, dup := r.byName[n]; dup {
		panic(fmt.Sprintf("op %q: registered twice", n))
	}
	if _, ok := r.groups[GroupOf(n)]; !ok {
		panic(fmt.Sprintf("op %q: its group %q is not declared (Registry.Group)", n, GroupOf(n)))
	}
	r.byName[n] = o
	r.ops = append(r.ops, o)
}

// Ops returns the registered ops, in registration order.
func (r *Registry[Call, Caller]) Ops() []Op[Call] { return r.ops }

// Lookup returns the op named n.
func (r *Registry[Call, Caller]) Lookup(n string) (Op[Call], bool) {
	o, ok := r.byName[n]
	return o, ok
}

// GroupSummary is the group's summary, as declared (Group); "" for none.
func (r *Registry[Call, Caller]) GroupSummary(group string) string { return r.groups[group] }

// Runtime is what Invoke needs beyond the op: one per server.
type Runtime struct {
	// Schemas holds the JSON Schemas generated from op types. Transports share
	// it: HTTP builds its OpenAPI from it, MCP its tool schemas, so the validated
	// schema and the published one are the same.
	Schemas huma.Registry
	// Strict fails an op that returns a public code it did not declare, or an
	// output that does not match its schema. On in tests, so the OpenAPI built
	// from the declarations cannot quietly lie.
	Strict bool
	// Implied are the public codes the host's admission and transports produce,
	// which no op declares. With apikit's own (internal_server_error,
	// invalid_request), strict mode accepts them from any op.
	Implied []apperr.Code
	// Timeout makes the error of an op that ran out of its time the host's —
	// its "unavailable", say. Nil leaves the error as it is. Its code is implied
	// for every op, as Implied's are: an op cannot know it, least of all a
	// mounted one, whose host's timeout may be the one that ran out.
	Timeout func(err error) error
}

// timeoutCode is the public code Timeout answers, or the zero code without one.
func (rt *Runtime) timeoutCode() apperr.Code {
	if rt.Timeout == nil {
		return apperr.Code{}
	}
	return apperr.From(rt.Timeout(context.DeadlineExceeded)).Code.Public()
}

// NewRuntime returns a runtime with a fresh schema registry.
func NewRuntime() *Runtime {
	return &Runtime{Schemas: huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)}
}

// Schema is t's JSON Schema, as Invoke validates against it: a reference into
// Schemas for a named type, inline for the empty struct. Transports publish it,
// so what is validated and what is published are the same.
func (rt *Runtime) Schema(t reflect.Type) *huma.Schema { return decode.SchemaOf(rt.Schemas, t) }

// steps holds each op's private step for the process, by qualified name.
//
// appstep refuses to define a step name twice, which is right: a public step's
// name reaches clients. Ops, though, are built per instance — each handler
// closes over its service — and tests build them many times in one binary, each
// over its own fakes. So each op's step is defined once and reused. In
// production, ops are built once and every lookup misses; the cache is not on
// the request path.
var (
	steps   = map[string]appstep.Def{}
	stepsMu sync.Mutex
)

func stepFor(ns apperr.Namespace, name string) appstep.Def {
	stepsMu.Lock()
	defer stepsMu.Unlock()
	key := ns.String() + ".op." + name
	if d, ok := steps[key]; ok {
		return d
	}
	d := appstep.Define(ns, "op."+name)
	steps[key] = d
	return d
}
