// Package errcode is the engine's catalog of error codes: what Dispatch can
// fail with, for a caller to handle. The engine package re-exports them, and
// lists them with engine.ErrorCodes.
//
// The codes carry a scope but no status: the CLI has none, and a host assigns
// its own when it translates an engine error to one of its codes. Anything
// else Dispatch returns is unclassified, and a host treats it as
// internal_server_error.
package errcode

import "github.com/holistics/anfra/shared/apperr"

// NS is the engine's namespace, for its codes and its steps.
var NS = apperr.DefineNamespace("anfra")

var (
	UnknownCommand = apperr.DefinePublicCode(NS, "unknown_command", apperr.Client, "No such command.")
	// InvalidArgs carries one violation per arg the caller got wrong, by its
	// canonical name.
	InvalidArgs        = apperr.DefinePublicCodeWith[apperr.Violations](NS, "invalid_args", apperr.Client, "The arguments are invalid.")
	DataPermsMissing   = apperr.DefinePublicCode(NS, "data_perms_missing", apperr.Client, "No data permissions were decided.")
	SidecarUnavailable = apperr.DefinePublicCode(NS, "sidecar_unavailable", apperr.Server, "A sidecar is unavailable.")
)
