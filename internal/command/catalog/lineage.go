package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
)

// Lineage traces fields through the local catalog: what each one is built from.
var Lineage = command.Define(command.Def[LineageInput, LineageResult]{
	Name:  "lineage",
	Short: "Trace fields through the local catalog: what each one is built from",
	Long: "Trace each field through the local catalog to what it is built from (the fields, models and\n" +
		"sources upstream of it), as a diagram: cards in layers, upstream first, linked by what uses it.",
	ReadOnly:   true,
	Idempotent: true,
	Needs:      func(LineageInput) command.Sidecars { return command.Sidecars{Node: true, CanalQuery: true} },
	Check:      LineageInput.check,
	Run: func(ctx context.Context, cc command.CommandContext, in LineageInput) (LineageResult, error) {
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true, CanalQuery: true}); err != nil {
			return LineageResult{}, err
		}
		local, err := localRepo(cc, "Lineage")
		if err != nil {
			return LineageResult{}, err
		}
		targets := in.targets()
		res, err := cc.Clients.Node.CatalogLineage(ctx, anfranode.CatalogLineageRequest{
			Context: anfranode.CatalogContext{
				RepoPath:          local.Dir,
				SourcesPath:       local.ContextSourcesPath(),
				CanalQueryBaseURL: cc.Clients.CanalQuery.BaseURL(),
			},
			Targets: targets, Limit: in.Limit, Offset: in.Offset,
		})
		if err != nil {
			return LineageResult{}, err
		}
		if len(res.Results) != len(targets) {
			return LineageResult{}, fmt.Errorf("anfra-node traced %d fields, asked %d", len(res.Results), len(targets))
		}
		out := LineageResult{Results: make([]FieldLineage, len(targets))}
		for i, r := range res.Results {
			if r.Target != targets[i] {
				return LineageResult{}, fmt.Errorf("anfra-node traced %s.%s where %s.%s was asked", r.Target.Parent, r.Target.Child, targets[i].Parent, targets[i].Child)
			}
			out.Results[i] = FieldLineage{Target: in.Targets[i], Query: r.Query, Diagram: r.Diagram, Error: r.Error}
		}
		return out, nil
	},
})

// LineageInput is lineage's input: the fields to trace, and the page of what
// each one is built from.
type LineageInput struct {
	Targets []anfranode.LineageTarget `json:"targets,omitempty" doc:"the fields to trace, each by its parent and its name in it: {\"parent\": \"orders\", \"child\": \"amount\"}"`
	Limit   int                       `json:"limit,omitempty" minimum:"1" doc:"at most this many upstream entities per field; absent, all of them"`
	Offset  int                       `json:"offset,omitempty" minimum:"0" doc:"skip this many upstream entities first, to page with limit"`
}

func (LineageInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[LineageInput](s)
}

func (in LineageInput) targets() []anfranode.LineageTarget {
	targets := make([]anfranode.LineageTarget, len(in.Targets))
	for i, t := range in.Targets {
		targets[i] = anfranode.LineageTarget{Parent: strings.TrimSpace(t.Parent), Child: strings.TrimSpace(t.Child)}
	}
	return targets
}

func (in LineageInput) check() error {
	targets := in.targets()
	if len(targets) == 0 {
		return command.InvalidArg("targets", "required", "name at least one field to trace")
	}
	for _, t := range targets {
		if t.Parent == "" || t.Child == "" {
			return command.InvalidArg("targets", "invalid", "name each field by its parent and its name in it, neither blank")
		}
	}
	return nil
}

// LineageResult is lineage's answer: each field's lineage, in the order of the
// fields asked.
type LineageResult struct {
	Results []FieldLineage `json:"results"`
}

// FieldLineage is one field's lineage, or why it could not be traced.
type FieldLineage struct {
	Target  anfranode.LineageTarget `json:"target" doc:"the field, as asked"`
	Query   string                  `json:"query" doc:"the catalog search its lineage is, which search also takes; empty when the field could not be searched for"`
	Diagram *anfranode.Diagram      `json:"diagram,omitempty" doc:"the field's lineage; absent with error"`
	Error   string                  `json:"error,omitempty" doc:"why the field's lineage could not be traced"`
}
