// Package validate is the use-case for validating: the AML repo, and a
// single query against a dataset.
package validate

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/apperr"
)

// QueryInvalid is a query that cannot run or compile, with its diagnostics. A
// command that judges the query answers a QueryValidation instead.
var QueryInvalid = apperr.DefinePublicCodeWith[QueryValidation](errcode.NS, "query_invalid", apperr.User, "The query is invalid.")

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

// Repo validates the AML repo via the node sidecar. paths (optional) are the
// file/dir/glob selectors — expanded in the node; empty validates the whole repo.
func Repo(ctx context.Context, node *sidecar.AnfraNodeClient, r repo.Repo, paths []string) (RepoValidation, error) {
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

// QueryValidation is the outcome of validating a query: its verdict, and the
// type-check diagnostics for a single AQL query.
type QueryValidation struct {
	// Valid: no diagnostic has "error" severity.
	Valid       bool                    `json:"valid"`
	Diagnostics []sidecar.AQLDiagnostic `json:"diagnostics"`
}

func queryValid(diags []sidecar.AQLDiagnostic) bool {
	for _, d := range diags {
		if d.Severity == "error" {
			return false
		}
	}
	return true
}

// AQL type-checks a single AQL query against a dataset and returns its
// diagnostics (which field/why), rather than failing on the first error.
// dataset is required.
func AQL(ctx context.Context, node *sidecar.AnfraNodeClient, r repo.Repo, dataset, aql string) (QueryValidation, error) {
	req, err := query.CompileRequest(r, dataset, aql)
	if err != nil {
		return QueryValidation{}, err
	}
	res, err := node.ValidateAQL(ctx, req)
	if err != nil {
		return QueryValidation{}, fmt.Errorf("validate AQL for dataset %q: %w", dataset, err)
	}
	return QueryValidation{Valid: queryValid(res.Diagnostics), Diagnostics: res.Diagnostics}, nil
}
