package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/jsonkit"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
)

// NewRegistry is every command as an op, core.<command>, admitted from the
// command.CommandContext a host passes with each request: `anfra serve` passes its own,
// the one-shot CLI one over the sidecars it spawned.
func NewRegistry() *apikit.Registry[command.CommandContext] {
	reg := apikit.NewRegistry(apikit.RegistryConfig[command.CommandContext]{Namespace: errcode.NS})
	reg.Group("core", "anfra core's commands: query a dataset of the semantic layer, compile or validate a query, "+
		"validate the repo, and build and search its catalog.")
	for _, c := range Commands {
		c.Register(reg)
	}
	return reg
}

// NewRuntime is the runtime the ops run with. The host-mistake codes are
// implied for every op.
func NewRuntime() *apikit.Runtime {
	rt := apikit.NewRuntime()
	rt.Implied = command.HostMistakes
	return rt
}

// dispatch is the registry and runtime Invoke runs ops with, built from the
// command list once — again only if the list is replaced, which tests do.
func dispatch() (*apikit.Registry[command.CommandContext], *apikit.Runtime) {
	built.mu.Lock()
	defer built.mu.Unlock()
	if built.reg == nil || len(built.from) != len(Commands) || (len(Commands) > 0 && &built.from[0] != &Commands[0]) {
		built.from, built.reg, built.rt = Commands, NewRegistry(), NewRuntime()
	}
	return built.reg, built.rt
}

var built struct {
	mu   sync.Mutex
	from []command.Command
	reg  *apikit.Registry[command.CommandContext]
	rt   *apikit.Runtime
}

// Invoke runs the command's op on its JSON input, and returns its answer.
func Invoke(ctx context.Context, cc command.CommandContext, name string, input []byte) (any, error) {
	reg, rt := dispatch()
	o, ok := reg.Lookup(command.OpName(name))
	if !ok {
		return nil, apperr.New(errcode.UnknownCommand, fmt.Sprintf("unknown command %q", name))
	}
	return o.Invoke(ctx, rt, cc, input)
}

// inputOf is a request's args as the op's JSON input.
func inputOf(args map[string]any) ([]byte, error) {
	if len(args) == 0 {
		return []byte("{}"), nil
	}
	return jsonkit.Marshal(args)
}
