package apikit

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"time"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/apptracing"

	"github.com/holistics/anfra/shared/apikit/internal/decode"
)

// Invoke runs the op. Every call is a private step, op.<name>, so every op gets
// a span and an entry on its error's log line with no code in the module.
//
// The order is the order a client would fix things in: be admitted (signed in,
// in an org), send valid input, hold the permissions. Admission comes before
// the input is read, so a caller who may not call the op cannot probe its
// validation; authorization comes after it is prepared (Def.Prepare), since it
// may be derived from it.
func (o *registered[Call, Caller, In, Out]) Invoke(ctx context.Context, rt *Runtime, r Call, raw []byte) (any, error) {
	return o.invoke(ctx, rt, r, raw, nil)
}

// InvokeStream is Invoke for an op that returns a Run, emitting its events
// rather than answering its page.
func (o *registered[Call, Caller, In, Out]) InvokeStream(ctx context.Context, rt *Runtime, r Call, raw []byte, emit func(any) error) error {
	if o.events == nil {
		return fmt.Errorf("op %q does not stream: it returns no Run", o.decl.Name)
	}
	_, err := o.invoke(ctx, rt, r, raw, emit)
	return err
}

// invoke runs the op; with emit, streaming its Run.
func (o *registered[Call, Caller, In, Out]) invoke(ctx context.Context, rt *Runtime, r Call, raw []byte, emit func(any) error) (_ any, err error) {
	var params []any
	if o.params != nil {
		params = o.params(r)
	}
	ctx, span := apptracing.Start(ctx, o.step, params...)
	defer span.End(&err)

	c, err := o.adm.Admit(ctx, r)
	if err != nil {
		return nil, err
	}
	in, err := o.prepare(rt, raw)
	if err != nil {
		return nil, err
	}
	return o.run(ctx, rt, r, c, in, emit)
}

// invokePrepared runs the op on an input prepare answered, for a host that
// mounts it (Mount): the op's own admission, authorization and handler, with
// nothing decoded again. With emit, streaming its Run.
func (o *registered[Call, Caller, In, Out]) invokePrepared(ctx context.Context, rt *Runtime, r Call, in any, emit func(any) error) (_ any, err error) {
	if emit != nil && o.events == nil {
		return nil, fmt.Errorf("op %q does not stream: it returns no Run", o.decl.Name)
	}
	var params []any
	if o.params != nil {
		params = o.params(r)
	}
	ctx, span := apptracing.Start(ctx, o.step, params...)
	defer span.End(&err)

	c, err := o.adm.Admit(ctx, r)
	if err != nil {
		return nil, err
	}
	return o.run(ctx, rt, r, c, in.(In), emit)
}

func (o *registered[Call, Caller, In, Out]) inType() reflect.Type { return reflect.TypeFor[In]() }

func (o *registered[Call, Caller, In, Out]) prepareAny(rt *Runtime, raw []byte) (any, error) {
	return o.prepare(rt, raw)
}

// prepare is raw decoded and validated against In's schema, then prepared
// (Def.Prepare): the input authorization and the handler see.
func (o *registered[Call, Caller, In, Out]) prepare(rt *Runtime, raw []byte) (In, error) {
	in, err := decode.InputTo[In](rt.Schemas, o.InSchema(rt), raw)
	if err != nil || o.decl.Prepare == nil {
		return in, err
	}
	return o.decl.Prepare(in)
}

// run authorizes the admitted caller c on the prepared input, then answers it.
func (o *registered[Call, Caller, In, Out]) run(ctx context.Context, rt *Runtime, r Call, c Caller, in In, emit func(any) error) (any, error) {
	if o.adm.Authorize != nil {
		if err := o.adm.Authorize(ctx, r, c, in); err != nil {
			return nil, err
		}
	}
	if emit != nil && rt.Strict {
		emit = o.checkedEmit(rt, emit)
	}
	out, err := o.answer(ctx, rt, c, in, emit)
	if err != nil {
		if rt.Strict {
			o.checkDeclared(rt, err)
		}
		return nil, err
	}
	if rt.Strict && emit == nil {
		if err := decode.CheckOutput(rt.Schemas, o.OutType(), out); err != nil {
			panic(fmt.Sprintf("op %q: %v", o.decl.Name, err))
		}
	}
	return out, nil
}

// answer is the op's answer: its handler's output; or, when the handler returns
// a Run, the run submitted to its executor and executed in the form asked — a
// stream, with emit, which starts once the executor has taken the run; or a
// page. A mounted op's run is its host's: submitted to the mounting registry's
// executor, for its caller.
//
// The op's timeout bounds its own work: the handler, and the run from when it
// starts — when the executor calls its Page or Stream — not what the executor
// does before (waiting for capacity, say). So no executor has a timeout to
// apply, and each run's deadline is its own, wherever it runs.
func (o *registered[Call, Caller, In, Out]) answer(ctx context.Context, rt *Runtime, c Caller, in In, emit func(any) error) (any, error) {
	timeout := o.decl.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	if host, ok := hostTimeoutFrom(ctx); ok && host < timeout {
		timeout = host
	}
	out, err := withTimeout(ctx, rt, timeout, func(ctx context.Context) (Out, error) { return o.decl.Handle(ctx, c, in) })
	if err != nil {
		return nil, err
	}
	r, ok := any(out).(run)
	if !ok {
		return out, nil
	}
	submit, mounted := submitterFrom(ctx)
	if !mounted {
		submit = submitTo(o.executor, c, o.Meta())
	}
	exec, err := submit(ctx, r.work().within(rt, timeout))
	if err != nil {
		return nil, err // refused: an ordinary error, nothing sent yet
	}
	if emit == nil {
		return exec.Page(ctx)
	}
	// The stream starts: what fails from here on ends it rather than answering
	// an error.
	if err := emit(StreamStart{Type: "start", ID: exec.ID}); err != nil {
		// Its client is gone before the run began. The execution is executed
		// still — every execution is, once — but cancelled, so it ends at once,
		// and its executor sees the end of what it took.
		gone, cancel := context.WithCancelCause(ctx)
		cancel(err)
		_ = exec.Stream(gone, emit)
		return nil, err
	}
	return nil, exec.Stream(ctx, emit)
}

// withTimeout is fn within timeout. Running out of it is the host's to name
// (Runtime.Timeout) — unless parent ended first, which is the caller going
// away.
func withTimeout[T any](parent context.Context, rt *Runtime, timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	out, err := fn(ctx)
	if err != nil && rt.Timeout != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
		return out, rt.Timeout(err)
	}
	return out, err
}

// checkedEmit is emit checking each of the op's own events against its event
// schema, as strict mode checks an output.
func (o *registered[Call, Caller, In, Out]) checkedEmit(rt *Runtime, emit func(any) error) func(any) error {
	return func(e any) error {
		t := reflect.TypeOf(e)
		if !slices.Contains(o.events, t) {
			if slices.Contains(slices.Collect(maps.Values(reservedEvents)), t) {
				return emit(e)
			}
			panic(fmt.Sprintf("op %q: emitted a %v, which is none of its events", o.decl.Name, t))
		}
		if err := decode.CheckOutput(rt.Schemas, t, e); err != nil {
			panic(fmt.Sprintf("op %q: event: %v", o.decl.Name, err))
		}
		return emit(e)
	}
}

// implied are the public codes apikit itself produces, which no op declares.
var implied = []apperr.Code{apperr.InternalServerError, apperr.InvalidRequest.Code()}

func (o *registered[Call, Caller, In, Out]) checkDeclared(rt *Runtime, err error) {
	pub := apperr.From(err).Code.Public()
	if slices.Contains(implied, pub) || slices.Contains(rt.Implied, pub) || pub == rt.timeoutCode() {
		return
	}
	for _, c := range o.decl.Errors {
		if c.Code().Public() == pub {
			return
		}
	}
	panic(fmt.Sprintf("op %q returned %s, which it does not declare in Errors", o.decl.Name, pub))
}
