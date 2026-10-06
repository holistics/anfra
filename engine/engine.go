// Package engine is the anfra engine's public Go API: with the shared packages
// it classifies its errors with (shared/apperr, shared/appstep and shared/apptracing), the
// only packages outside internal/ that another module may import.
//
// It exists so that a separate application — a server, a hosted product, a
// different front end — can run anfra commands in-process, with the same
// registry and therefore the same semantics as the `anfra` CLI. Everything
// reaches commands through Dispatch, so the two surfaces cannot drift.
//
// # What this package is for
//
// Keeping the surface small is the point, not an accident. Everything the
// engine knows how to do is reachable through Dispatch, and everything else
// here exists only to build one call: connect to sidecars, open a repo, state
// what the caller may see. If a symbol below is not needed for that, it should
// not be here — and if something in internal/ is needed and is not here, adding
// it is a deliberate act rather than an import away.
//
// # What this package is NOT for
//
// The engine makes no access decisions. There is no principal, no role, no
// policy, and no notion of a tenant anywhere in this API. A host decides
// whether a caller may run a command before it calls Dispatch; what crosses
// this boundary is the consequence of that decision — the data restrictions
// that apply — and never its inputs. See DataPerms and Attribution, which look
// alike and are emphatically not the same thing.
//
// # Stability
//
// This package is versioned with the module. Everything under internal/ is not
// part of it and may change in any release; if you find yourself wanting
// something from there, open an issue rather than a fork.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/validate"
	"github.com/holistics/anfra/shared/apperr"
)

// The types below are aliases, not wrappers: an engine.Request IS an
// app.Request. That keeps this package free of conversion code that could drift
// from what it converts, and lets a caller implement or compose these types
// without importing anything internal.
type (
	// Request is a command invocation: a name plus its arguments. It mirrors a
	// CLI invocation, and it is the untrusted half of a call — decode it from a
	// request body if you like.
	Request = app.Request

	// Response is the envelope every command returns: an explicit status plus a
	// result payload.
	Response = app.Response

	// Status is a command's explicit outcome. A command that ran correctly but
	// produced an invalid result (a failed validation, say) reports
	// StatusInvalid, which is not an error.
	Status = app.Status

	// Clients are the sidecar clients commands run against. Build one with
	// Connect.
	Clients = app.Clients

	// Repo is a resolved AML repo: its directory, its identity, and where its
	// state lives. Build one with OpenRepo.
	Repo = repo.Repo

	// Attributes are caller attributes that data restrictions are evaluated
	// against — a region, a department, an email. They are decision-bearing: the
	// compiler reads them, and changing one changes which rows come back.
	Attributes = dataperm.Attributes

	// DataPerms is what a caller may see, for one invocation. It cannot be built
	// with a struct literal: use Unrestricted or Restricted, so that "nothing is
	// restricted" is something a host states rather than something it defaults
	// into by leaving a field blank.
	DataPerms = dataperm.Set

	// Attribution names the caller of one invocation for logs, spans and audit
	// records — a tenant, a user, a session, a request id, whatever you can
	// attribute by.
	//
	// It is tracing baggage and nothing else. No command reads it and it never
	// reaches the compiler. If a value must change which rows a query returns,
	// it belongs in Attributes, and the engine enforces that separation rather
	// than trusting this comment.
	Attribution = attribution.Fields

	// Invocation is everything a command runs against other than its arguments.
	// It is the trusted half of a call: build it server-side and never decode it
	// from a request body, or a caller could assert their own permissions.
	Invocation = app.CommandContext
)

const (
	// StatusOK reports a command that ran and produced a valid result.
	StatusOK = app.StatusOK
	// StatusInvalid reports a command that ran correctly and produced a result
	// that is not valid — validation diagnostics, typically. It is an outcome,
	// not a failure to run.
	StatusInvalid = app.StatusInvalid
)

// The codes Dispatch fails with, beside apperr.ValidationFailed for args it
// refuses (an arg unknown, missing, malformed, conflicting with another, or not
// applicable; its violations name each one). They carry a scope and no status:
// a host translates them to its own codes where it calls Dispatch
// (apperr.Translate), and treats anything else Dispatch returns as an internal
// error.
var (
	// UnknownCommand: the request names no registered command.
	UnknownCommand = errcode.UnknownCommand
	// DataPermsMissing: the Invocation's DataPerms were never decided.
	DataPermsMissing = errcode.DataPermsMissing
	// DataPermsUnenforceable: the Invocation's DataPerms are Restricted, and the
	// query cannot apply restrictions (raw SQL). A host that authorized the
	// caller on the data source states Unrestricted.
	DataPermsUnenforceable = errcode.DataPermsUnenforceable
	// SidecarUnavailable: a sidecar the command needs did not respond, or is
	// not connected.
	SidecarUnavailable = errcode.SidecarUnavailable
	// QueryInvalid: a query that cannot run or compile, with its diagnostics.
	QueryInvalid = validate.QueryInvalid
	// QueryFailed: the data source could not run the query — unreachable,
	// credentials refused, or the database rejecting it. Not classified further
	// yet.
	QueryFailed = errcode.QueryFailed
)

// Namespace is the engine's namespace, for its codes and its steps: anfra.
var Namespace = errcode.NS

// ErrorCodes lists the engine's own codes Dispatch can fail with — every one
// but the generic validation_failed — with its scope and details type: for a
// host to test that it translates each of them, and fail its build on one added
// by an engine upgrade.
func ErrorCodes() []apperr.Code { return apperr.Codes(errcode.NS) }

// Unrestricted states that no data restrictions apply to this caller. It is an
// answer, not a default: Dispatch refuses an Invocation whose DataPerms nobody
// set, so a host that has not thought about permissions cannot accidentally get
// unrestricted access.
func Unrestricted() DataPerms { return dataperm.Unrestricted() }

// Restricted carries the attributes of a caller whose access depends on them.
func Restricted(attrs Attributes) DataPerms { return dataperm.Restricted(attrs) }

// OpenRepo resolves an AML repo directory to its identity and state layout. It
// creates no directories and does not validate the contents.
//
// Unlike the CLI, which derives a repo from the working directory, a server has
// no meaningful working directory and must be told.
func OpenRepo(dir string) Repo { return repo.Resolve(dir) }

// The command metadata Describe returns: aliases, not wrappers, like the types
// above.
type (
	// CommandSpec is a command as an API caller sees it.
	CommandSpec = app.CommandSpec
	// ArgSpec is one of a command's args as an API caller sees it.
	ArgSpec = app.ArgSpec
	// ArgType is an arg's type: ArgString, ArgBool or ArgStringArray.
	ArgType = app.ArgType
)

const (
	ArgString      = app.ArgString
	ArgBool        = app.ArgBool
	ArgStringArray = app.ArgStringArray
)

// Describe returns every registered command, in registry order, in the shape an
// API caller sees: its args with their types, which are required and which are
// closed to a set of values, the groups of which exactly one must be set, the
// type of its answer and whether it can be invalid, and the codes it can fail
// with. A host publishes each as an operation (an OpenAPI
// path, an MCP tool, a usage page) without re-declaring what the engine knows.
//
// It is the API's view, not the CLI's: aliases, shorthands and stdin are left
// out. Dispatch enforces what it says, and every field is derived from the
// command's definition, so it cannot drift from what the command does. The specs
// are copies.
func Describe() []CommandSpec { return app.Describe() }

// Commands returns the names of every registered command, in registry order.
//
// Names only: what a command *requires* of a caller is not the engine's
// question, so no permission vocabulary crosses this boundary. This exists so a
// host that maintains its own command-to-permission mapping can test that the
// mapping still covers the registry after an engine upgrade — and fail its
// build rather than silently exposing a new command.
func Commands() []string { return app.Names() }

// Connect dials sidecars that something else is running — the compose or
// Kubernetes topology, where anfra-node and canal-query are separate
// containers. Both URLs are required; this never starts a process.
//
// The returned Closer releases the clients. It does not stop the sidecars,
// because this process does not own them.
//
// Spawning sidecars is deliberately not exposed. It is how the local CLI works,
// where one short-lived process owns its children, and it is the wrong shape for
// a server: a long-lived host wants sidecars with their own lifecycle, restart
// policy and scaling.
func Connect(ctx context.Context, nodeURL, canalQueryURL string, opts ...ConnectOption) (Clients, io.Closer, error) {
	if nodeURL == "" || canalQueryURL == "" {
		return Clients{}, nil, errors.New("engine.Connect: both nodeURL and canalQueryURL are required")
	}
	cfg := sidecar.Config{
		NodeURL:       nodeURL,
		CanalQueryURL: canalQueryURL,
		// Long-lived host: canal reuses warehouse connections across requests.
		EnablePooling: true,
	}
	var opt connectConfig
	for _, o := range opts {
		o(&opt)
	}
	cfg.WrapTransport = opt.wrap

	node := sidecar.NewAnfraNode(cfg)
	if err := node.Start(ctx); err != nil {
		return Clients{}, nil, fmt.Errorf("engine.Connect: anfra-node at %s: %w", nodeURL, err)
	}
	canal := sidecar.NewCanalQuery(cfg)
	if err := canal.Start(ctx); err != nil {
		node.Close()
		return Clients{}, nil, fmt.Errorf("engine.Connect: canal-query at %s: %w", canalQueryURL, err)
	}

	clients := Clients{Node: node.Client(), CanalQuery: canal.Client()}
	return clients, closerFunc(func() error {
		canal.Close()
		node.Close()
		return nil
	}), nil
}

// ConnectOption configures Connect.
type ConnectOption func(*connectConfig)

type connectConfig struct {
	wrap func(sidecar string, rt http.RoundTripper) http.RoundTripper
}

// WithTransport wraps each sidecar client's HTTP transport, e.g. with
// otelhttp.NewTransport, so a host traces calls to the sidecars and propagates
// its trace context into them. wrap receives the engine's own transport, so its
// settings (timeouts, pooling) are kept, and the sidecar's name ("anfra-node",
// "canal-query") to label spans by. It applies once a sidecar is ready:
// readiness polling is not wrapped.
//
// The engine takes no tracing dependency for this: the host brings the wrapper.
func WithTransport(wrap func(sidecar string, rt http.RoundTripper) http.RoundTripper) ConnectOption {
	return func(c *connectConfig) { c.wrap = wrap }
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// Dispatch runs one command and returns its Response.
//
// It refuses an Invocation whose DataPerms nobody decided — every command, with
// no exemptions. That is not an authorization decision: the engine does not
// know who is calling or what they are entitled to. It is the guarantee that
// nothing runs until the host has said what the caller may see.
func Dispatch(ctx context.Context, inv Invocation, req Request) (Response, error) {
	return app.Dispatch(ctx, inv, req)
}
