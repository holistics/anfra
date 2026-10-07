package command

import (
	"context"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
)

// HostMistakes are the codes that mean the host built an invocation wrong: no
// data permissions stated, or restrictions on a query that cannot apply them.
// They are not part of the core API: no op declares them, and a host that hits
// one has a bug of its own.
var HostMistakes = []apperr.Code{errcode.DataPermsMissing, errcode.DataPermsUnenforceable}

// admission is the one precondition every command has: nothing runs until the
// host has stated what the caller may see (see app.Dispatch). The host's
// CommandContext is otherwise the handler's as it is.
var admission = apikit.Admission[CommandContext, CommandContext]{
	Admit: func(_ context.Context, cc CommandContext) (CommandContext, error) {
		if !cc.DataPerms.Decided() {
			return CommandContext{}, ErrDataPermsMissing()
		}
		return cc, nil
	},
}

// ErrDataPermsMissing is the refusal of an invocation whose data permissions
// nobody decided.
func ErrDataPermsMissing() error {
	return apperr.New(errcode.DataPermsMissing, "no data permissions supplied: every invocation must "+
		"state what the caller may see (dataperm.Unrestricted() when nothing is restricted)")
}

// NoInput is the input of a command that takes no args.
type NoInput struct{}

// InvalidArg is a refusal of the arg field: a violation with its code and
// message, as validation_failed.
func InvalidArg(field, code, msg string) error {
	return apperr.NewWith(apperr.ValidationFailed, msg, apperr.Violate(apperr.Violation{Field: field, Code: code, Message: msg}))
}

// RequireSidecars refuses a command whose sidecars are not connected, before it
// calls one: a host that dialled none, or the one-shot CLI before it spawned.
func RequireSidecars(cc CommandContext, need Sidecars) error {
	switch {
	case need.Node && cc.Clients.Node == nil:
		return apperr.New(errcode.SidecarUnavailable, "this command requires the anfra-node sidecar")
	case need.CanalQuery && cc.Clients.CanalQuery == nil:
		return apperr.New(errcode.SidecarUnavailable, "this command requires the canal-query sidecar")
	}
	return nil
}
