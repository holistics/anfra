package apperr

import (
	"slices"
	"strings"

	"github.com/holistics/anfra/shared/appstep"
)

// Error is a formal error: a failure with a code, and the steps it passed
// through since it was made. Reach it with errors.As, which finds the
// outermost formal error in a chain.
//
// Its inner error, if any, stays reachable to errors.Is and errors.As and to
// the log. It is hidden from clients, along with the inner error's own steps,
// unless the error discloses them: Translate does, and so does Encapsulate once
// DisableErrorEncapsulation was called.
type Error struct {
	Code    Code
	Message string
	Details any
	// Steps are the steps the error passed through since it was made,
	// outermost first. Added by WithStep, on a copy.
	Steps []appstep.Step

	inner error
	// discloseInnerSteps makes inner's public steps part of what a response
	// shows, after this error's own. Only Translate sets it: inner is the same
	// failure, re-coded. Inner's code, message and details are not read through
	// it; Translate copies the message when it makes the error.
	discloseInnerSteps bool
}

// New makes a formal error that starts here. An empty msg uses the code's
// default message.
func New(c AnyCode, msg string) error {
	return newError(nil, c.Code(), msg, nil)
}

// NewWith is New with typed details.
func NewWith[D any](c TypedCode[D], msg string, details D) error {
	return newError(nil, c.Code(), msg, details)
}

// Encapsulate makes a formal error from an existing one, which becomes its
// inner error: kept for the log and for errors.Is, hidden from clients along
// with the steps beneath it. Chain only what actually caused the failure.
//
// An empty msg uses the code's default message.
func Encapsulate(inner error, c AnyCode, msg string) error {
	return newError(inner, c.Code(), msg, nil)
}

// EncapsulateWith is Encapsulate with typed details.
func EncapsulateWith[D any](inner error, c TypedCode[D], msg string, details D) error {
	return newError(inner, c.Code(), msg, details)
}

// Translate re-codes err, a failure under another module's code, as c: where a
// host calls a library, it is how the library's errors become the host's. It is
// the same failure, so err's message and context stay visible to clients — its
// code, details and chain do not, and the log keeps all of it. To show a
// different message, or hide the context, encapsulate instead.
//
// Only an internal_server_error's message is never carried over: it is a
// diagnosis, so c's default message is used — unless encapsulation is disabled
// (DisableEncapsulation).
func Translate(err error, c AnyCode) error {
	return translate(err, c.Code(), nil)
}

// TranslateWith is Translate with typed details: c's own, since err's are its
// module's type. Read err's with DetailsOf.
func TranslateWith[D any](err error, c TypedCode[D], details D) error {
	return translate(err, c.Code(), details)
}

func translate(err error, c Code, details any) error {
	if err == nil {
		return nil
	}
	from := From(err)
	msg := from.Message
	if from.Code.Public() == InternalServerError {
		if encapsulating() {
			msg = c.c.message
		} else {
			msg = from.clientMessage()
		}
	}
	return &Error{Code: c, Message: msg, Details: details, inner: err, discloseInnerSteps: true}
}

// disclosedSteps are the steps a response describes: e's own, outermost first,
// then inner's, when e discloses them.
func (e *Error) disclosedSteps() []appstep.Step {
	if !e.discloseInnerSteps {
		return e.Steps
	}
	return append(slices.Clip(e.Steps), From(e.inner).disclosedSteps()...)
}

func newError(inner error, c Code, msg string, details any) *Error {
	disclose := inner != nil && !encapsulating()
	if msg == "" {
		if disclose {
			msg = From(inner).clientMessage()
		} else {
			msg = c.c.message
		}
	}
	return &Error{Code: c, Message: msg, Details: details, inner: inner, discloseInnerSteps: disclose}
}

// Error is the log form: the steps, outermost first, then the qualified code and message,
// then the inner chain. Sensitive step parameters are redacted.
func (e *Error) Error() string {
	var b strings.Builder
	for _, s := range e.Steps {
		b.WriteString(s.String())
		b.WriteString(": ")
	}
	b.WriteString(e.Code.Qualified())
	b.WriteString(": ")
	b.WriteString(e.Message)
	if e.inner != nil {
		b.WriteString(": ")
		b.WriteString(e.inner.Error())
	}
	return b.String()
}

// Unwrap returns the inner error.
func (e *Error) Unwrap() error { return e.inner }

// Is lets errors.Is match the error's code, or the public code it maps to.
func (e *Error) Is(target error) bool { return matches(e.Code, target) }

// DetailsOf finds the formal error with code c in err's chain, or with an
// internal code mapped to c, and returns its details as c types them. ok
// reports whether the chain has c at all; the details are zero when the error
// carries none.
//
// It reads err's own details at c, which a translation does not change: the
// library's error beneath a host's Translate keeps the library's.
func DetailsOf[D any](err error, c TypedCode[D]) (details D, ok bool) {
	want := c.Code()
	has := func(k Code) bool { return k == want || (k.c != nil && k.c.public == want.c) }
	var found *Error
	walk(err, func(err error) bool {
		switch x := err.(type) {
		case *Error:
			if has(x.Code) {
				found, ok = x, true
			}
		case AnyCode:
			ok = has(x.Code())
		}
		return ok
	})
	if found != nil {
		details, _ = found.Details.(D)
	}
	return details, ok
}

// walk visits err's chain depth first, as errors.Is does, until visit returns
// true.
func walk(err error, visit func(error) bool) bool {
	if err == nil {
		return false
	}
	if visit(err) {
		return true
	}
	switch x := err.(type) {
	case interface{ Unwrap() error }:
		return walk(x.Unwrap(), visit)
	case interface{ Unwrap() []error }:
		for _, e := range x.Unwrap() {
			if walk(e, visit) {
				return true
			}
		}
	}
	return false
}
