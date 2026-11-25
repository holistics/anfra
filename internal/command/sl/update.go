package sl

import (
	"context"
	"errors"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// Update updates the semantic layer by generating AML from catalog metadata.
var Update = command.Define(command.Def[UpdateInput, Result]{
	Name:       "sl.update",
	Short:      "Update the semantic layer from table metadata",
	Long:       "Update an AML table model in the repo from catalog metadata. Keep custom fields, comments, and user properties. Write the file only when its content changes.\n\nRun ingest against the same running server first to load the current database schema.",
	Errors:     []apperr.AnyCode{repo.WriteFailed},
	Idempotent: true,
	Needs:      func(UpdateInput) command.Sidecars { return command.Sidecars{Node: true} },
	Check:      UpdateInput.check,
	Run: func(ctx context.Context, cc command.CommandContext, in UpdateInput) (Result, error) {
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true}); err != nil {
			return Result{}, err
		}

		result, err := cc.Clients.Node.SLUpdate(ctx, anfranode.SLUpdateRequest{
			Context: anfranode.CatalogContextOf(cc.Repo, ""), ModelName: in.ModelName,
		})
		if err != nil {
			if v, ok := anfranode.Violation(err, anfranode.ArgMap{"modelName": "model_name"}); ok {
				return Result{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
			}
			return Result{}, err
		}
		if result.Changed == nil {
			return Result{}, errors.New("the model update did not report whether its content changed")
		}

		if *result.Changed {
			writes := cc.Repo.WriteFiles([]repo.File{{Path: result.Path, Content: result.Content}})
			if err := writes[0].Err; err != nil {
				return Result{}, err
			}
		}

		return newResult(result, *result.Changed), nil
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
