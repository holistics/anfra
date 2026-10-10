// Package errcode is the engine's namespace, and the error codes no one domain
// owns: what Dispatch can fail with whatever a command does. A code that belongs
// to a domain lives with it, beside the details it carries (query_invalid, with
// its QueryValidation, is the query package's). The engine package re-exports
// every code, and lists them all, by namespace, with engine.ErrorCodes.
//
// The codes carry a scope but no status: the CLI has none, and a host assigns
// its own when it translates an engine error to one of its codes. Args the
// engine refuses are apperr's generic validation_failed, with a violation per
// arg by its canonical name. Anything else Dispatch returns is unclassified,
// and a host treats it as internal_server_error.
package errcode

import "github.com/holistics/anfra/shared/apperr"

// NS is the engine's namespace, for its codes and its steps.
var NS = apperr.DefineNamespace("anfra")

var (
	UnknownCommand     = apperr.DefinePublicCode(NS, "unknown_command", apperr.Client, "No such command.")
	DataPermsMissing   = apperr.DefinePublicCode(NS, "data_perms_missing", apperr.Client, "No data permissions were decided.")
	SidecarUnavailable = apperr.DefinePublicCode(NS, "sidecar_unavailable", apperr.Server, "A sidecar is unavailable.")
	// QueryFailed: canal-query could not run a query on its data source — the
	// data source unreachable, the credentials refused, the database rejecting
	// the query, or canal itself failing. canal-query scopes the first three all
	// "User" and does not say which, so they are not classified further yet.
	//
	// TODO: classify by canal's scope, re-decided here, never copied: "User"
	// splits into a query the database rejects (the caller's: query_invalid)
	// and a data source it cannot reach or log in to (not the caller's);
	// "Server" is a dependency failing; "Client" is a request the engine built
	// wrong — its own bug, and already left unclassified.
	QueryFailed = apperr.DefinePublicCode(NS, "query_failed", apperr.Server, "The query failed to run on its data source.")
	// DataPermsUnenforceable: the caller's data permissions are restricted, and
	// the query cannot apply them — raw SQL, which no restriction is compiled
	// into. The host decided the caller may run it; it states that with
	// Unrestricted.
	DataPermsUnenforceable = apperr.DefinePublicCode(NS, "data_perms_unenforceable", apperr.Client,
		"These data permissions cannot be applied to this query.")
	// ExportsUnavailable: the server stores no exports (it provides no
	// command.ExportStore), so an export cannot be made here.
	ExportsUnavailable = apperr.DefinePublicCode(NS, "exports_unavailable", apperr.Server, "This server cannot make exports.")
)
