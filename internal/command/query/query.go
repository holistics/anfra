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

// Query runs a query: its rows, and the SQL that produced them.
var Query = command.Define(command.Def[QueryRunInput, QueryResult]{
	Name:     "query",
	Short:    "Run a query: its rows, and the SQL that produced them",
	ReadOnly: true,
	Timeout:  10 * time.Minute,
	Needs: func(in QueryRunInput) command.Sidecars {
		return command.Sidecars{Node: in.Lang != "sql", CanalQuery: true} // SQL is not compiled
	},
	Check:  QueryRunInput.check,
	Errors: []apperr.AnyCode{query.QueryInvalid, errcode.QueryFailed, errcode.DataPermsUnenforceable},
	Run: func(ctx context.Context, cc command.CommandContext, in QueryRunInput) (QueryResult, error) {
		if in.Lang == "sql" {
			ds, err := in.dataSource(cc)
			if err != nil {
				return QueryResult{}, err
			}
			if err := command.RequireSidecars(cc, command.Sidecars{CanalQuery: true}); err != nil {
				return QueryResult{}, err
			}
			r, err := query.ExecuteSQL(ctx, cc.Clients.CanalQuery, ds, in.Query)
			if err != nil {
				return QueryResult{}, failed(err)
			}
			return QueryResult{SQL: r.SQL, Columns: describeFields(r.Fields, nil),
				Result: QueryRows{Fields: r.Fields, Records: r.Records}}, nil
		}
		aql, limit, err := in.aql()
		if err != nil {
			return QueryResult{}, err
		}
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true, CanalQuery: true}); err != nil {
			return QueryResult{}, err
		}
		compiled, err := compileAQL(ctx, cc, in.Dataset, aql, in.run())
		if err != nil {
			return QueryResult{}, err
		}
		r, err := query.Execute(ctx, cc.Clients.CanalQuery, cc.Repo, compiled, limit)
		if err != nil {
			return QueryResult{}, failed(err)
		}
		return QueryResult{SQL: r.SQL, AQL: compiled.AQL, Columns: describeFields(r.Fields, compiled.Columns),
			Result: QueryRows{Fields: r.Fields, Records: r.Records}}, nil
	},
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
			if _, err := in.dataSource(cc); err != nil {
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
		compiled, err := compileAQL(ctx, cc, in.Dataset, aql, in.run())
		if err != nil {
			return CompiledQuery{}, err
		}
		return CompiledQuery{SQL: compiled.SQL, AQL: compiled.AQL, Columns: compiled.Columns}, nil
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
		return query.Check(ctx, cc.Clients.Node, cc.Repo, in.Dataset, aql)
	},
	Valid: func(r query.QueryValidation) bool { return r.Valid },
})
