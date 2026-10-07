// Package app is the single registry of anfra operations. Each command is
// defined once, as a command.Def in its group's package under internal/command,
// and listed once, in Commands; it drives every surface: the `anfra` CLI
// generates its commands and flags from it, the command is an apikit op
// (core.<command>) that `anfra serve` serves and the CLI invokes, and Describe
// publishes it. Add a command in one place and it shows up everywhere — no
// per-side registration to forget.
package app

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/command"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apperr"
)

// Request mirrors a CLI invocation: a command name + its args, as the op's JSON
// input has them.
type Request struct {
	Command string         `json:"command"`
	Args    map[string]any `json:"args"`
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
// makes no access decisions (see command.CommandContext). What it does guarantee is that
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
func Dispatch(ctx context.Context, cc command.CommandContext, req Request) (Response, error) {
	if !cc.DataPerms.Decided() {
		return Response{}, command.ErrDataPermsMissing()
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
