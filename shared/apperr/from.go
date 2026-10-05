package apperr

// From returns the formal error a response is built from. Nil for nil.
//
// Only the outermost value counts: a formal error, or a bare code with its
// default message. Anything else — a plain error, a fmt.Errorf wrap, a
// third-party wrapper, a join — is unclassified, so internal. That is stricter
// than errors.As, which looks through a foreign wrapper; code that wraps with
// fmt.Errorf is flagged by the linter. A step never leaves a foreign wrapper
// outermost: it formalises whatever passes through it.
//
// Log err itself: its text is the whole chain.
func From(err error) *Error {
	switch x := err.(type) {
	case nil:
		return nil
	case *Error:
		return x
	case AnyCode:
		c := x.Code()
		return &Error{Code: c, Message: c.c.message}
	default:
		return unclassified(err)
	}
}

// unclassified formalises an error nobody classified as internal_server_error.
// Its message is for the log — a client sees that code's default message
// whatever the message is.
func unclassified(err error) *Error {
	return &Error{Code: InternalServerError, Message: unclassifiedMessage, inner: err}
}
