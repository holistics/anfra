package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
)

// NewRegistry is every command as an op, core.<command>, admitted from the
// CommandContext a host passes with each request: `anfra serve` passes its own,
// the one-shot CLI one over the sidecars it spawned.
func NewRegistry() *apikit.Registry[CommandContext] {
	reg := apikit.NewRegistry(apikit.RegistryConfig[CommandContext]{Namespace: errcode.NS})
	reg.Group("core", "anfra core's commands: query a dataset of the semantic layer, compile or validate a query, "+
		"validate the repo, and build and search its catalog.")
	for _, c := range Commands {
		c.register(reg)
	}
	return reg
}

// hostMistakes are the codes that mean the host built an invocation wrong: no
// data permissions stated, or restrictions on a query that cannot apply them.
// They are not part of the core API: no op declares them, and a host that hits
// one has a bug of its own.
var hostMistakes = []apperr.Code{errcode.DataPermsMissing, errcode.DataPermsUnenforceable}

// NewRuntime is the runtime the ops run with. The host-mistake codes are
// implied for every op.
func NewRuntime() *apikit.Runtime {
	rt := apikit.NewRuntime()
	rt.Implied = hostMistakes
	return rt
}

// admission is the one precondition every command has: nothing runs until the
// host has stated what the caller may see (see Dispatch). The host's
// CommandContext is otherwise the handler's as it is.
var admission = apikit.Admission[CommandContext, CommandContext]{
	Admit: func(_ context.Context, cc CommandContext) (CommandContext, error) {
		if !cc.DataPerms.Decided() {
			return CommandContext{}, errDataPermsMissing()
		}
		return cc, nil
	},
}

func errDataPermsMissing() error {
	return apperr.New(errcode.DataPermsMissing, "no data permissions supplied: every invocation must "+
		"state what the caller may see (dataperm.Unrestricted() when nothing is restricted)")
}

// dispatch is the registry and runtime Invoke runs ops with, built from the
// command list once — again only if the list is replaced, which tests do.
func dispatch() (*apikit.Registry[CommandContext], *apikit.Runtime) {
	built.mu.Lock()
	defer built.mu.Unlock()
	if built.reg == nil || len(built.from) != len(Commands) || (len(Commands) > 0 && &built.from[0] != &Commands[0]) {
		built.from, built.reg, built.rt = Commands, NewRegistry(), NewRuntime()
	}
	return built.reg, built.rt
}

var built struct {
	mu   sync.Mutex
	from []Command
	reg  *apikit.Registry[CommandContext]
	rt   *apikit.Runtime
}

// Invoke runs the command's op on its JSON input, and returns its answer.
func Invoke(ctx context.Context, cc CommandContext, command string, input []byte) (any, error) {
	reg, rt := dispatch()
	o, ok := reg.Lookup(OpName(command))
	if !ok {
		return nil, apperr.New(errcode.UnknownCommand, fmt.Sprintf("unknown command %q", command))
	}
	return o.Invoke(ctx, rt, cc, input)
}

// inputOf is a request's args as the op's JSON input.
func inputOf(args map[string]any) ([]byte, error) {
	if len(args) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(args)
}
