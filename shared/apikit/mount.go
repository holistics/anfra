package apikit

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/appstep"
	"github.com/holistics/anfra/shared/apptracing"
)

// OpRef names an op with its input type: what a host mounts an op by, so that
// its authorization reads the op's input, typed, and a derivation for another
// type does not compile. A registry whose ops hosts mount exports one per op
// they authorize on; Mount checks, at startup, that the op named takes In.
type OpRef[In any] struct{ Name string }

// Untyped refers to the op named name with its input untyped: for a host whose
// authorization does not read the input.
func Untyped(name string) OpRef[any] { return OpRef[any]{Name: name} }

// CheckRef reports whether ref names an op of set that takes ref's input: for
// a registry to test the refs it exports.
func CheckRef[Call, In any](set OpSet[Call], ref OpRef[In]) error {
	_, err := preparableOf(set, ref)
	return err
}

// Mounting is how a host serves another registry's op as it is — its name,
// schemas, handler and codes unchanged — under its own admission. Call is what
// the host's transports pass; Caller is the host's caller; BoundCall is what
// the mounted op is called with: its own registry's Call (the engine's
// Invocation, say); In is the op's input, as its OpRef says.
//
// A call runs Admit; then the op's decoding and preparation (Def.Prepare);
// then Authorize, on the prepared input; then Bind, with the caller Authorize
// answered; then the mounted op, on the same input, decoded once. A run it
// returns is the host's: submitted to the host's registry's executor, for the
// host's caller.
type Mounting[Call, Caller, BoundCall, In any] struct {
	// Admit resolves the call into the host's caller, or refuses it. It runs
	// before the input is read. Required.
	Admit func(ctx context.Context, call Call) (Caller, error)
	// Authorize checks what the caller may do with the input, as the op
	// prepared it, and answers the caller Bind gets: narrowed to what it was
	// authorized for, say. It must not change in. Nil: admission is all, and
	// Bind gets the caller Admit answered.
	Authorize func(ctx context.Context, call Call, caller Caller, in In) (Caller, error)
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

// WithTimeout bounds the op on this host, as the op's own timeout does — its
// handler, and its run from when the run starts — when it is the tighter.
// Zero: the op's own.
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

// Mount serves from's op that ref names in into, with m and opts. It panics, as
// Register does, on an op from does not hold, one that does not take ref's
// input, a name into already holds, or a malformed m.
func Mount[Call, Caller, BoundCall, In any](into *Registry[Call, Caller], from OpSet[BoundCall], ref OpRef[In], m Mounting[Call, Caller, BoundCall, In], opts ...MountOption) {
	inner, err := preparableOf(from, ref)
	switch {
	case err != nil:
		panic(err.Error())
	case m.Admit == nil || m.Bind == nil:
		panic(fmt.Sprintf("op %q: a mounting needs Admit and Bind", ref.Name))
	}
	meta := inner.Meta()
	for _, opt := range opts {
		opt(&meta)
	}
	if group := GroupOf(ref.Name); into.groups[group] == "" {
		// The mounted op's group, as its registry describes it, unless the host
		// describes it itself.
		into.groups[group] = from.GroupSummary(group)
	}
	into.add(&mounted[Call, Caller, BoundCall, In]{inner: inner, m: m, meta: meta, step: stepFor(into.ns, ref.Name), params: into.params, executor: into.executor})
}

// preparable is an op whose input can be prepared apart from running it, which
// Mount needs to authorize on the prepared input: every op Register makes.
type preparable[Call any] interface {
	Op[Call]
	inType() reflect.Type
	prepareAny(rt *Runtime, raw []byte) (any, error)
	invokePrepared(ctx context.Context, rt *Runtime, call Call, in any, emit func(any) error) (any, error)
}

// preparableOf is the op of set that ref names, checked to take ref's input
// (any input, for an untyped ref).
func preparableOf[Call, In any](set OpSet[Call], ref OpRef[In]) (preparable[Call], error) {
	o, ok := set.Lookup(ref.Name)
	if !ok {
		return nil, fmt.Errorf("op %q: not in the registry to mount from", ref.Name)
	}
	p, ok := o.(preparable[Call])
	if !ok {
		return nil, fmt.Errorf("op %q: not a registered op (a mounted one cannot be mounted again)", ref.Name)
	}
	if want := reflect.TypeFor[In](); want.Kind() != reflect.Interface && p.inType() != want {
		return nil, fmt.Errorf("op %q: takes %v, not %v", ref.Name, p.inType(), want)
	}
	return p, nil
}

type mounted[Call, Caller, BoundCall, In any] struct {
	inner    preparable[BoundCall]
	m        Mounting[Call, Caller, BoundCall, In]
	meta     Meta
	step     appstep.Def
	params   func(Call) []any
	executor Executor[Caller]
}

func (o *mounted[Call, Caller, BoundCall, In]) Meta() Meta { return o.meta }
func (o *mounted[Call, Caller, BoundCall, In]) InSchema(rt *Runtime) *huma.Schema {
	return o.inner.InSchema(rt)
}
func (o *mounted[Call, Caller, BoundCall, In]) OutType() reflect.Type { return o.inner.OutType() }
func (o *mounted[Call, Caller, BoundCall, In]) EventTypes() []reflect.Type {
	return o.inner.EventTypes()
}

// Invoke runs the host's steps, then the mounted op, within the host's timeout
// when it set one (the op's own still applies inside it). Every call is the
// host's private step for the op, around the mounted op's own.
func (o *mounted[Call, Caller, BoundCall, In]) Invoke(ctx context.Context, rt *Runtime, call Call, raw []byte) (any, error) {
	return o.invoke(ctx, rt, call, raw, nil)
}

// InvokeStream is Invoke, streaming the mounted op's Run.
func (o *mounted[Call, Caller, BoundCall, In]) InvokeStream(ctx context.Context, rt *Runtime, call Call, raw []byte, emit func(any) error) error {
	_, err := o.invoke(ctx, rt, call, raw, emit)
	return err
}

func (o *mounted[Call, Caller, BoundCall, In]) invoke(ctx context.Context, rt *Runtime, call Call, raw []byte, emit func(any) error) (_ any, err error) {
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
	in, err := o.inner.prepareAny(rt, raw)
	if err != nil {
		return nil, err
	}
	if o.m.Authorize != nil {
		if caller, err = o.m.Authorize(ctx, call, caller, in.(In)); err != nil {
			return nil, err
		}
	}
	bound, err := o.m.Bind(ctx, caller)
	if err != nil {
		return nil, err
	}
	// A run the mounted op returns is the host's, for its caller; and the
	// host's timeout, where it set one, bounds the mounted op's own work as the
	// op's does: its handler, and its run from when it starts.
	ctx = withSubmitter(ctx, submitTo(o.executor, caller, o.meta))
	if o.meta.Timeout != 0 {
		ctx = withHostTimeout(ctx, o.meta.Timeout)
	}
	return o.inner.invokePrepared(ctx, rt, bound, in, emit)
}

type hostTimeoutKey struct{}

// withHostTimeout is ctx carrying a mount's timeout, for the mounted op to
// bound its work by when it is the tighter; within mounts within mounts, the
// tightest.
func withHostTimeout(ctx context.Context, d time.Duration) context.Context {
	if outer, ok := hostTimeoutFrom(ctx); ok && outer <= d {
		return ctx
	}
	return context.WithValue(ctx, hostTimeoutKey{}, d)
}

func hostTimeoutFrom(ctx context.Context) (time.Duration, bool) {
	d, ok := ctx.Value(hostTimeoutKey{}).(time.Duration)
	return d, ok
}
