package app

import (
	"context"
	"fmt"
	"reflect"

	"github.com/holistics/anfra/shared/apperr"
)

// Def is the single definition of one anfra operation: what it takes (In),
// what it answers (Out), and how it runs. It drives every surface: the CLI
// builds the command and its flags from it, /call dispatches to it by Name,
// and Describe publishes it.
//
// In is a struct whose fields are the command's args, described by field tags
// (see Arg). Out is the answer: Run returns one, and Valid, if set, judges it.
type Def[In, Out any] struct {
	// Name is dotted by group: "query.compile" is the CLI's `query compile`.
	Name  string
	Short string // one line
	Long  string // optional; full usage
	// Needs declares the sidecars the command needs for in (so the one-shot CLI
	// knows what to spawn; under `serve` they are all warm regardless). Nil: none.
	Needs func(in In) Sidecars
	Run   func(ctx context.Context, cc CommandContext, in In) (Out, error)
	// Valid reports whether an answer is valid. Set for a command whose job is
	// to judge something (a validator, a health check): an answer it reports
	// invalid is StatusInvalid, which is an outcome, not an error. Nil: every
	// answer is StatusOK.
	Valid func(out Out) bool
	// Errors are the codes Run itself fails with, beyond those every command can
	// (undecided permissions, invalid args) and sidecar_unavailable, implied by
	// Needs.
	Errors []apperr.AnyCode
}

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
	// CanBeInvalid reports whether the command can answer StatusInvalid.
	CanBeInvalid() bool
	// Needs is the sidecars the command needs for these args; none when they do
	// not decode, since Dispatch will refuse them.
	Needs(args map[string]any) Sidecars
	errors() []apperr.AnyCode
	needsSidecars() bool
	dispatch(ctx context.Context, cc CommandContext, args map[string]any) (Response, error)
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

func (c *command[In, Out]) Needs(args map[string]any) Sidecars {
	if c.def.Needs == nil {
		return Sidecars{}
	}
	in, err := decode[In](c.def.Name, c.args, args)
	if err != nil {
		return Sidecars{}
	}
	return c.def.Needs(in)
}

func (c *command[In, Out]) dispatch(ctx context.Context, cc CommandContext, args map[string]any) (Response, error) {
	in, err := decode[In](c.def.Name, c.args, args)
	if err != nil {
		return Response{}, err
	}
	out, err := c.def.Run(ctx, cc, in)
	if err != nil {
		return Response{}, err
	}
	st := StatusOK
	if c.def.Valid != nil && !c.def.Valid(out) {
		st = StatusInvalid
	}
	return Response{Status: st, Data: out}, nil
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
