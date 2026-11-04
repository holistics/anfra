// Package clierr is the CLI's own error codes: the failures of its CLI-only
// commands (and of what they call), which no op has, so they're in no host's
// catalog (docs/implementations/cli-json.md, step 3). Three are anfra-cloud's,
// mirrored with their meaning and details, so a client reads one code whichever
// side found the failure: unauthenticated, unavailable, repo_invalid.
package clierr

import (
	"encoding/json/jsontext"
	"fmt"

	"github.com/holistics/anfra/shared/apperr"
)

// NS is the CLI's namespace, for its codes.
var NS = apperr.DefineNamespace("anfra_cli")

var (
	// CommandFailed: a CLI-only command didn't do its job, for a reason the
	// user can fix, in a way no program branches on yet, as query_failed is for
	// a query. Its details, when it has any, are the command's to say, such as
	// its result as far as it got. A failure a program comes to act on gets a
	// code of its own.
	CommandFailed = apperr.DefinePublicCodeWith[any](NS, "command_failed", apperr.User, "The command failed.")

	// RepoInvalid: the repo fails validation, here before a publish as on the
	// org's server, whose anfra-cloud code it is, with the same details. Any
	// command refused for an invalid repo fails with it; were an op to, it would
	// move to the engine's namespace with its name and details, so a client sees
	// no change.
	RepoInvalid = apperr.DefinePublicCodeWith[RepoProblems](NS, "repo_invalid", apperr.User, "The repo has errors.")

	// Unauthenticated: no usable login for the org, saved here (none, or one
	// that can't be used) or as its server says: anfra-cloud's code. A program
	// asks the user to sign in (anfra auth login), which takes a browser.
	Unauthenticated = apperr.DefinePublicCode(NS, "unauthenticated", apperr.User, "Sign in to continue.")

	// OrgNotSet: the repo has no org to publish to, and no terminal to choose
	// one. A program passes --set-org, or asks the user which org.
	OrgNotSet = apperr.DefinePublicCode(NS, "org_not_set", apperr.User, "The repo has no org to publish to.")

	// ConfirmationRequired: the command needs the user's yes, with no terminal
	// to ask on. A program asks the user, and passes --yes only on a yes: never
	// on its own.
	ConfirmationRequired = apperr.DefinePublicCode(NS, "confirmation_required", apperr.User, "This needs your confirmation.")

	// Interrupted: the user stopped the command: Ctrl-C, a no to its question,
	// a sign-in denied in the browser. A program neither retries nor reports a
	// failure. Not anfra-cloud's cancelled: an operation someone else stopped.
	Interrupted = apperr.DefinePublicCode(NS, "interrupted", apperr.User, "Interrupted.")

	// PreconditionFailed: things aren't as the command needs them, and the
	// user can change that, as the message says: a build from source to update,
	// no coding agent, a file anfra won't write through, a port taken.
	PreconditionFailed = apperr.DefinePublicCode(NS, "precondition_failed", apperr.User, "The command can't run as things are.")

	// Unavailable: a service anfra needs didn't answer, or not as it should:
	// anfra-cloud's code, with which service. A program retries later.
	Unavailable = apperr.DefinePublicCodeWith[Service](NS, "unavailable", apperr.Server, "A service anfra needs is unavailable. Try again shortly.")
)

// RepoProblems are repo_invalid's details, as anfra-cloud's.
type RepoProblems struct {
	// Validation is validate's answer: the diagnostics to fix.
	Validation jsontext.Value `json:"validation"`
}

// Service is unavailable's details: which service, anfra's or a third
// party's, and when to retry, when it said.
type Service struct {
	Service           string `json:"service"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

// The services the CLI depends on.
const (
	GitHub     = "github"      // releases, the changelog, the skills' manifest
	MotherDuck = "motherduck"  // its CLI and API, for a copy of a DuckDB file
	Org        = "org"         // the org's anfra-cloud server
	Auth       = "auth"        // the org's authentication server
	AnfraServe = "anfra_serve" // the repo's warm server
)

// Precondition is a precondition_failed saying what to change.
func Precondition(format string, args ...any) error {
	return apperr.New(PreconditionFailed, fmt.Sprintf(format, args...))
}

// Unreachable is service failing, cause the error that says how, as its
// message.
func Unreachable(service string, cause error) error {
	return apperr.EncapsulateWith(cause, Unavailable, "", Service{Service: service})
}

// Unreachablef is service failing, as format says, cause kept for the log.
func Unreachablef(service string, cause error, format string, args ...any) error {
	return apperr.EncapsulateWith(cause, Unavailable, fmt.Sprintf(format, args...), Service{Service: service})
}
