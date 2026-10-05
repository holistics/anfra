package apperr

import "sync/atomic"

// encapsulationDisabled is false by default — the safe mode, in which a client
// never sees what an error encapsulates. Only DisableErrorEncapsulation sets it.
var encapsulationDisabled atomic.Bool

// DisableErrorEncapsulation makes every error show its cause to clients, for a
// local entry point whose user is its operator: the person looking at a failed
// chart is the one who can fix it, and the log is not where they look.
//
// From then on:
//   - Encapsulate discloses the inner error's steps, as Translate does, and with
//     no message of its own shows the inner error's rather than its code's
//     default.
//   - An internal_server_error shows its own message, an unclassified one the
//     text of the error it formalised, instead of the generic message.
//
// Call it once, at the start of such an entry point, before any error is made.
// It cannot be undone, and a host serving other people must never call it: the
// default keeps causes hidden.
func DisableErrorEncapsulation() { encapsulationDisabled.Store(true) }

func encapsulating() bool { return !encapsulationDisabled.Load() }

// unclassifiedMessage is an unclassified error's message: for the log, never a
// client.
const unclassifiedMessage = "unclassified"

// clientMessage is the message a client sees for e. An internal_server_error's
// own message is a diagnosis, so a client sees the generic one, unless
// encapsulation is disabled: then its own, or for an unclassified error, the
// text of the error it formalised.
func (e *Error) clientMessage() string {
	if e.Code.Public() != InternalServerError {
		return e.Message
	}
	if encapsulating() {
		return InternalServerError.c.message
	}
	if e.Message == unclassifiedMessage && e.inner != nil {
		return e.inner.Error()
	}
	return e.Message
}
