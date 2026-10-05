package apperr

import "github.com/holistics/anfra/shared/appstep"

// WithStep records that err passed through a step, leaving its meaning as it
// was. apptracing calls it when a step ends; application code starts a step
// instead.
//
// A formal error — or a bare code — comes back as a copy with the step added
// in front of its steps; the original is not mutated. Anything else is
// formalised as an unclassified internal_server_error carrying the step: it
// would render as that anyway, and the flag keeps "nobody classified this" apart
// from "somebody decided internal_server_error".
func WithStep(err error, step appstep.Step) error {
	var e *Error
	switch x := err.(type) {
	case nil:
		return nil
	case *Error:
		c := *x
		e = &c
	case AnyCode:
		c := x.Code()
		e = &Error{Code: c, Message: c.c.message}
	default:
		e = unclassified(err)
	}
	e.Steps = append([]appstep.Step{step}, e.Steps...)
	return e
}
