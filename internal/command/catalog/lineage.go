package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
)

// Lineage traces fields, or whole models and datasets, through the local
// catalog: what each one is built from.
var Lineage = command.Define(command.Def[LineageInput, LineageResult]{
	Name:  "lineage",
	Short: "Trace fields through the local catalog to what each one is built from",
	Long: "Trace each field through the local catalog to the fields, models, and sources it is built from. " +
		"Name a field by its parent (a model or a dataset) and its name in it. Leave out the name to trace the whole model or dataset, with every field in it. " +
		"Return each target's lineage as a diagram to draw, with cards in layers from upstream to downstream, and the links between them.",
	ReadOnly:   true,
	Idempotent: true,
	Needs:      func(LineageInput) command.Sidecars { return command.Sidecars{Node: true, CanalQuery: true} },
	Check:      LineageInput.check,
	Run: func(ctx context.Context, cc command.CommandContext, in LineageInput) (LineageResult, error) {
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true, CanalQuery: true}); err != nil {
			return LineageResult{}, err
		}
		targets := in.targets()
		res, err := cc.Clients.Node.CatalogLineage(ctx, anfranode.CatalogLineageRequest{
			Context: anfranode.CatalogContextOf(cc.Repo, cc.Clients.CanalQuery.BaseURL()),
			Targets: targets, Limit: in.Limit, Offset: in.Offset,
		})
		if err != nil {
			return LineageResult{}, err
		}
		if len(res.Results) != len(targets) {
			return LineageResult{}, fmt.Errorf("anfra-node traced %d targets, asked %d", len(res.Results), len(targets))
		}
		out := LineageResult{Results: make([]FieldLineage, len(targets))}
		for i, r := range res.Results {
			if r.Target != targets[i] {
				return LineageResult{}, fmt.Errorf("anfra-node traced %s where %s was asked", r.Target, targets[i])
			}
			out.Results[i] = FieldLineage{Target: in.Targets[i], Query: r.Query, Diagram: r.Diagram, Error: r.Error}
		}
		return out, nil
	},
})

// LineageInput is lineage's input: the fields or parents to trace, and the
// page of what each one is built from.
type LineageInput struct {
	Targets []anfranode.LineageTarget `json:"targets,omitempty" doc:"the fields or parents to trace. Name a field by its parent and its name in it: {\"parent\": \"orders\", \"child\": \"amount\"}. Name a parent alone to trace it with every field in it: {\"parent\": \"orders\"}"`
	Limit   int                       `json:"limit,omitempty" minimum:"1" doc:"the most upstream entities to return per target, all of them by default"`
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
		return command.InvalidArg("targets", "required", "name at least one field or parent to trace")
	}
	for i, t := range targets {
		if t.Parent == "" || (t.Child == "" && in.Targets[i].Child != "") {
			return command.InvalidArg("targets", "invalid", "each target needs a non-blank `parent`, and a non-blank `child` if it has one")
		}
	}
	return nil
}

// LineageResult is lineage's answer: each target's lineage, in the order of
// the targets asked.
type LineageResult struct {
	Results []FieldLineage `json:"results"`
}

// FieldLineage is one target's lineage, or why it could not be traced.
type FieldLineage struct {
	Target  anfranode.LineageTarget `json:"target" doc:"the field or parent, as asked"`
	Query   string                  `json:"query" doc:"the search query that finds the target's lineage, which the search command also takes. Empty when the target could not be searched for."`
	Diagram *anfranode.Diagram      `json:"diagram,omitempty" doc:"the target's lineage. Absent when error is set."`
	Error   string                  `json:"error,omitempty" doc:"why the target's lineage could not be traced, such as a parent that does not exist, or a parent without that child"`
}
