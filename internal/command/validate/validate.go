// Package validate is the validate command: the repo's AML, checked.
package validate

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/aml"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/repoformat"
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
		res, err := aml.Validate(ctx, cc.Clients.Node, cc.Repo, in.Globs)
		if err != nil {
			return res, err
		}
		// A managed repo's Data Apps were checked where they were published from.
		if local, ok := cc.Repo.Local(); ok {
			res.Reports = append(res.Reports, appReports(local.Dir, in.Globs)...)
		}
		return res, nil
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

// appReports are what in the repo's Data Apps will not work in a host (repoformat.CheckApp), as
// warnings: they never make the repo invalid. Only the apps the globs select, when there are any:
// a glob matching the app's path, or a folder holding it.
func appReports(dir string, globs []string) []anfranode.ValidationReport {
	var reports []anfranode.ValidationReport
	_ = filepath.WalkDir(filepath.Join(dir, repoformat.AppsDir), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry has nothing to check
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil //nolint:nilerr // not under the repo
		}
		rel = filepath.ToSlash(rel)
		if !repoformat.DataApp(rel) || !selected(rel, globs) {
			return nil
		}
		content, err := os.ReadFile(p) //nolint:gosec // G304: a Data App under the repo's apps/
		if err != nil {
			return nil //nolint:nilerr // gone since the walk saw it
		}
		for _, issue := range repoformat.CheckApp(content) {
			reports = append(reports, anfranode.ValidationReport{
				Validator: "data_app", FilePath: rel, Type: "data_app", Severity: "warning", Message: issue.String(),
			})
		}
		return nil
	})
	return reports
}

func selected(rel string, globs []string) bool {
	if len(globs) == 0 {
		return true
	}
	for _, g := range globs {
		g = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(g)), "/")
		if ok, _ := path.Match(g, rel); ok || g == "." || strings.HasPrefix(rel, g+"/") {
			return true
		}
	}
	return false
}
