// Package validate is the validate command: the repo's AML, checked.
package validate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/aml"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
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
		if v, ok := refused(err); ok {
			return aml.RepoValidation{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
		}
		if err != nil {
			return res, err
		}
		// A managed repo's Data Apps were checked where they were published from.
		local, isLocal := cc.Repo.Local()
		var apps []string
		if isLocal {
			apps = dataApps(local.Dir)
			res.Reports = append(res.Reports, appReports(local.Dir, apps, in.Globs)...)
		}
		if err := unmatched(in.Globs, res.Unmatched, apps, isLocal); err != nil {
			return aml.RepoValidation{}, err
		}
		return res, nil
	},
	Valid: func(r aml.RepoValidation) bool { return r.Valid },
})

// ValidateInput is validate's input.
type ValidateInput struct {
	Globs []string `json:"globs,omitempty" cli:"positional" maxItems:"100" doc:"optional file globs, at most 100; report only diagnostics for matching files"`
}

func (ValidateInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[ValidateInput](s)
}

// unmatched refuses the globs that selected nothing: no AML file (unmatchedAML,
// as anfra-node answers it) and, in a local repo, no Data App. Validating nothing is not a
// clean repo, and a glob that selects nothing is most often not a path at all:
// a query passed where a file was meant.
func unmatched(globs, unmatchedAML, apps []string, local bool) error {
	none := map[string]bool{}
	for _, g := range unmatchedAML {
		none[g] = true
	}
	what := "no AML file"
	if local {
		what = "no AML file or Data App"
	}
	var vs []apperr.Violation
	for i, g := range globs {
		if !none[g] || slices.ContainsFunc(apps, func(app string) bool { return selected(app, []string{g}) }) {
			continue
		}
		msg := fmt.Sprintf("%q matches %s in the repo", g, what)
		if looksLikeQuery(g) {
			msg += "; to check a query, use core.query.validate (anfra query validate)"
		}
		vs = append(vs, apperr.Violation{Field: fmt.Sprintf("globs[%d]", i), Code: "invalid", Message: msg})
	}
	if len(vs) == 0 {
		return nil
	}
	return apperr.NewWith(apperr.ValidationFailed, vs[0].Message, apperr.Violate(vs...))
}

// callRe is a name called: sum(, select(. A glob's parentheses follow an
// extglob's operator instead (@(a|b), !(x)), and its braces are alternatives
// ({a,b}), so neither alone says query.
var callRe = regexp.MustCompile(`[\p{L}\p{N}_]\(`)

// looksLikeQuery reports whether a glob is more likely AQL than a path: it has
// whitespace (explore { … }, a | b) or calls a function (sum(orders.quantity)).
// A path with a space gets the hint too; it is only a hint.
func looksLikeQuery(g string) bool {
	return strings.ContainsFunc(g, unicode.IsSpace) || callRe.MatchString(g)
}

// refused is anfra-node's refusal of a glob (one resolving outside the repo), as
// the violation of that glob.
func refused(err error) (apperr.Violation, bool) {
	e, ok := errors.AsType[*anfranode.RPCError](err)
	if !ok {
		return apperr.Violation{}, false
	}
	path, _ := e.Path()
	i, ok := strings.CutPrefix(path, "paths[")
	if !ok {
		return apperr.Violation{}, false
	}
	return apperr.Violation{Field: "globs[" + i, Code: "invalid", Message: e.Message}, true
}

// dataApps are the repo's Data App definitions, repo-relative and slash-separated.
func dataApps(dir string) []string {
	var apps []string
	_ = filepath.WalkDir(filepath.Join(dir, repoformat.AppsDir), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry has nothing to check
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil //nolint:nilerr // not under the repo
		}
		if rel = filepath.ToSlash(rel); repoformat.DataApp(rel) {
			apps = append(apps, rel)
		}
		return nil
	})
	return apps
}

// appReports are what in the repo's Data Apps will not work in a host (repoformat.CheckApp), as
// warnings: they never make the repo invalid. Only the apps the globs select, when there are any:
// a glob matching the app's path, or a folder holding it.
func appReports(dir string, apps, globs []string) []anfranode.ValidationReport {
	var reports []anfranode.ValidationReport
	for _, rel := range apps {
		if !selected(rel, globs) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))) //nolint:gosec // G304: a Data App under the repo's apps/
		if err != nil {
			continue // gone since the walk saw it
		}
		for _, issue := range repoformat.CheckApp(content) {
			reports = append(reports, anfranode.ValidationReport{
				Validator: "data_app", FilePath: rel, Type: "data_app", Severity: "warning", Message: issue.String(),
			})
		}
	}
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
