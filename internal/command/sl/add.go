// Package sl exposes semantic-layer commands over the repo's catalog.
package sl

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// Add adds to the semantic layer by generating AML from catalog metadata.
var Add = command.Define(command.Def[AddInput, Result]{
	Name:  "sl.add",
	Short: "Add a table to the semantic layer",
	Long: "Add a table to the semantic layer by generating an AML model from catalog metadata and writing it under models in the repo.\n\n" +
		"Run ingest first against the same running server.",
	Errors:     []apperr.AnyCode{repo.WriteFailed},
	Idempotent: true,
	Needs:      func(AddInput) command.Sidecars { return command.Sidecars{Node: true} },
	Check:      AddInput.check,
	Run: func(ctx context.Context, cc command.CommandContext, in AddInput) (Result, error) {
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true}); err != nil {
			return Result{}, err
		}

		result, err := cc.Clients.Node.SLAdd(ctx, anfranode.SLAddRequest{
			Context: anfranode.CatalogContextOf(cc.Repo, ""), Table: in.Table, DataSource: in.DataSource,
		})
		if err != nil {
			if v, ok := anfranode.Violation(err, anfranode.ArgMap{"table": "table", "dataSource": "data_source"}); ok {
				return Result{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
			}
			return Result{}, err
		}

		writes := cc.Repo.WriteFiles([]repo.File{{Path: result.Path, Content: result.Content}})
		if err := writes[0].Err; err != nil {
			return Result{}, err
		}

		return newResult(result, true), nil
	},
})

// AddInput selects one table and the data source that owns it.
type AddInput struct {
	Table      string `json:"table" cli:"positional" doc:"the table as schema.table, using its exact name without SQL identifier quotes"`
	DataSource string `json:"data_source" doc:"the Postgres data source that was ingested"`
}

func (in AddInput) check() error {
	parts := strings.Split(in.Table, ".")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.ContainsAny(in.Table, "\"\x00") {
		return command.InvalidArg("table", "invalid", "Name one table as schema.table, without SQL identifier quotes.")
	}

	if strings.TrimSpace(in.DataSource) == "" {
		return command.InvalidArg("data_source", "required", "Name the Postgres data source that was ingested.")
	}

	return nil
}

func (AddInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[AddInput](s)
}
