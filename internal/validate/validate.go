// Package validate is the use-case for validating an AML repo.
package validate

import (
	"context"
	"fmt"

	"github.com/anfra-ai/anfra/internal/query"
	"github.com/anfra-ai/anfra/internal/repo"
	"github.com/anfra-ai/anfra/internal/sidecar"
)

// Result is the outcome of validating a repo: files that failed to compile plus
// the validator suite's findings.
type Result struct {
	CompileErrors []sidecar.CompileError     `json:"compileErrors"`
	Reports       []sidecar.ValidationReport `json:"reports"`
}

// Invalid reports whether the repo is invalid: any file failed to compile, or any
// validator reported an "error"-severity finding.
func (r *Result) Invalid() bool {
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
func Repo(ctx context.Context, node *sidecar.AnfraNodeClient, r repo.Repo, paths []string) (*Result, error) {
	res, err := node.ValidateAML(ctx, sidecar.ValidateAMLRequest{RepoPath: r.Dir, Paths: paths})
	if err != nil {
		return nil, fmt.Errorf("validate AML for repo %q: %w", r.Dir, err)
	}
	return &Result{CompileErrors: res.CompileErrors, Reports: res.Reports}, nil
}

// AQLResult is the `query --validate` result: type-check diagnostics for a single
// AQL query. No error-severity diagnostics means the query is valid.
type AQLResult struct {
	Diagnostics []sidecar.AQLDiagnostic `json:"diagnostics"`
}

// Invalid reports whether the query has any error-severity diagnostic.
func (r *AQLResult) Invalid() bool {
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
func AQL(ctx context.Context, node *sidecar.AnfraNodeClient, r repo.Repo, dataset, aql string) (*AQLResult, error) {
	if dataset == "" {
		return nil, fmt.Errorf("dataset is required")
	}
	req, err := query.CompileRequest(r, dataset, aql)
	if err != nil {
		return nil, err
	}
	res, err := node.ValidateAQL(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("validate AQL for dataset %q: %w", dataset, err)
	}
	return &AQLResult{Diagnostics: res.Diagnostics}, nil
}
