// Package aml is the engine's work on the repo's AML as a whole: validating
// it. (A single query is the query package's.)
package aml

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

// RepoValidation is the outcome of validating the repo: its verdict, files that
// failed to compile, and the validator suite's findings.
type RepoValidation struct {
	// Valid: no file failed to compile, and no validator reported an
	// "error"-severity finding.
	Valid         bool                       `json:"valid"`
	CompileErrors []sidecar.CompileError     `json:"compileErrors"`
	Reports       []sidecar.ValidationReport `json:"reports"`
}

func repoValid(compileErrors []sidecar.CompileError, reports []sidecar.ValidationReport) bool {
	if len(compileErrors) > 0 {
		return false
	}
	for _, rep := range reports {
		if rep.Severity == "error" {
			return false
		}
	}
	return true
}

// Validate validates the AML repo via the node sidecar. paths (optional) are the
// file/dir/glob selectors — expanded in the node; empty validates the whole repo.
func Validate(ctx context.Context, node *sidecar.AnfraNodeClient, r repo.Repo, paths []string) (RepoValidation, error) {
	res, err := node.ValidateAML(ctx, sidecar.ValidateAMLRequest{RepoPath: r.Dir, RepoID: r.ID, Paths: paths})
	if err != nil {
		return RepoValidation{}, fmt.Errorf("validate AML for repo %q: %w", r.Dir, err)
	}
	return RepoValidation{
		Valid:         repoValid(res.CompileErrors, res.Reports),
		CompileErrors: res.CompileErrors,
		Reports:       res.Reports,
	}, nil
}
