// Package validate is the validate command: the repo's AML, checked.
package validate

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/aml"
	"github.com/holistics/anfra/internal/command"
)

// Validate checks the AML repo, optionally scoped to file globs.
var Validate = command.Define(command.Def[ValidateInput, aml.RepoValidation]{
	Name:     "validate",
	Short:    "Validate the AML repo, optionally scoped to file globs",
	ReadOnly: true,
	Timeout:  5 * time.Minute,
	Needs:    func(ValidateInput) command.Sidecars { return command.Sidecars{Node: true} },
	Run: func(ctx context.Context, cc command.CommandContext, in ValidateInput) (aml.RepoValidation, error) {
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true}); err != nil {
			return aml.RepoValidation{}, err
		}
		return aml.Validate(ctx, cc.Clients.Node, cc.Repo, in.Globs)
	},
	Valid: func(r aml.RepoValidation) bool { return r.Valid },
})

// ValidateInput is validate's input.
type ValidateInput struct {
	Globs []string `json:"globs,omitempty" cli:"positional" doc:"optional file globs; report only diagnostics for matching files"`
}

func (ValidateInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[ValidateInput](s)
}
