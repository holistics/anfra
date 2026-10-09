package apikit

import (
	"context"
	"errors"
	"fmt"
	"reflect"

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
// schemas, handler and codes unchanged — under its own admission. R is what the
// host's transports pass; C is the host's caller; From is what the mounted op is
// admitted from (the engine's invocation, say).
//
// A call runs Admit, then Authorize, then Bind, then the mounted op, which
// validates its input and runs. So authorization sees the input as the client
// sent it, not yet validated: a caller who may not call the op is refused before
// anything about the input is said.
type Mounting[R, C, From any] struct {
	// Admit resolves the request into the host's caller, or refuses it. Required.
	Admit func(ctx context.Context, r R) (C, error)
	// Authorize checks what the caller may do with this input. Nil: admission
	// is all.
	Authorize func(ctx context.Context, r R, c C, input Args) error
	// Bind builds what the mounted op is admitted from, for this caller.
	// Required.
	Bind func(ctx context.Context, c C) (From, error)
	// Overlay amends the op's declaration for this host — its doc, its timeout,
	// its Ext — never its name, schemas or codes. Nil: as it is.
	Overlay func(m Meta) Meta
}

// Mount serves from's op named name in into, with m. It panics, as Register does,
// on an op from does not hold, a name into already holds, or a malformed m.
func Mount[R, C, From any](into *Registry[R], from *Registry[From], name string, m Mounting[R, C, From]) {
	inner, ok := from.Lookup(name)
	switch {
	case !ok:
		panic(fmt.Sprintf("op %q: not in the registry to mount from", name))
	case m.Admit == nil || m.Bind == nil:
		panic(fmt.Sprintf("op %q: a mounting needs Admit and Bind", name))
	}
	meta := inner.Meta()
	if m.Overlay != nil {
		meta = m.Overlay(meta)
		if meta.Name != name || !reflect.DeepEqual(meta.Errors, inner.Meta().Errors) {
			panic(fmt.Sprintf("op %q: an overlay may not rename an op or change its codes", name))
		}
	}
	if group := GroupOf(name); into.groups[group] == "" {
		// The mounted op's group, as its registry describes it, unless the host
		// describes it itself.
		into.groups[group] = from.groups[group]
	}
	into.add(&mounted[R, C, From]{inner: inner, m: m, meta: meta, step: stepFor(into.ns, name), params: into.params})
}

type mounted[R, C, From any] struct {
	inner  Op[From]
	m      Mounting[R, C, From]
	meta   Meta
	step   appstep.Def
	params func(R) []any
}

func (o *mounted[R, C, From]) Meta() Meta                        { return o.meta }
func (o *mounted[R, C, From]) InSchema(rt *Runtime) *huma.Schema { return o.inner.InSchema(rt) }
func (o *mounted[R, C, From]) OutType() reflect.Type             { return o.inner.OutType() }

// Invoke runs the host's steps, then the mounted op, within the overlay's
// timeout when it set one (the op's own still applies inside it). Every call is
// the host's private step for the op, around the mounted op's own.
func (o *mounted[R, C, From]) Invoke(ctx context.Context, rt *Runtime, r R, raw []byte) (_ any, err error) {
	var params []any
	if o.params != nil {
		params = o.params(r)
	}
	ctx, span := apptracing.Start(ctx, o.step, params...)
	defer span.End(&err)

	c, err := o.m.Admit(ctx, r)
	if err != nil {
		return nil, err
	}
	if o.m.Authorize != nil {
		input, err := argsOf(raw)
		if err != nil {
			return nil, err
		}
		if err := o.m.Authorize(ctx, r, c, input); err != nil {
			return nil, err
		}
	}
	from, err := o.m.Bind(ctx, c)
	if err != nil {
		return nil, err
	}
	if o.meta.Timeout == 0 || o.meta.Timeout == o.inner.Meta().Timeout {
		return o.inner.Invoke(ctx, rt, from, raw)
	}
	inner, cancel := context.WithTimeout(ctx, o.meta.Timeout)
	defer cancel()
	out, err := o.inner.Invoke(inner, rt, from, raw)
	if err != nil && rt.Timeout != nil && errors.Is(inner.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
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
