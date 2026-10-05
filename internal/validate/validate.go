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

// RepoValidation is the outcome of validating the repo: files that
// failed to compile plus the validator suite's findings.
type RepoValidation struct {
	CompileErrors []sidecar.CompileError     `json:"compileErrors"`
	Reports       []sidecar.ValidationReport `json:"reports"`
}

// Invalid reports whether the repo is invalid: any file failed to compile, or any
// validator reported an "error"-severity finding.
func (r RepoValidation) Invalid() bool {
	if len(r.CompileErrors) > 0 {
		return true
	}
	for _, rep := range r.Reports {
		if rep.Severity == "error" {
			return true
		}
	}
	return false
}

// Repo validates the AML repo via the node sidecar. paths (optional) are the
// file/dir/glob selectors — expanded in the node; empty validates the whole repo.
func Repo(ctx context.Context, node *sidecar.AnfraNodeClient, r repo.Repo, paths []string) (RepoValidation, error) {
	res, err := node.ValidateAML(ctx, sidecar.ValidateAMLRequest{RepoPath: r.Dir, RepoID: r.ID, Paths: paths})
	if err != nil {
		return RepoValidation{}, fmt.Errorf("validate AML for repo %q: %w", r.Dir, err)
	}
	return RepoValidation{CompileErrors: res.CompileErrors, Reports: res.Reports}, nil
}

// QueryValidation is the outcome of validating a query: type-check diagnostics
// for a single AQL query. No error-severity diagnostics means the query is valid.
type QueryValidation struct {
	Diagnostics []sidecar.AQLDiagnostic `json:"diagnostics"`
}

// Invalid reports whether the query has any error-severity diagnostic.
func (r QueryValidation) Invalid() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == "error" {
			return true
		}
	}
	return false
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
	return QueryValidation{Diagnostics: res.Diagnostics}, nil
}
