// Package catalog is the commands over the local search catalog: ingest builds
// it from the repo's context sources, search queries it, and lineage traces
// fields through it.
package catalog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/ingest"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/search"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// Ingest builds the local search catalog from context sources.
var Ingest = command.Define(command.Def[IngestInput, string]{
	Name:       "ingest",
	Short:      "Build the local search catalog from context sources",
	Idempotent: true, // rebuilds the same catalog
	Timeout:    30 * time.Minute,
	Needs:      func(IngestInput) command.Sidecars { return command.Sidecars{Node: true, CanalQuery: true} },
	Run: func(ctx context.Context, cc command.CommandContext, in IngestInput) (string, error) {
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true, CanalQuery: true}); err != nil {
			return "", err
		}
		local, err := localRepo(cc, "Ingest")
		if err != nil {
			return "", err
		}
		return ingest.Run(ctx, cc.Clients.Node, cc.Clients.CanalQuery, local, in.Source)
	},
})

// Search searches the local catalog.
var Search = command.Define(command.Def[SearchInput, anfranode.CatalogSearchResult]{
	Name:     "search",
	Short:    "Search the local catalog",
	ReadOnly: true,
	Needs:    func(SearchInput) command.Sidecars { return command.Sidecars{Node: true, CanalQuery: true} },
	Run: func(ctx context.Context, cc command.CommandContext, in SearchInput) (anfranode.CatalogSearchResult, error) {
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true, CanalQuery: true}); err != nil {
			return nil, err
		}
		local, err := localRepo(cc, "Search")
		if err != nil {
			return nil, err
		}
		return search.Run(ctx, cc.Clients.Node, cc.Clients.CanalQuery, local, strings.TrimSpace(strings.Join(in.Query, " ")))
	},
})

// localRepo is the repo's directory, where the catalog is built and read. cmd is
// the command, for its refusal on a managed repo, which is not supported yet.
func localRepo(cc command.CommandContext, cmd string) (repo.Local, error) {
	if cc.Repo.IsZero() {
		return repo.Local{}, fmt.Errorf("no repo: the host built an invocation without one")
	}
	local, ok := cc.Repo.Local()
	if !ok {
		return repo.Local{}, apperr.New(errcode.CatalogUnavailable, cmd+" is not supported for managed repos yet.")
	}
	return local, nil
}

// IngestInput is ingest's input.
type IngestInput struct {
	Source string `json:"source,omitempty" short:"s" doc:"optional context source key to ingest"`
}

func (IngestInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[IngestInput](s)
}

// SearchInput is search's input.
type SearchInput struct {
	Query []string `json:"query,omitempty" cli:"positional" doc:"search query"`
}

func (SearchInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[SearchInput](s)
}
