package query

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// QueryInvalid is a query that cannot run or compile, with its diagnostics: the
// QueryValidation that says why. A command that judges the query answers the
// QueryValidation instead.
var QueryInvalid = apperr.DefinePublicCodeWith[QueryValidation](errcode.NS, "query_invalid", apperr.User, "The query is invalid.")

// QueryValidation is the outcome of validating a query: its verdict, and the
// type-check diagnostics for a single AQL query.
type QueryValidation struct {
	// Valid: no diagnostic has "error" severity.
	Valid       bool                      `json:"valid"`
	Diagnostics []anfranode.AQLDiagnostic `json:"diagnostics"`
}

func queryValid(diags []anfranode.AQLDiagnostic) bool {
	for _, d := range diags {
		if d.Severity == "error" {
			return false
		}
	}
	return true
}

// Check type-checks a single AQL query against a dataset and returns its
// diagnostics (which field/why), rather than failing on the first error.
// dataset is required.
func Check(ctx context.Context, node *anfranode.Client, r repo.Repo, dataset, aql string) (QueryValidation, error) {
	req, err := CompileRequest(r, dataset, aql)
	if err != nil {
		return QueryValidation{}, err
	}
	res, err := node.ValidateAQL(ctx, req)
	if err != nil {
		return QueryValidation{}, fmt.Errorf("validate AQL for dataset %q: %w", dataset, err)
	}
	return QueryValidation{Valid: queryValid(res.Diagnostics), Diagnostics: res.Diagnostics}, nil
}
