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
var Query = command.Define(command.Def[QueryRunInput, QueryRun]{
	Name:  "query",
	Short: "Run a query: its rows, and the SQL that produced them",
	Long: `Answers one page of rows: page_size's, or without it the first 10000, with has_more saying whether there may be more.
Ask the query for what you need (aggregate, filter, a limit: directive) rather than paging through rows; for every row of a large result, export it.
Over HTTP, a request that accepts application/x-ndjson is answered as the query runs: its phase, its SQL, its columns, then its rows in batches — every row without page_size, up to the server's maximum.`,
	ReadOnly: true,
	Timeout:  10 * time.Minute,
	Needs: func(in QueryRunInput) command.Sidecars {
		return command.Sidecars{Node: in.Lang != "sql", CanalQuery: true} // SQL is not compiled
	},
	Check:  QueryRunInput.check,
	Errors: []apperr.AnyCode{query.QueryInvalid, errcode.QueryFailed, errcode.DataPermsUnenforceable, query.ResultTooLarge, query.PageTooLarge},
	Events: queryEvents,
	Run:    newQueryRun,
})

// Compile compiles a query to SQL, without running it.
var Compile = command.Define(command.Def[QueryRunInput, CompiledQuery]{
	Name:     "query.compile",
	Short:    "Compile a query to SQL, without running it",
	ReadOnly: true,
	Timeout:  2 * time.Minute,
	Needs:    func(in QueryRunInput) command.Sidecars { return command.Sidecars{Node: in.Lang != "sql"} },
	Check:    QueryRunInput.check,
	Errors:   []apperr.AnyCode{query.QueryInvalid, errcode.DataPermsUnenforceable},
	Run: func(ctx context.Context, cc command.CommandContext, in QueryRunInput) (CompiledQuery, error) {
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
		return CompiledQuery{SQL: compiled.SQL, AQL: compiled.AQL, Columns: compiledColumns(compiled.Columns)}, nil
	},
})

// Validate checks a query: its diagnostics, without running it.
var Validate = command.Define(command.Def[QueryInput, query.QueryValidation]{
	Name:     "query.validate",
	Short:    "Validate a query: its diagnostics, without running it",
	ReadOnly: true,
	Timeout:  2 * time.Minute,
	Needs:    func(QueryInput) command.Sidecars { return command.Sidecars{Node: true} },
	Check: func(in QueryInput) error {
		if in.Lang == "sql" {
			// The intent is a dry run on the data source; not every dialect has
			// one through canal-query, and its errors carry no positions yet.
			return command.InvalidArg("lang", "unsupported", "validating a SQL query is not supported yet; you can still run it directly")
		}
		return in.target()
	},
	Run: func(ctx context.Context, cc command.CommandContext, in QueryInput) (query.QueryValidation, error) {
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
