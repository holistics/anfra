// Package apikit is the API framework the anfra hosts share: an operation is
// declared once, as a Def, and served by every host and transport the same way —
// its input validated against the schema of its type, its errors rendered from
// their codes, its OpenAPI derived from both.
//
// A host adds what is its own through hooks, never by forking the framework:
// who may call an op and how a request becomes the caller a handler receives
// (Admission), which codes it serves and with which HTTP status (httpkit.Codes),
// and what its transport reads and writes beyond the op (HTTP's hooks). `anfra
// serve` admits everyone; a hosted server resolves sessions, orgs and
// permissions. The handler and its input and output types do not change.
//
// The design is anfra-cloud's api_framework.md.
package apikit

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"

	"github.com/holistics/anfra/shared/apikit/internal/decode"
)

// DefaultTimeout bounds an op that states no Timeout.
const DefaultTimeout = 30 * time.Second

// Def declares one operation: what it takes, what it answers, and how it runs.
// C is the call context its handler receives — the caller, as the host's
// Admission resolves it.
type Def[C, In, Out any] struct {
	// Name is the op's name: dotted, group then verb ("invitations.create"). It
	// routes the op (POST /api/<Name>), names its MCP tool, and groups it for
	// discovery.
	Name    string
	Summary string // one line
	Doc     string // full usage, written for an agent: when to use it, the common mistake

	// Errors are the public codes the op itself returns, beyond those the host's
	// admission and transports produce. Published in OpenAPI; checked in strict
	// mode.
	Errors []apperr.AnyCode

	// ReadOnly: the op changes nothing, so a client may retry it freely.
	// Idempotent: repeating it with the same input has no further effect.
	// POST-only routing carries no method semantics, so the op states them; they
	// feed MCP's tool hints and client retry policy.
	ReadOnly   bool
	Idempotent bool

	// Timeout bounds the op; zero means DefaultTimeout.
	Timeout time.Duration

	HTTP bool // served at POST /api/<Name>
	MCP  bool // served as an MCP tool named Name

	// Core is set for an op whose types are known only at run time; see Core.
	Core *Core

	// Ext is the host's own declaration of the op, carried to Meta for the host's
	// transports: who may call it, its rate limit. apikit never reads it.
	Ext any

	// Handle is glue: it maps In to a service call and the result to Out, with no
	// branching on domain state.
	Handle func(ctx context.Context, c C, in In) (Out, error)
}

// Admission is how a host turns a request into the call context a handler
// receives. R is what the host's transports pass for each request — its
// credential; C is what its handlers take.
type Admission[R, C any] struct {
	// Admit resolves the request into C, or refuses it. It runs before the input
	// is read, so a caller who may not call the op cannot probe its validation.
	Admit func(ctx context.Context, r R) (C, error)
	// Authorize checks what the admitted caller may do with this input: a
	// permission derived from it, say. in is the op's In. Nil: admission is all.
	Authorize func(ctx context.Context, r R, c C, in any) error
}

// Args is the input of an op with a Core: its args by name, as JSON decodes them.
type Args = map[string]any

// Core describes an op whose input and output types are known only at run time:
// a command another registry describes, served through this one. Its Def is a
// Def[C, Args, any]; its input is validated against In, as any op's against its
// type's schema, and its answer published and checked as Out's.
type Core struct {
	// InName is In's name in the contract: "CoreQueryIn".
	InName string
	// In is the input's schema: an object, one property per arg. Exactly-one
	// groups are ExactlyOne entries in its OneOf, or one per AllOf entry.
	In *huma.Schema
	// Out is the answer's type; its schema is reflected, as any op's output's.
	Out reflect.Type
}

// ExactlyOne is the schema of a group of args of which exactly one must be set,
// for a Core's In to carry in OneOf: a oneOf over each arg being required. A
// caller missing the group, or setting more than one, is told which args to fix.
func ExactlyOne(args []string) []*huma.Schema { return decode.ExactlyOne(args) }

// Meta is an op's declaration without its types.
type Meta struct {
	Name, Summary, Doc string
	Errors             []apperr.Code
	ReadOnly           bool
	Idempotent         bool // implied by ReadOnly
	Timeout            time.Duration
	HTTP               bool
	MCP                bool
	Ext                any
}

// Op is a registered op, its types hidden: what a Registry holds and transports
// iterate. Only Register makes one.
type Op[R any] interface {
	Meta() Meta
	// InSchema is the input's schema, as Invoke validates it: transports publish
	// it, so what is validated and what is published are the same.
	InSchema(rt *Runtime) *huma.Schema
	OutType() reflect.Type
	// Invoke runs the op on raw JSON input, for the request r.
	Invoke(ctx context.Context, rt *Runtime, r R, raw []byte) (any, error)
}

// name is a dotted op name: a group, then a verb, each lowercase snake_case.
var name = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Register checks d and adds it to reg, admitted by adm, as huma.Register and
// mcp.AddTool do for theirs. A malformed declaration, or a name reg already
// holds, panics: a wiring mistake, and failing at startup is the only safe
// answer.
//
// The op is a copy of d, taken now, so a later change to d does not change the
// running op. Ext is copied as a value; a host keeps it free of anything it
// would mutate.
func Register[R, C, In, Out any](reg *Registry[R], adm Admission[R, C], d *Def[C, In, Out]) {
	switch {
	case !name.MatchString(d.Name):
		panic(fmt.Sprintf("op %q: a name is dotted lowercase snake_case, group then verb", d.Name))
	case d.Summary == "" || d.Handle == nil:
		panic(fmt.Sprintf("op %q: needs a summary and a handler", d.Name))
	case adm.Admit == nil:
		panic(fmt.Sprintf("op %q: needs an admission", d.Name))
	case d.Timeout < 0:
		panic(fmt.Sprintf("op %q: negative timeout", d.Name))
	}
	checkCore(d)
	if d.Core == nil {
		for _, t := range []reflect.Type{reflect.TypeFor[In](), reflect.TypeFor[Out]()} {
			if t.Kind() == reflect.Struct && t.Name() == "" && t.NumField() > 0 {
				// An unnamed struct has no name to publish its schema under, so it
				// could not be referenced from the API contract.
				panic(fmt.Sprintf("op %q: name the input and output types; an anonymous struct has no schema name", d.Name))
			}
		}
	}
	frozen := *d
	frozen.Errors = slices.Clone(d.Errors)
	if d.Core != nil {
		c := *d.Core
		frozen.Core = &c
	}
	reg.add(&registered[R, C, In, Out]{decl: &frozen, adm: adm, step: stepFor(reg.ns, d.Name), params: reg.params})
}

// checkCore refuses a Core that does not describe the op's types. Its schema is
// prepared here, once, for validation.
func checkCore[C, In, Out any](d *Def[C, In, Out]) {
	c := d.Core
	if c == nil {
		return
	}
	switch {
	case reflect.TypeFor[In]() != reflect.TypeFor[Args]() || reflect.TypeFor[Out]() != reflect.TypeFor[any]():
		panic(fmt.Sprintf("op %q: an op with a Core is a Def[C, apikit.Args, any]; its Core holds the schemas", d.Name))
	case c.InName == "" || c.In == nil || c.In.Type != huma.TypeObject || c.Out == nil:
		panic(fmt.Sprintf("op %q: a Core needs an InName, an object In and an Out", d.Name))
	}
	prepare(c.In)
}

// prepare readies a schema built by hand for validation, as huma does for the
// schemas it reflects. huma's own PrecomputeMessages skips AllOf.
func prepare(s *huma.Schema) {
	if s == nil {
		return
	}
	s.PrecomputeMessages()
	for _, sub := range s.AllOf {
		prepare(sub)
	}
	for _, sub := range s.OneOf {
		prepare(sub) // PrecomputeMessages reaches these, but not their AllOf
	}
}

// registered is a registered op: the frozen copy of its declaration, its
// admission, and its private step.
type registered[R, C, In, Out any] struct {
	decl   *Def[C, In, Out]
	adm    Admission[R, C]
	step   appstep.Def
	params func(R) []any
}

func (o *registered[R, C, In, Out]) Meta() Meta {
	codes := make([]apperr.Code, len(o.decl.Errors))
	for i, c := range o.decl.Errors {
		codes[i] = c.Code()
	}
	timeout := o.decl.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return Meta{
		Name: o.decl.Name, Summary: o.decl.Summary, Doc: o.decl.Doc, Errors: codes,
		ReadOnly: o.decl.ReadOnly, Idempotent: o.decl.Idempotent || o.decl.ReadOnly,
		Timeout: timeout, HTTP: o.decl.HTTP, MCP: o.decl.MCP, Ext: o.decl.Ext,
	}
}

// InSchema is a reference to In's schema in rt.Schemas: reflected from In, or
// for an op with a Core its Core's, added under its name on first use.
func (o *registered[R, C, In, Out]) InSchema(rt *Runtime) *huma.Schema {
	c := o.decl.Core
	if c == nil {
		return rt.Schema(reflect.TypeFor[In]())
	}
	schemas := rt.Schemas.Map()
	if existing, ok := schemas[c.InName]; !ok {
		schemas[c.InName] = c.In
	} else if existing != c.In && !reflect.DeepEqual(existing, c.In) {
		panic(fmt.Sprintf("op %q: schema %s is already another", o.decl.Name, c.InName))
	}
	return &huma.Schema{Ref: "#/components/schemas/" + c.InName}
}

// OutType is Out, or for an op with a Core its Core's.
func (o *registered[R, C, In, Out]) OutType() reflect.Type {
	if o.decl.Core != nil {
		return o.decl.Core.Out
	}
	return reflect.TypeFor[Out]()
}
