package app

import (
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/command/catalog"
	"github.com/holistics/anfra/internal/command/query"
	"github.com/holistics/anfra/internal/command/status"
	"github.com/holistics/anfra/internal/command/validate"
)

// Commands is the registry — the single source for the CLI, the ops `anfra
// serve` serves, and Describe. Add a command here and it appears on every
// surface (and in help). Each answers one type: a command's answer never
// depends on its args.
var Commands = []command.Command{
	status.Version,
	status.Status,
	query.Query,
	query.Compile,
	query.Validate,
	catalog.Ingest,
	catalog.Search,
	validate.Validate,
}

// Find returns the registered command by name.
func Find(name string) (command.Command, bool) {
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
