// Package app is the single registry of anfra operations. Each Command is
// defined once (in commands.go) and drives BOTH surfaces: the `anfra` CLI
// generates its cobra command + flags from it, and `anfra serve`'s `POST /call`
// dispatches to it by name. Add a command in one place → it shows up on both
// sides (and in help) — no per-side registration to forget.
package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/anfra-ai/anfra/internal/repo"
	"github.com/anfra-ai/anfra/internal/sidecar"
)

// Request mirrors a CLI invocation: a command name + its args.
type Request struct {
	Command string         `json:"command"`
	Args    map[string]any `json:"args"`
}

// Clients are the sidecar clients a command runs against. A client is nil when
// the command doesn't need that sidecar (see Command.Needs).
type Clients struct {
	Node       *sidecar.AnfraNodeClient
	CanalQuery *sidecar.CanalQueryClient
}

// Sidecars declares which sidecars a command needs (so the one-shot CLI knows
// what to spawn; under `serve` they're all warm regardless).
type Sidecars struct {
	Node       bool
	CanalQuery bool
}

// ArgType is the type of a command argument (drives the generated CLI flag).
type ArgType string

const (
	ArgString ArgType = "string"
	ArgBool   ArgType = "bool"
)

// Alias is an alternate name for an Arg (an extra CLI flag + /call key), folded
// into the canonical Name by NormalizeReqArgs. Usage overrides the help text for
// this name; if empty it falls back to the Arg's Usage.
type Alias struct {
	Name  string
	Usage string
}

// Arg declares a command argument: a CLI flag + a /call args key.
type Arg struct {
	Name    string
	Type    ArgType
	Usage   string
	Aliases []Alias
}

// Positional describes a command's variadic positional arg: on the CLI it's the
// trailing args (`validate a.aml b.aml`), on /call a string-array arg by Name.
type Positional struct {
	Name  string
	Usage string
}

// Command is the single definition of an anfra operation.
type Command struct {
	Name  string
	Short string
	Args  []Arg
	// Positional, if set, collects the command's trailing positional args.
	Positional *Positional
	// StdinArg, if set, is the one string arg that falls back to piped stdin when
	// empty (CLI only). Only one arg per command may read stdin.
	StdinArg string
	// Needs declares the sidecars required for the given args (arg-dependent).
	// nil => no sidecars.
	Needs func(args map[string]any) Sidecars
	Run   func(ctx context.Context, clients Clients, repo repo.Repo, args map[string]any) (any, error)
}

// Find returns the registered command by name.
func Find(name string) (Command, bool) {
	for _, c := range Commands {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

// Status is the explicit outcome of a command, carried in every Response.
type Status string

const (
	StatusOK      Status = "ok"
	StatusInvalid Status = "invalid" // command ran fine but its result isn't valid (e.g. validation errors)
)

// Response is the envelope every command returns: an explicit status plus the
// result payload. The CLI renders Data and maps Status to its exit code; /call
// callers get both. A command that needs a non-ok status returns a Response from
// its Run; anything else is wrapped as StatusOK.
type Response struct {
	Status Status `json:"status"`
	Data   any    `json:"data"`
}

// Dispatch runs a registered command and returns its Response envelope. An empty
// command lists the commands.
func Dispatch(ctx context.Context, clients Clients, repo repo.Repo, req Request) (Response, error) {
	if req.Command == "" {
		names := make([]string, len(Commands))
		for i, c := range Commands {
			names[i] = c.Name
		}
		return Response{Status: StatusOK, Data: map[string]any{"commands": names}}, nil
	}
	cmd, ok := Find(req.Command)
	if !ok {
		return Response{}, fmt.Errorf(`unknown command %q; send {"help": true} to list commands`, req.Command)
	}
	NormalizeReqArgs(cmd, req.Args)
	if err := validateReqArgs(cmd, req.Args); err != nil {
		return Response{}, err
	}
	res, err := cmd.Run(ctx, clients, repo, req.Args)
	if err != nil {
		return Response{}, err
	}
	if resp, ok := res.(Response); ok {
		return resp, nil // command set its own status
	}
	return Response{Status: StatusOK, Data: res}, nil
}

// validateReqArgs rejects args the command doesn't declare (the /call analog of
// cobra's "unknown flag" rejection). Call after NormalizeReqArgs so folded
// aliases don't trip it. `help` is always allowed (it's the universal arg).
func validateReqArgs(c Command, args map[string]any) error {
	known := map[string]bool{"help": true}
	for _, a := range c.Args {
		known[a.Name] = true
		for _, alias := range a.Aliases {
			known[alias.Name] = true
		}
	}
	if c.Positional != nil {
		known[c.Positional.Name] = true
	}
	var unknown []string
	for k := range args {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf(`unknown arg(s) %s for command %q; send {"command": %q, "help": true} for its args`,
		strings.Join(unknown, ", "), c.Name, c.Name)
}

// NormalizeReqArgs folds each arg's aliases into its canonical name and drops the
// alias keys, so command logic (and Needs) only ever reads the canonical arg.
// Safe to call more than once (idempotent) and on a nil map.
func NormalizeReqArgs(c Command, args map[string]any) {
	if args == nil {
		return
	}
	for _, a := range c.Args {
		for _, alias := range a.Aliases {
			v, ok := args[alias.Name]
			if !ok {
				continue
			}
			delete(args, alias.Name)
			switch a.Type {
			case ArgBool:
				if !IsTruthy(args[a.Name]) && IsTruthy(v) {
					args[a.Name] = true
				}
			case ArgString:
				if s, _ := args[a.Name].(string); strings.TrimSpace(s) == "" {
					args[a.Name] = v
				}
			}
		}
	}
}

// IsTruthy reports whether a /call arg value (which arrives as arbitrary JSON)
// is true. Only these are falsy: absent/null, false, a numeric zero, and an
// empty/whitespace string. Everything else is truthy — including non-empty
// strings ("1", "10", even "false"), objects, and arrays.
func IsTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return strings.TrimSpace(t) != ""
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	default:
		return true
	}
}

func argString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

// argStrings reads a string-list arg, accepting []string (CLI positional) or
// []any (a JSON array over /call).
func argStrings(args map[string]any, key string) []string {
	switch v := args[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
