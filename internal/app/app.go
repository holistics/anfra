// Package app is the single registry of anfra operations. Each command is
// defined once (in commands.go), as a Def, and drives every surface: the `anfra`
// CLI generates its commands and flags from it, the command is an apikit op
// (core.<command>) that `anfra serve` serves and the CLI invokes, and Describe
// publishes it. Add a command in one place and it shows up everywhere — no
// per-side registration to forget.
package app

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/apperr"
)

// Request mirrors a CLI invocation: a command name + its args, as the op's JSON
// input has them.
type Request struct {
	Command string         `json:"command"`
	Args    map[string]any `json:"args"`
}

// Clients are the sidecar clients a command runs against. A client is nil when
// the command doesn't need that sidecar (see Def.Needs).
type Clients struct {
	Node       *sidecar.AnfraNodeClient
	CanalQuery *sidecar.CanalQueryClient
}

// CommandContext is everything a command runs against, other than its args: the
// sidecars to call, the repo to act on, the data restrictions to apply, and who
// the invocation is for. It is host-constructed and never deserialized —
// Request is the caller-supplied half of an invocation, this is the trusted
// half, and that boundary is why the two are not one struct.
//
// Note what is absent: there is no principal, role or policy. The engine makes
// no access decisions. A host decides whether a caller may run a command before
// it builds one of these; what reaches the engine is the consequence of that
// decision, not its inputs.
type CommandContext struct {
	Clients Clients
	Repo    repo.Repo
	// DataPerms are the restrictions to compile into queries. Required for
	// EVERY invocation, not only the ones that read data — see Dispatch.
	DataPerms dataperm.Set
	// Attribution names the caller for logs and audit. Never read by a command,
	// and never by the query layer — see internal/attribution.
	Attribution attribution.Fields
	// Server is the warm server the command runs in, for status to report: nil
	// one-shot, and in a host that embeds the engine.
	Server *ServerInfo
}

// ServerInfo is a running `anfra serve`, as status reports it.
type ServerInfo struct {
	URL        string `json:"url"`
	InstanceID string `json:"instance_id"`
	Version    string `json:"version"`
}

// Sidecars declares which sidecars a command needs (so the one-shot CLI knows
// what to spawn; under `serve` they're all warm regardless).
type Sidecars struct {
	Node       bool
	CanalQuery bool
}

// Status is the explicit outcome of a command, carried in every Response.
type Status string

const (
	StatusOK      Status = "ok"
	StatusInvalid Status = "invalid" // command ran fine but its result isn't valid (e.g. validation errors)
)

// Response is what Dispatch returns: the answer, and its verdict as a status —
// StatusInvalid when the command's Valid judges the answer invalid, StatusOK
// otherwise. Over HTTP an answer stands alone and carries its verdict itself
// (valid, state); the envelope is Dispatch's, for a host embedding the engine.
type Response struct {
	Status Status `json:"status"`
	Data   any    `json:"data"`
}

// Dispatch runs a registered command and returns its Response envelope. An empty
// command lists the commands.
//
// This is not a policy enforcement point and has nothing to enforce: the engine
// makes no access decisions (see CommandContext). What it does guarantee is that
// nothing runs until the host has stated what the caller may see.
//
// That requirement applies to EVERY command, including ones that read no data.
// Exempting them would mean maintaining a list of which commands are exempt, and
// such a list is only ever wrong in one direction: mark a data-reading command
// exempt and it silently runs unrestricted. There is no list, so there is
// nothing to get wrong — and a host that knows who is calling already has the
// answer, so being asked for it costs nothing. The check is a precondition on
// the context rather than on the request, so it runs before the command is even
// looked up.
func Dispatch(ctx context.Context, cc CommandContext, req Request) (Response, error) {
	if !cc.DataPerms.Decided() {
		return Response{}, errDataPermsMissing()
	}
	if req.Command == "" {
		return Response{Status: StatusOK, Data: map[string]any{"commands": Names()}}, nil
	}
	cmd, ok := Find(req.Command)
	if !ok {
		return Response{}, apperr.New(errcode.UnknownCommand, fmt.Sprintf("unknown command %q", req.Command))
	}
	input, err := inputOf(req.Args)
	if err != nil {
		return Response{}, apperr.Encapsulate(err, apperr.InvalidRequest, "The args are not JSON.")
	}
	out, err := Invoke(ctx, cc, req.Command, input)
	if err != nil {
		return Response{}, err
	}
	st := StatusOK
	if valid, err := cmd.Valid(out); err != nil {
		return Response{}, err
	} else if !valid {
		st = StatusInvalid
	}
	return Response{Status: st, Data: out}, nil
}
