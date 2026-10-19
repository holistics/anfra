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
	// QueryFailed: the data source did not run the query — the database
	// rejecting it, or the data source unreachable, or its credentials refused:
	// what canal-query scopes "User", without saying which. Each is the user's to
	// fix, the query or the data source, never the server's; so user scope, with
	// the data source's own message. canal-query failing itself ("Server"), or
	// refusing a request the engine built wrong ("Client"), is the server's, and
	// is no query_failed: an internal error.
	//
	// TODO: when canal-query says which of its "User" cases it is, a query the
	// database rejects becomes query_invalid's, and this the data source's.
	QueryFailed = apperr.DefinePublicCode(NS, "query_failed", apperr.User, "The query failed to run on its data source.")
	// DataPermsUnenforceable: the caller's data permissions are governed, and
	// the query cannot be — raw SQL, which no rule is compiled into. A host that
	// decided the caller may run it states that with Ungoverned.
	DataPermsUnenforceable = apperr.DefinePublicCode(NS, "data_perms_unenforceable", apperr.Client,
		"These data permissions cannot be applied to this query.")
	// ExportsUnavailable: the server stores no exports (it provides no
	// command.ExportStore), so an export cannot be made here.
	ExportsUnavailable = apperr.DefinePublicCode(NS, "exports_unavailable", apperr.Server, "This server cannot make exports.")
	// LocalRepoNotAllowed: an invocation on a local repo from a host that did not
	// allow local repos (CommandContext.AllowLocalRepo). A host mistake, as
	// DataPermsMissing is: a platform serving managed repos never builds a local
	// one, and one that serves a directory says so.
	LocalRepoNotAllowed = apperr.DefinePublicCode(NS, "local_repo_not_allowed", apperr.Client, "This host does not allow local repos.")
)
