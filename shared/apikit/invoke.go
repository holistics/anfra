package apikit

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/apptracing"

	"github.com/holistics/anfra/shared/apikit/internal/decode"
)

// Invoke runs the op. Every call is a private step, op.<name>, so every op gets
// a span and an entry on its error's log line with no code in the module.
//
// The order is the order a client would fix things in: be admitted (signed in,
// in an org), send valid input, hold the permissions. Admission comes before
// decoding, so a caller who may not call the op cannot probe its validation;
// only authorization needs the input, since it may be derived from it.
func (o *registered[R, C, In, Out]) Invoke(ctx context.Context, rt *Runtime, r R, raw []byte) (_ any, err error) {
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
	in, err := decode.InputTo[In](rt.Schemas, o.InSchema(rt), raw)
	if err != nil {
		return nil, err
	}
	if o.adm.Authorize != nil {
		if err := o.adm.Authorize(ctx, r, c, in); err != nil {
			return nil, err
		}
	}
	out, err := o.handle(ctx, rt, c, in)
	if err != nil {
		if rt.Strict {
			o.checkDeclared(rt, err)
		}
		return nil, err
	}
	if rt.Strict {
		if err := decode.CheckOutput(rt.Schemas, o.OutType(), out); err != nil {
			panic(fmt.Sprintf("op %q: %v", o.decl.Name, err))
		}
	}
	return out, nil
}

// handle runs the handler within the op's timeout. Running out of it is the
// host's to name (Runtime.Timeout) — unless the caller's own context ended
// first, which is the caller going away.
func (o *registered[R, C, In, Out]) handle(parent context.Context, rt *Runtime, c C, in In) (Out, error) {
	timeout := o.decl.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	out, err := o.decl.Handle(ctx, c, in)
	if err != nil && rt.Timeout != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
		return out, rt.Timeout(err)
	}
	return out, err
}

// implied are the public codes apikit itself produces, which no op declares.
var implied = []apperr.Code{apperr.InternalServerError, apperr.InvalidRequest.Code()}

func (o *registered[R, C, In, Out]) checkDeclared(rt *Runtime, err error) {
	pub := apperr.From(err).Code.Public()
	if slices.Contains(implied, pub) || slices.Contains(rt.Implied, pub) {
		return
	}
	for _, c := range o.decl.Errors {
		if c.Code().Public() == pub {
			return
		}
	}
	panic(fmt.Sprintf("op %q returned %s, which it does not declare in Errors", o.decl.Name, pub))
}
