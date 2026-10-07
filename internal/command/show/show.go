// Package show is the show command: an object of the repo, read from its
// compiled AML, by the identity the semantic catalog gives it.
package show

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/aml"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// Show shows the repo, or a dataset in full.
var Show = command.Define(command.Def[ShowInput, anfranode.ShowResult]{
	Name:  "show",
	Short: "Show the repo's datasets, or a dataset in full: its models, fields and metrics",
	Long: "Show an object of the repo, read from its compiled AML: with no fqn, the repo and its datasets\n" +
		"in outline; with a dataset's fqn, that dataset in full, as AQL can query it. Its interface, not\n" +
		"its implementation: names, labels, types, roles and AQL definitions, never SQL or tables. The\n" +
		"repo's files that do not compile come with it, as diagnostics.",
	ReadOnly:   true,
	Idempotent: true,
	Timeout:    2 * time.Minute,
	Needs:      func(ShowInput) command.Sidecars { return command.Sidecars{Node: true} },
	Run: func(ctx context.Context, cc command.CommandContext, in ShowInput) (anfranode.ShowResult, error) {
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true}); err != nil {
			return anfranode.ShowResult{}, err
		}
		// The type is the fqn's: with no fqn, the repo, whatever the type defaulted to.
		fqn, typ := strings.TrimSpace(in.Fqn), ""
		if fqn != "" {
			typ = in.Type
		}
		res, err := aml.Show(ctx, cc.Clients.Node, cc.Repo, typ, fqn)
		if v, ok := refused(err); ok {
			return anfranode.ShowResult{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
		}
		return res, err
	},
})

// ShowInput is what to show, as the semantic catalog identifies it: an entity
// type and its fqn. No fqn: the repo.
type ShowInput struct {
	Fqn  string `json:"fqn,omitempty" cli:"positional" doc:"the fqn of the object to show; absent, the repo"`
	Type string `json:"type,omitempty" enum:"aml.dataset" default:"aml.dataset" doc:"the catalog's entity type of fqn; aml.dataset is the only one for now"`
}

func (ShowInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[ShowInput](s)
}

// refused is anfra-node's refusal of what to show, as the violation of the arg
// at fault: an fqn it found nothing at, or a type it does not show.
func refused(err error) (apperr.Violation, bool) {
	e, ok := errors.AsType[*anfranode.RPCError](err)
	if !ok {
		return apperr.Violation{}, false
	}
	switch path, _ := e.Path(); path {
	case "fqn", "type":
		return apperr.Violation{Field: path, Code: "invalid", Message: e.Message}, true
	}
	return apperr.Violation{}, false
}
