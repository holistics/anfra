package app

import (
	"context"
	"errors"
	"strings"

	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/ingest"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/query"
	searchcmd "github.com/holistics/anfra/internal/search"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/validate"
	"github.com/holistics/anfra/shared/apperr"
)

// Commands is the registry — the single source for the CLI, /call and
// Describe. Add a command here and it appears on every surface (and in help).
// Each answers one type: a command's answer never depends on its args.
var Commands = []Command{
	Define(Def[NoInput, VersionResult]{
		Name:  "version",
		Short: "Print the anfra version",
		// No Needs: pure metadata, spawns nothing.
		Run: func(context.Context, CommandContext, NoInput) (VersionResult, error) {
			return VersionResult{Version: meta.Version}, nil
		},
	}),
	Define(Def[NoInput, StatusResult]{
		Name:  "status",
		Short: "Report whether a warm server is running and its sidecars are healthy",
		// No Needs on purpose: status must NOT spawn sidecars. One-shot (no warm
		// server) then honestly reports "not running" instead of starting the
		// sidecars just to declare them healthy.
		Run: func(ctx context.Context, cc CommandContext, _ NoInput) (StatusResult, error) {
			return checkStatus(ctx, cc.Clients), nil
		},
		Valid: func(r StatusResult) bool {
			return r.Server == "running" && r.Sidecars != nil && r.Sidecars.Node == "ok" && r.Sidecars.CanalQuery == "ok"
		},
	}),
	Define(Def[QueryInput, QueryResult]{
		Name:  "query",
		Short: "Run a query: its rows, and the SQL that produced them",
		Needs: func(in QueryInput) Sidecars {
			return Sidecars{Node: in.Lang != "sql", CanalQuery: true} // SQL is not compiled
		},
		Errors: []apperr.AnyCode{validate.QueryInvalid, errcode.QueryFailed, errcode.DataPermsUnenforceable},
		Run: func(ctx context.Context, cc CommandContext, in QueryInput) (QueryResult, error) {
			if in.Lang == "sql" {
				ds, err := in.dataSource(cc)
				if err != nil {
					return QueryResult{}, err
				}
				if err := requireSidecars(cc, Sidecars{CanalQuery: true}); err != nil {
					return QueryResult{}, err
				}
				r, err := query.ExecuteSQL(ctx, cc.Clients.CanalQuery, ds, in.Query)
				if err != nil {
					return QueryResult{}, failed(err)
				}
				return QueryResult{SQL: r.SQL, Result: QueryRows{Fields: r.Fields, Records: r.Records}}, nil
			}
			aql, limit, err := in.aql()
			if err != nil {
				return QueryResult{}, err
			}
			if err := requireSidecars(cc, Sidecars{Node: true, CanalQuery: true}); err != nil {
				return QueryResult{}, err
			}
			compiled, err := compileAQL(ctx, cc, in.Dataset, aql)
			if err != nil {
				return QueryResult{}, err
			}
			r, err := query.Execute(ctx, cc.Clients.CanalQuery, cc.Repo, compiled, limit)
			if err != nil {
				return QueryResult{}, failed(err)
			}
			return QueryResult{SQL: r.SQL, Result: QueryRows{Fields: r.Fields, Records: r.Records}}, nil
		},
	}),
	Define(Def[QueryInput, CompiledQuery]{
		Name:   "query.compile",
		Short:  "Compile a query to SQL, without running it",
		Needs:  func(in QueryInput) Sidecars { return Sidecars{Node: in.Lang != "sql"} },
		Errors: []apperr.AnyCode{validate.QueryInvalid, errcode.DataPermsUnenforceable},
		Run: func(ctx context.Context, cc CommandContext, in QueryInput) (CompiledQuery, error) {
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
			if err := requireSidecars(cc, Sidecars{Node: true}); err != nil {
				return CompiledQuery{}, err
			}
			compiled, err := compileAQL(ctx, cc, in.Dataset, aql)
			if err != nil {
				return CompiledQuery{}, err
			}
			return CompiledQuery{SQL: compiled.SQL}, nil
		},
	}),
	Define(Def[QueryInput, validate.QueryValidation]{
		Name:  "query.validate",
		Short: "Validate a query: its diagnostics, without running it",
		Needs: func(QueryInput) Sidecars { return Sidecars{Node: true} },
		Run: func(ctx context.Context, cc CommandContext, in QueryInput) (validate.QueryValidation, error) {
			if in.Lang == "sql" {
				// The intent is a dry run on the data source; not every dialect has
				// one through canal-query, and its errors carry no positions yet.
				return validate.QueryValidation{}, invalidArg("lang", "unsupported", "validating a SQL query is not supported yet")
			}
			aql, _, err := in.aql()
			if err != nil {
				return validate.QueryValidation{}, err
			}
			if err := requireSidecars(cc, Sidecars{Node: true}); err != nil {
				return validate.QueryValidation{}, err
			}
			return validate.AQL(ctx, cc.Clients.Node, cc.Repo, in.Dataset, aql)
		},
		Valid: func(r validate.QueryValidation) bool { return !r.Invalid() },
	}),
	Define(Def[IngestInput, string]{
		Name:  "ingest",
		Short: "Build the local search catalog from context sources",
		Needs: func(IngestInput) Sidecars { return Sidecars{Node: true, CanalQuery: true} },
		Run: func(ctx context.Context, cc CommandContext, in IngestInput) (string, error) {
			return ingest.Run(ctx, cc.Clients.Node, cc.Clients.CanalQuery, cc.Repo, in.Source)
		},
	}),
	Define(Def[SearchInput, sidecar.CatalogSearchResult]{
		Name:  "search",
		Short: "Search the local catalog",
		Needs: func(SearchInput) Sidecars { return Sidecars{Node: true, CanalQuery: true} },
		Run: func(ctx context.Context, cc CommandContext, in SearchInput) (sidecar.CatalogSearchResult, error) {
			return searchcmd.Run(ctx, cc.Clients.Node, cc.Clients.CanalQuery, cc.Repo, strings.TrimSpace(strings.Join(in.Query, " ")))
		},
	}),
	Define(Def[ValidateInput, validate.RepoValidation]{
		Name:  "validate",
		Short: "Validate the AML repo, optionally scoped to file globs",
		Needs: func(ValidateInput) Sidecars { return Sidecars{Node: true} },
		Run: func(ctx context.Context, cc CommandContext, in ValidateInput) (validate.RepoValidation, error) {
			return validate.Repo(ctx, cc.Clients.Node, cc.Repo, in.Globs)
		},
		Valid: func(r validate.RepoValidation) bool { return !r.Invalid() },
	}),
}

// NoInput is the input of a command that takes no args.
type NoInput struct{}

// QueryInput is a query: its text, the language it is written in, and the
// target it runs against. Which combinations run is aql's and dataSource's to
// say: AQL against a dataset, and SQL against a data source.
type QueryInput struct {
	Query      string `arg:"query" cli:"positional,stdin" required:"true" usage:"the query; read from stdin when omitted"`
	Lang       string `arg:"lang" short:"l" enum:"aql,sql" default:"aql" usage:"the language the query is written in"`
	Dataset    string `arg:"dataset" short:"d" group:"target" usage:"the dataset to query (AQL)"`
	DataSource string `arg:"data_source" alias:"ds" group:"target" usage:"the data source to query (SQL)"`
}

// aql is an AQL query's text, with its `limit:` taken out. It runs against a
// dataset; against a data source (an ad-hoc dataset declared inline) it is
// refused as unsupported until the engine runs it.
//
// The AQL engine doesn't support `limit:`, so it is stripped here (see
// query.ExtractLimit) and applied at execution time as canal's truncate_rows.
func (in QueryInput) aql() (aql string, limit int, err error) {
	if in.DataSource != "" {
		return "", 0, invalidArg("data_source", "unsupported", "an AQL query against a data source is not supported yet")
	}
	aql, limit, err = query.ExtractLimit(in.Query)
	if err != nil {
		return "", 0, apperr.EncapsulateWith(err, errcode.InvalidArgs, err.Error(),
			apperr.Violations{{Field: "query", Code: "invalid", Message: err.Error()}})
	}
	return aql, limit, nil
}

// requireSidecars refuses a command whose sidecars are not connected, before it
// calls one: a host that dialled none, or the one-shot CLI before it spawned.
func requireSidecars(cc CommandContext, need Sidecars) error {
	switch {
	case need.Node && cc.Clients.Node == nil:
		return apperr.New(errcode.SidecarUnavailable, "this command requires the anfra-node sidecar")
	case need.CanalQuery && cc.Clients.CanalQuery == nil:
		return apperr.New(errcode.SidecarUnavailable, "this command requires the canal-query sidecar")
	}
	return nil
}

// dataSource is a SQL query's data source. SQL runs against one, never a
// dataset; nothing restricts it, so it runs only when the host stated the
// caller is unrestricted (it authorizes the caller on the data source itself).
func (in QueryInput) dataSource(cc CommandContext) (datasource.DataSource, error) {
	if in.Dataset != "" {
		return datasource.DataSource{}, invalidArg("dataset", "invalid", "a SQL query runs against a data source, not a dataset")
	}
	if cc.DataPerms.Restricted() {
		return datasource.DataSource{}, apperr.New(errcode.DataPermsUnenforceable,
			"a SQL query runs only for a caller with unrestricted data permissions: no restriction can be applied to it")
	}
	ds, ok, err := query.DataSource(cc.Repo, in.DataSource)
	if err != nil {
		return datasource.DataSource{}, err
	}
	if !ok {
		return datasource.DataSource{}, invalidArg("data_source", "invalid", "no data source "+in.DataSource+" in data_sources.yml")
	}
	return ds, nil
}

func invalidArg(field, code, msg string) error {
	return apperr.NewWith(errcode.InvalidArgs, msg, apperr.Violations{{Field: field, Code: code, Message: msg}})
}

// IngestInput is ingest's input.
type IngestInput struct {
	Source string `arg:"source" short:"s" usage:"optional context source key to ingest"`
}

// SearchInput is search's input.
type SearchInput struct {
	Query []string `arg:"query" cli:"positional" usage:"search query"`
}

// ValidateInput is validate's input.
type ValidateInput struct {
	Globs []string `arg:"globs" cli:"positional" usage:"optional file globs; report only diagnostics for matching files"`
}

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

// compileAQL compiles a query, failing with query_invalid and its diagnostics
// when it does not compile. A failure that is not the query's (an unknown
// dataset, a missing data source) is not diagnostic-shaped, and is returned as
// it is.
func compileAQL(ctx context.Context, cc CommandContext, dataset, aql string) (sidecar.CompileToSQLResult, error) {
	compiled, err := query.Compile(ctx, cc.Clients.Node, cc.Repo, dataset, aql)
	if err == nil {
		return compiled, nil
	}
	if diags, verr := validate.AQL(ctx, cc.Clients.Node, cc.Repo, dataset, aql); verr == nil && diags.Invalid() {
		return sidecar.CompileToSQLResult{}, apperr.EncapsulateWith(err, validate.QueryInvalid, "", diags)
	}
	return sidecar.CompileToSQLResult{}, err
}

// VersionResult is the `version` result.
type VersionResult struct {
	Version string `json:"version"`
}

// StatusResult is the `status` result: whether the warm server is running and,
// when it is, its sidecars' health nested under `sidecars`.
type StatusResult struct {
	Server   string         `json:"server"` // "running" | "not running"
	Sidecars *SidecarHealth `json:"sidecars,omitempty"`
}

// SidecarHealth is each sidecar's health: "ok" or the error message.
type SidecarHealth struct {
	Node       string `json:"node"`
	CanalQuery string `json:"canal-query"`
}

// checkStatus reports the warm server's health. status spawns no sidecars, so in
// one-shot mode (no warm server) both clients are nil → "not running"; under a
// warm server the clients are live and get health-checked.
func checkStatus(ctx context.Context, c Clients) StatusResult {
	if c.Node == nil && c.CanalQuery == nil {
		return StatusResult{Server: "not running"}
	}
	sc := &SidecarHealth{Node: "ok", CanalQuery: "ok"}
	if c.Node == nil {
		sc.Node = "unavailable"
	} else if _, err := c.Node.Ping(ctx); err != nil {
		sc.Node = err.Error()
	}
	if c.CanalQuery == nil {
		sc.CanalQuery = "unavailable"
	} else if err := c.CanalQuery.Health(ctx); err != nil {
		sc.CanalQuery = err.Error()
	}
	return StatusResult{Server: "running", Sidecars: sc}
}

// QueryResult is the `query` result: the SQL that ran, and its rows.
type QueryResult struct {
	SQL    string    `json:"sql"`
	Result QueryRows `json:"result"`
}

type QueryRows struct {
	Fields  []string `json:"fields"`
	Records [][]any  `json:"records"`
}

// CompiledQuery is the `query compile` result: the SQL the query compiles to.
type CompiledQuery struct {
	SQL string `json:"sql"`
}
