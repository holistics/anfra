package query

import (
	"context"
	"errors"
	"strings"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/apperr"
)

// failed formalises canal-query's failure to run a query as query_failed, with
// canal's message. A client-scope error from canal is a request the engine
// built wrong — the engine's bug, not the data source's — and stays
// unclassified, as does anything else.
func failed(err error) error {
	e, ok := errors.AsType[*sidecar.CanalQueryError](err)
	if !ok || e.Scope == "Client" {
		return err
	}
	return apperr.Encapsulate(err, errcode.QueryFailed, e.Message)
}

// compileAQL compiles a query (see query.Compile), with anfra-node's refusal of
// what shapes the run as the violation of the arg at fault.
func compileAQL(ctx context.Context, cc command.CommandContext, dataset, aql string, run query.Run) (sidecar.CompileToSQLResult, error) {
	compiled, err := query.Compile(ctx, cc.Clients.Node, cc.Repo, dataset, aql, run)
	if v, ok := runViolation(err); ok {
		return sidecar.CompileToSQLResult{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
	}
	return compiled, err
}

// runViolation is anfra-node's refusal of what shapes a run, as the violation of
// the arg at fault: a Query Input entry, under input (its path is the same in the
// API, which passes the Query Input through), or the paging of a query that
// cannot be paged.
func runViolation(err error) (apperr.Violation, bool) {
	e, ok := errors.AsType[*sidecar.RPCError](err)
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
