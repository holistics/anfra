package apikit

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"

	"github.com/holistics/anfra/shared/apikit/internal/decode"
)

// Registry is the set of ops one server serves, all admitted from the same
// per-request R. The composition root makes one and passes it to each module,
// which registers its ops with Register.
type Registry[R any] struct {
	ns     apperr.Namespace
	params func(R) []any
	ops    []Op[R]
	byName map[string]Op[R]
}

// RegistryConfig is what a registry needs of its host.
type RegistryConfig[R any] struct {
	// Namespace is the host's: each op's private step is op.<name> in it.
	Namespace apperr.Namespace
	// StepParams are an op step's parameters for a request — who called, in
	// which org — for its log entry and span. Nil: none.
	StepParams func(r R) []any
}

// NewRegistry returns an empty registry.
func NewRegistry[R any](c RegistryConfig[R]) *Registry[R] {
	if c.Namespace.String() == "" {
		panic("apikit: a registry needs its host's namespace")
	}
	return &Registry[R]{ns: c.Namespace, params: c.StepParams, byName: map[string]Op[R]{}}
}

// add holds o; a name the registry already holds panics. It is unexported so
// that Register, which checks the declaration, is the only way in.
func (r *Registry[R]) add(o Op[R]) {
	n := o.Meta().Name
	if _, dup := r.byName[n]; dup {
		panic(fmt.Sprintf("op %q: registered twice", n))
	}
	r.byName[n] = o
	r.ops = append(r.ops, o)
}

// Ops returns the registered ops, in registration order.
func (r *Registry[R]) Ops() []Op[R] { return r.ops }

// Lookup returns the op named n.
func (r *Registry[R]) Lookup(n string) (Op[R], bool) {
	o, ok := r.byName[n]
	return o, ok
}

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
	// its "unavailable", say. Nil leaves the error as it is.
	Timeout func(err error) error
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
