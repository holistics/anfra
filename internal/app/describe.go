package app

import (
	"reflect"
	"slices"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apperr"
)

// CommandSpec is a command as an API caller sees it: what a host publishes as
// an operation (a schema, a tool, a usage page) without re-declaring its args.
// Every field is derived from the command's Def.
type CommandSpec struct {
	Name  string // dotted by group: "query.compile"
	Short string // one line
	Long  string // optional; full usage
	Args  []ArgSpec
	// ExactlyOne lists groups of args of which exactly one must be set. Dispatch
	// enforces it; this is for documentation and schemas.
	ExactlyOne [][]string
	// Output is the type of the command's answer, a Response's Data. A host
	// builds its schema by reflection; the type need not be importable.
	Output reflect.Type
	// CanBeInvalid reports whether the command can answer StatusInvalid: it
	// judges something, and the answer says what is wrong.
	CanBeInvalid bool
	// ErrorCodes lists the codes the command can fail with, each with its scope
	// and details type.
	ErrorCodes []apperr.Code
}

// ArgSpec is an arg as an API caller sees it.
type ArgSpec struct {
	Name     string  // the /call key
	Type     ArgType // string, bool or string_array
	Required bool
	Enum     []string // the allowed values, when closed
	Default  string   // the value when unset, if any
	Usage    string
}

// Describe returns every registered command, in registry order, in the shape
// an API caller sees. The CLI's shape stays in the CLI: aliases, shorthands,
// stdin and positional are left out (aliases are still accepted, but not
// advertised).
//
// The specs are copies: changing one changes nothing.
func Describe() []CommandSpec {
	specs := make([]CommandSpec, len(Commands))
	for i, c := range Commands {
		specs[i] = describe(c)
	}
	return specs
}

func describe(c Command) CommandSpec {
	args := c.Args()
	s := CommandSpec{Name: c.Name(), Short: c.Short(), Long: c.Long(), Output: c.Output(),
		CanBeInvalid: c.CanBeInvalid(), ErrorCodes: errorCodes(c)}
	for _, a := range args {
		s.Args = append(s.Args, ArgSpec{Name: a.Name, Type: a.Type, Required: a.Required, Enum: a.Enum, Default: a.Default, Usage: a.Usage})
	}
	for _, g := range groupNames(args) {
		s.ExactlyOne = append(s.ExactlyOne, groupMembers(args, g))
	}
	return s
}

// errorCodes are the codes c can fail with. Every command can be refused before
// it runs — undecided permissions, invalid args — one that needs a sidecar can
// find it unavailable, and a command declares the codes of its own.
func errorCodes(c Command) []apperr.Code {
	codes := []apperr.Code{errcode.DataPermsMissing, errcode.InvalidArgs.Code()}
	if c.needsSidecars() {
		codes = append(codes, errcode.SidecarUnavailable)
	}
	for _, e := range c.errors() {
		if !slices.Contains(codes, e.Code()) {
			codes = append(codes, e.Code())
		}
	}
	return codes
}
