// Package query is the query commands: query runs a query, query.compile
// compiles one to SQL, and query.validate checks one. What they share, and only
// they, stays in here.
package query

import (
	"context"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/shared/apperr"
)

// Query runs a query: its rows, and the SQL that produced them — as one page,
// or streamed as it runs (QueryRun).
var Query = command.Define(command.Def[QueryRequestParams, QueryRun]{
	Name:  "query",
	Short: "Run a query, and return its rows and the SQL that produced them",
	Long: `Run a query, and return a page of its rows and the SQL that produced them.

A page is input.pagination.pageSize rows, or the first 10000 without it, and has_more says whether there may be more. Rather than paging through many rows, ask the query for what you need: an aggregate, a filter, or a limit: directive. To get every row of a large result, export it.

Over HTTP, a request that accepts application/x-ndjson gets the result as the query runs: its phase, its SQL, its columns, then its rows in batches. Without input.pagination, that's every row, up to the server's maximum.`,
	ReadOnly: true,
	Timeout:  10 * time.Minute,
	Needs: func(in QueryRequestParams) command.Sidecars {
		return command.Sidecars{Node: in.Lang != "sql", CanalQuery: true} // SQL is not compiled
	},
	Normalize: QueryRequestParams.normalize,
	Check:     QueryRequestParams.check,
	Errors:    []apperr.AnyCode{query.QueryInvalid, errcode.QueryFailed, errcode.DataPermsUnenforceable, query.ResultTooLarge, query.PageTooLarge},
	Events:    queryEvents,
	Run:       newQueryRun,
})

// Compile compiles a query to SQL, without running it.
var Compile = command.Define(command.Def[QueryCompileRequestParams, CompiledQuery]{
	Name:     "query.compile",
	Short:    "Compile a query to SQL, without running it",
	ReadOnly: true,
	Timeout:  2 * time.Minute,
	Needs:    func(in QueryCompileRequestParams) command.Sidecars { return command.Sidecars{Node: in.Lang != "sql"} },
	Check:    func(in QueryCompileRequestParams) error { return in.queryParams().check() },
	Errors:   []apperr.AnyCode{query.QueryInvalid, errcode.DataPermsUnenforceable},
	Run: func(ctx context.Context, cc command.CommandContext, raw QueryCompileRequestParams) (CompiledQuery, error) {
		in := raw.queryParams()
		if in.Lang == "sql" {
			// Already the SQL that would run.
			if err := in.checkDataSource(ctx, cc); err != nil {
				return CompiledQuery{}, err
			}
			return CompiledQuery{SQL: in.Query}, nil
		}
		aql, _, err := in.aql()
		if err != nil {
			return CompiledQuery{}, err
		}
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true}); err != nil {
			return CompiledQuery{}, err
		}
		src, err := sourceOf(cc)
		if err != nil {
			return CompiledQuery{}, err
		}
		compiled, err := compileAQL(ctx, cc, src, in.Dataset, aql, in.run())
		if err != nil {
			return CompiledQuery{}, err
		}
		return CompiledQuery{SQL: compiled.SQL, AQL: compiled.AQL, Input: in.Input.ran(compiled.Seeds), Columns: compiledColumns(compiled.Columns)}, nil
	},
})

// Validate checks a query: its diagnostics, without running it.
var Validate = command.Define(command.Def[QueryDefinition, query.QueryValidation]{
	Name:     "query.validate",
	Short:    "Validate a query without running it, and return its diagnostics",
	ReadOnly: true,
	Timeout:  2 * time.Minute,
	Needs:    func(QueryDefinition) command.Sidecars { return command.Sidecars{Node: true} },
	Check: func(in QueryDefinition) error {
		if in.Lang == "sql" {
			// The intent is a dry run on the data source; not every dialect has
			// one through canal-query, and its errors carry no positions yet.
			return command.InvalidArg("lang", "unsupported", "validating a SQL query is not supported yet")
		}
		return in.target()
	},
	Run: func(ctx context.Context, cc command.CommandContext, in QueryDefinition) (query.QueryValidation, error) {
		aql, _, err := in.aql()
		if err != nil {
			return query.QueryValidation{}, err
		}
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true}); err != nil {
			return query.QueryValidation{}, err
		}
		src, err := sourceOf(cc)
		if err != nil {
			return query.QueryValidation{}, err
		}
		return checkAQL(ctx, cc, src, in.Dataset, aql)
	},
	Valid: func(r query.QueryValidation) bool { return r.Valid },
})
