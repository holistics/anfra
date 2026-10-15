package query

import (
	"context"
	"errors"
	"strings"

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

// runViolation is anfra-node's refusal of what shapes a run, as the violation of
// the arg at fault: a Query Input entry, under input (its path is the same in the
// API, which passes the Query Input through), or the paging of a query that
// cannot be paged.
func runViolation(err error) (apperr.Violation, bool) {
	e, ok := errors.AsType[*anfranode.RPCError](err)
	if !ok {
		return apperr.Violation{}, false
	}
	path, ok := e.Path()
	if !ok {
		return apperr.Violation{}, false
	}
	msg := strings.TrimPrefix(e.Message, path+": ")
	switch path {
	case "pagination":
		return apperr.Violation{Field: "page_size", Code: "unsupported", Message: msg}, true
	case "":
		return apperr.Violation{Field: "input", Code: "invalid", Message: msg}, true
	}
	return apperr.Violation{Field: "input." + path, Code: "invalid", Message: msg}, true
}
