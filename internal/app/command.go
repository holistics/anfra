package app

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
)

// Def is the single definition of one anfra operation: what it takes (In),
// what it answers (Out), and how it runs. It drives every surface: the CLI
// builds the command and its flags from it, `anfra serve` serves it as the op
// core.<Name>, and Describe publishes it.
//
// In is a struct whose fields are the command's args, described by field tags
// (see Arg); it is also the op's input type. Out is the answer: Run returns one,
// and Valid, if set, judges it.
type Def[In, Out any] struct {
	// Name is dotted by group: "query.compile" is the CLI's `query compile`, and
	// the op core.query.compile.
	Name  string
	Short string // one line
	Long  string // optional; full usage
	// Needs declares the sidecars the command needs for in (so the one-shot CLI
	// knows what to spawn; under `serve` they are all warm regardless). Nil: none.
	Needs func(in In) Sidecars
	Run   func(ctx context.Context, cc CommandContext, in In) (Out, error)
	// Valid reports whether an answer is valid. Set for a command whose job is
	// to judge something (a validator, a health check): an answer it reports
	// invalid is still an answer, carrying its verdict, not an error. Nil: every
	// answer is valid.
	Valid func(out Out) bool
	// Errors are the codes Run itself fails with, beyond those every command can
	// (undecided permissions, invalid args) and sidecar_unavailable, implied by
	// Needs.
	Errors []apperr.AnyCode
	// ReadOnly: the command changes nothing. Idempotent: running it again with
	// the same input has no further effect. Published on the op.
	ReadOnly   bool
	Idempotent bool
	// Timeout bounds the command as an op; zero means apikit's default.
	Timeout time.Duration
}

// OpName is the op a command is served as: core.<command>.
func OpName(command string) string { return "core." + command }

// Command is a registered Def, its types erased: what the registry holds and
// every surface reads. Only Define makes one.
type Command interface {
	Name() string
	Short() string
	Long() string
	// Args are the command's args, parsed from its In.
	Args() []Arg
	// Output is the type of the command's answer.
	Output() reflect.Type
	// CanBeInvalid reports whether the command's answer carries a verdict.
	CanBeInvalid() bool
	// Needs is the sidecars the command needs for this input, as the op's JSON;
	// none when it does not decode, since the op will refuse it.
	Needs(input []byte) Sidecars
	// Valid is an answer's verdict: an Out from an in-process call, or its JSON
	// from a server. A command that judges nothing answers true.
	Valid(out any) (bool, error)
	errors() []apperr.AnyCode
	needsSidecars() bool
	register(reg *apikit.Registry[CommandContext])
}

// Define registers nothing: it checks d and returns it as a Command, for the
// registry. Its In's tags are parsed here, so a malformed one panics at
// startup.
func Define[In, Out any](d Def[In, Out]) Command {
	if d.Name == "" || d.Short == "" || d.Run == nil {
		panic("app: a command needs a Name, a Short and a Run")
	}
	args, err := parseArgs(reflect.TypeFor[In]())
	if err != nil {
		panic(fmt.Sprintf("app: command %s: %v", d.Name, err))
	}
	if _, ok := any(new(In)).(huma.SchemaTransformer); len(args) > 0 && !ok {
		// Without it, the op's schema would miss the groups and required strings.
		panic(fmt.Sprintf("app: command %s: its input needs TransformSchema, returning argsSchema", d.Name))
	}
	return &command[In, Out]{def: d, args: args}
}

type command[In, Out any] struct {
	def  Def[In, Out]
	args []Arg
}

func (c *command[In, Out]) Name() string             { return c.def.Name }
func (c *command[In, Out]) Short() string            { return c.def.Short }
func (c *command[In, Out]) Long() string             { return c.def.Long }
func (c *command[In, Out]) Args() []Arg              { return cloneArgs(c.args) }
func (c *command[In, Out]) Output() reflect.Type     { return reflect.TypeFor[Out]() }
func (c *command[In, Out]) CanBeInvalid() bool       { return c.def.Valid != nil }
func (c *command[In, Out]) errors() []apperr.AnyCode { return c.def.Errors }
func (c *command[In, Out]) needsSidecars() bool      { return c.def.Needs != nil }

func (c *command[In, Out]) Needs(input []byte) Sidecars {
	if c.def.Needs == nil {
		return Sidecars{}
	}
	var in In
	if len(input) > 0 && json.Unmarshal(input, &in) != nil {
		return Sidecars{}
	}
	applyDefaults(c.args, &in)
	return c.def.Needs(in)
}

func (c *command[In, Out]) Valid(out any) (bool, error) {
	if c.def.Valid == nil {
		return true, nil
	}
	switch v := out.(type) {
	case []byte:
		var o Out
		if err := json.Unmarshal(v, &o); err != nil {
			return false, fmt.Errorf("decode %s's answer: %w", c.def.Name, err)
		}
		return c.def.Valid(o), nil
	case Out:
		return c.def.Valid(v), nil
	}
	return false, fmt.Errorf("%s answered a %T, not a %s", c.def.Name, out, reflect.TypeFor[Out]())
}

// register serves the command as the op core.<Name>: its In validated against
// its schema, its unset defaults applied, then Run.
func (c *command[In, Out]) register(reg *apikit.Registry[CommandContext]) {
	// The codes that mean the host built the invocation wrong are not the core
	// API's: a host that states the caller's data permissions, as every host
	// must, and refuses what they cannot apply, never answers with them
	// (NewRuntime implies them).
	var errs []apperr.AnyCode
	for _, code := range errorCodes(c) {
		if !slices.Contains(hostMistakes, code) {
			errs = append(errs, code)
		}
	}
	apikit.Register(reg, admission, &apikit.Def[CommandContext, In, Out]{
		Name: OpName(c.def.Name), Summary: c.def.Short, Doc: c.def.Long, Errors: errs,
		ReadOnly: c.def.ReadOnly, Idempotent: c.def.Idempotent, Timeout: c.def.Timeout, HTTP: true, MCP: true,
		Handle: func(ctx context.Context, cc CommandContext, in In) (Out, error) {
			applyDefaults(c.args, &in)
			return c.def.Run(ctx, cc, in)
		},
	})
}

// Find returns the registered command by name.
func Find(name string) (Command, bool) {
	for _, c := range Commands {
		if c.Name() == name {
			return c, true
		}
	}
	return nil, false
}

// Names returns every registered command's name, in registry order.
func Names() []string {
	names := make([]string, len(Commands))
	for i, c := range Commands {
		names[i] = c.Name()
	}
	return names
}
