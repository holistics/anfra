package query

import (
	"context"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// sourceOf is what the invocation's queries compile against and run on: its
// repo, with the repo's data sources (command.CommandContext.ResolveDataSources).
func sourceOf(cc command.CommandContext) (query.Source, error) {
	sources, err := cc.ResolveDataSources()
	if err != nil {
		return query.Source{}, err
	}
	return query.Source{Repo: cc.Repo, DataSources: sources}, nil
}

// compileAQL compiles a query (see query.Compile) against src, with anfra-node's
// refusal of what shapes the run as the violation of the arg at fault.
func compileAQL(ctx context.Context, cc command.CommandContext, src query.Source, dataset, aql string, run query.Run) (anfranode.CompileToSQLResult, error) {
	compiled, err := query.Compile(ctx, cc.Clients.Node, src, dataset, aql, run)
	if v, ok := runViolation(err); ok {
		return anfranode.CompileToSQLResult{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
	}
	return compiled, err
}

// runArgs are the args of a query command anfra-node's compile params are. A
// Query Input entry's path is the same under input in the API, which passes the
// Query Input through.
var runArgs = anfranode.ArgMap{"datasetFqn": "dataset", "input": "input", "pagination": "page_size"}

// runViolation is anfra-node's refusal of what the caller gave a run, as the
// violation of the arg at fault: a dataset that cannot be used, a Query Input
// entry, or the paging of a query that cannot be paged.
func runViolation(err error) (apperr.Violation, bool) {
	v, ok := anfranode.Violation(err, runArgs)
	if ok && v.Field == "page_size" {
		v.Code = "unsupported"
	}
	return v, ok
}

// datasetArgs are the args of query validate anfra-node's params are.
var datasetArgs = anfranode.ArgMap{"datasetFqn": "dataset"}

// checkAQL type-checks a query (see query.Check) against src, with anfra-node's
// refusal of the dataset as the violation of the dataset arg.
func checkAQL(ctx context.Context, cc command.CommandContext, src query.Source, dataset, aql string) (query.QueryValidation, error) {
	res, err := query.Check(ctx, cc.Clients.Node, src, dataset, aql)
	if v, ok := anfranode.Violation(err, datasetArgs); ok {
		return query.QueryValidation{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
	}
	return res, err
}
