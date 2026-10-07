// Package aml is the engine's work on the repo's AML as a whole: validating it,
// and showing its objects. (A single query is the query package's.)
package aml

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
)

// RepoValidation is the outcome of validating the repo: its verdict, what in it
// failed to compile, and the validator suite's findings.
type RepoValidation struct {
	// Valid: no file failed to compile, and no validator reported an
	// "error"-severity finding.
	Valid bool `json:"valid"`
	// Diagnostics are what could not be read, and why: today, the files that
	// failed to compile. The key core.show uses for the same items.
	Diagnostics []anfranode.CompileError     `json:"diagnostics"`
	Reports     []anfranode.ValidationReport `json:"reports"`
}

func repoValid(diagnostics []anfranode.CompileError, reports []anfranode.ValidationReport) bool {
	if len(diagnostics) > 0 {
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
func Validate(ctx context.Context, node *anfranode.Client, r repo.Repo, paths []string) (RepoValidation, error) {
	res, err := node.ValidateAML(ctx, anfranode.ValidateAMLRequest{RepoPath: r.Dir, RepoID: r.ID, Paths: paths})
	if err != nil {
		return RepoValidation{}, fmt.Errorf("validate AML for repo %q: %w", r.Dir, err)
	}
	return RepoValidation{
		Valid:       repoValid(res.CompileErrors, res.Reports),
		Diagnostics: res.CompileErrors,
		Reports:     res.Reports,
	}, nil
}
