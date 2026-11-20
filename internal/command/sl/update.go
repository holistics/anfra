package sl

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// Update generates updates to an existing table model using catalog metadata.
var Update = command.Define(command.Def[UpdateInput, Result]{
	Name:       "sl.update",
	Short:      "Update an AML model from its Postgres table",
	Long:       "Update a table model using metadata already in the catalog. Keep custom fields, comments and user properties. The CLI writes the file only when it changes. Other callers receive the content.\n\nRun ingest against the same running server first to load the current database schema.",
	ReadOnly:   true,
	Idempotent: true,
	Needs:      func(UpdateInput) command.Sidecars { return command.Sidecars{Node: true} },
	Check:      UpdateInput.check,
	Run: func(ctx context.Context, cc command.CommandContext, in UpdateInput) (Result, error) {
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true}); err != nil {
			return Result{}, err
		}
		file, err := cc.Clients.Node.UpdateModel(ctx, anfranode.UpdateModelRequest{
			Context: anfranode.CatalogContextOf(cc.Repo, ""), ModelName: in.ModelName,
		})
		if v, ok := anfranode.Violation(err, anfranode.ArgMap{"modelName": "model_name"}); ok {
			return Result{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
		}
		return modelResult(file, true), err
	},
})

// UpdateInput names one AML table model.
type UpdateInput struct {
	ModelName string `json:"model_name" cli:"positional" doc:"the AML table model's name, with dots or :: separating namespaces"`
}

func (in UpdateInput) check() error {
	if strings.TrimSpace(in.ModelName) == "" || strings.ContainsRune(in.ModelName, '\x00') {
		return command.InvalidArg("model_name", "required", "Name the AML table model to update.")
	}
	return nil
}

func (UpdateInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[UpdateInput](s)
}
