package apikit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"
	"github.com/holistics/anfra/shared/apptracing"
	"github.com/holistics/anfra/shared/jsonkit"
)

// Args is an op's input as the client sent it: a JSON object, by field name,
// before it is validated. What a mounting host authorizes from.
type Args = map[string]any

// Mounting is how a host serves another registry's op as it is — its name,
// schemas, handler and codes unchanged — under its own admission. Call is what
// the host's transports pass; Caller is the host's caller; BoundCall is what
// the mounted op is called with: its own registry's Call (the engine's
// Invocation, say).
//
// A call runs Admit, then Authorize, then Bind, then the mounted op, which
// validates its input and runs. A run it returns is the host's: submitted to
// the host's registry's executor, for the host's caller.
type Mounting[Call, Caller, BoundCall any] struct {
	// Admit resolves the call into the host's caller, or refuses it. Required.
	Admit func(ctx context.Context, call Call) (Caller, error)
	// Authorize checks what the caller may do with this input, as the client
	// sent it. Nil: admission is all.
	Authorize func(ctx context.Context, call Call, caller Caller, input Args) error
	// Bind builds what the mounted op is called with, for this caller.
	// Required.
	Bind func(ctx context.Context, caller Caller) (BoundCall, error)
}

// MountOption amends a mounted op's declaration for the host serving it: never
// its name, schemas or codes, which are the op's.
type MountOption func(*Meta)

// AppendDoc adds what is particular to calling the op on this host after the
// op's own doc.
func AppendDoc(doc string) MountOption {
	return func(m *Meta) {
		if doc != "" {
			m.Doc = strings.TrimSpace(m.Doc + "\n\n" + doc)
		}
	}
}

// WithTimeout bounds the op on this host; the op's own still applies inside
// it. Zero: the op's own.
func WithTimeout(d time.Duration) MountOption {
	return func(m *Meta) {
		if d > 0 {
			m.Timeout = d
		}
	}
}

// WithExt is the host's own declaration of the op (Def.Ext), for its
// transports and executor.
func WithExt(ext any) MountOption { return func(m *Meta) { m.Ext = ext } }

// Mount serves from's op named name in into, with m and opts. It panics, as
// Register does, on an op from does not hold, a name into already holds, or a
// malformed m.
func Mount[Call, Caller, BoundCall any](into *Registry[Call, Caller], from OpSet[BoundCall], name string, m Mounting[Call, Caller, BoundCall], opts ...MountOption) {
	inner, ok := from.Lookup(name)
	switch {
	case !ok:
		panic(fmt.Sprintf("op %q: not in the registry to mount from", name))
	case m.Admit == nil || m.Bind == nil:
		panic(fmt.Sprintf("op %q: a mounting needs Admit and Bind", name))
	}
	meta := inner.Meta()
	for _, opt := range opts {
		opt(&meta)
	}
	if group := GroupOf(name); into.groups[group] == "" {
		// The mounted op's group, as its registry describes it, unless the host
		// describes it itself.
		into.groups[group] = from.GroupSummary(group)
	}
	into.add(&mounted[Call, Caller, BoundCall]{inner: inner, m: m, meta: meta, step: stepFor(into.ns, name), params: into.params, executor: into.executor})
}

type mounted[Call, Caller, BoundCall any] struct {
	inner    Op[BoundCall]
	m        Mounting[Call, Caller, BoundCall]
	meta     Meta
	step     appstep.Def
	params   func(Call) []any
	executor Executor[Caller]
}

func (o *mounted[Call, Caller, BoundCall]) Meta() Meta { return o.meta }
func (o *mounted[Call, Caller, BoundCall]) InSchema(rt *Runtime) *huma.Schema {
	return o.inner.InSchema(rt)
}
func (o *mounted[Call, Caller, BoundCall]) OutType() reflect.Type      { return o.inner.OutType() }
func (o *mounted[Call, Caller, BoundCall]) EventTypes() []reflect.Type { return o.inner.EventTypes() }

// Invoke runs the host's steps, then the mounted op, within the host's timeout
// when it set one (the op's own still applies inside it). Every call is the
// host's private step for the op, around the mounted op's own.
func (o *mounted[Call, Caller, BoundCall]) Invoke(ctx context.Context, rt *Runtime, call Call, raw []byte) (any, error) {
	return o.invoke(ctx, rt, call, raw, func(ctx context.Context, bound BoundCall) (any, error) {
		return o.inner.Invoke(ctx, rt, bound, raw)
	})
}

// InvokeStream is Invoke, streaming the mounted op's Run.
func (o *mounted[Call, Caller, BoundCall]) InvokeStream(ctx context.Context, rt *Runtime, call Call, raw []byte, emit func(any) error) error {
	_, err := o.invoke(ctx, rt, call, raw, func(ctx context.Context, bound BoundCall) (any, error) {
		return nil, o.inner.InvokeStream(ctx, rt, bound, raw, emit)
	})
	return err
}

func (o *mounted[Call, Caller, BoundCall]) invoke(ctx context.Context, rt *Runtime, call Call, raw []byte, inner func(context.Context, BoundCall) (any, error)) (_ any, err error) {
	var params []any
	if o.params != nil {
		params = o.params(call)
	}
	ctx, span := apptracing.Start(ctx, o.step, params...)
	defer span.End(&err)

	caller, err := o.m.Admit(ctx, call)
	if err != nil {
		return nil, err
	}
	if o.m.Authorize != nil {
		input, err := argsOf(raw)
		if err != nil {
			return nil, err
		}
		if err := o.m.Authorize(ctx, call, caller, input); err != nil {
			return nil, err
		}
	}
	bound, err := o.m.Bind(ctx, caller)
	if err != nil {
		return nil, err
	}
	// A run the mounted op returns is the host's, for its caller.
	ctx = withSubmitter(ctx, submitTo(o.executor, caller, o.meta))
	if o.meta.Timeout == 0 || o.meta.Timeout == o.inner.Meta().Timeout {
		return inner(ctx, bound)
	}
	bounded, cancel := context.WithTimeout(ctx, o.meta.Timeout)
	defer cancel()
	out, err := inner(bounded, bound)
	if err != nil && rt.Timeout != nil && errors.Is(bounded.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		return out, rt.Timeout(err)
	}
	return out, err
}

// argsOf is a body as the object the client sent, for authorization: an empty
// body is an empty object, as decoding has it; anything but an object is no
// args, and the op's validation says what is wrong with it.
func argsOf(raw []byte) (Args, error) {
	if len(raw) == 0 {
		return Args{}, nil
	}
	var parsed any
	if err := jsonkit.Unmarshal(raw, &parsed); err != nil {
		return nil, apperr.Encapsulate(err, apperr.InvalidRequest, "The body is not valid JSON.")
	}
	args, _ := parsed.(map[string]any)
	if args == nil {
		args = Args{}
	}
	return args, nil
}
