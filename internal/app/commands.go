package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // the time zone check, on a machine without a zone database

	"github.com/danielgtaylor/huma/v2"
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

// Commands is the registry — the single source for the CLI, the ops `anfra
// serve` serves, and Describe. Add a command here and it appears on every surface (and in help).
// Each answers one type: a command's answer never depends on its args.
var Commands = []Command{
	Define(Def[NoInput, VersionResult]{
		Name:     "version",
		Short:    "Print the anfra version",
		ReadOnly: true,
		// No Needs: pure metadata, spawns nothing.
		Run: func(context.Context, CommandContext, NoInput) (VersionResult, error) {
			return VersionResult{Version: meta.Version}, nil
		},
	}),
	Define(Def[NoInput, StatusResult]{
		Name:     "status",
		Short:    "Report whether a warm server is running and its sidecars are healthy",
		ReadOnly: true,
		// No Needs on purpose: status must NOT spawn sidecars. One-shot (no warm
		// server) then honestly reports not_running instead of starting the
		// sidecars just to declare them healthy.
		Run: func(ctx context.Context, cc CommandContext, _ NoInput) (StatusResult, error) {
			r := checkStatus(ctx, cc.Clients)
			r.Server = cc.Server
			return r, nil
		},
		Valid: func(r StatusResult) bool { return r.State == StateHealthy },
	}),
	Define(Def[QueryRunInput, QueryResult]{
		Name:     "query",
		Short:    "Run a query: its rows, and the SQL that produced them",
		ReadOnly: true,
		Timeout:  10 * time.Minute,
		Needs: func(in QueryRunInput) Sidecars {
			return Sidecars{Node: in.Lang != "sql", CanalQuery: true} // SQL is not compiled
		},
		Check:  QueryRunInput.check,
		Errors: []apperr.AnyCode{validate.QueryInvalid, errcode.QueryFailed, errcode.DataPermsUnenforceable},
		Run: func(ctx context.Context, cc CommandContext, in QueryRunInput) (QueryResult, error) {
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
				return QueryResult{SQL: r.SQL, Columns: columnsFor(r.Fields, nil),
					Result: QueryRows{Fields: r.Fields, Records: r.Records}}, nil
			}
			aql, limit, err := in.aql()
			if err != nil {
				return QueryResult{}, err
			}
			if err := requireSidecars(cc, Sidecars{Node: true, CanalQuery: true}); err != nil {
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
			return QueryResult{SQL: r.SQL, AQL: compiled.AQL, Columns: columnsFor(r.Fields, compiled.Columns),
				Result: QueryRows{Fields: r.Fields, Records: r.Records}}, nil
		},
	}),
	Define(Def[QueryRunInput, CompiledQuery]{
		Name:     "query.compile",
		Short:    "Compile a query to SQL, without running it",
		ReadOnly: true,
		Timeout:  2 * time.Minute,
		Needs:    func(in QueryRunInput) Sidecars { return Sidecars{Node: in.Lang != "sql"} },
		Check:    QueryRunInput.check,
		Errors:   []apperr.AnyCode{validate.QueryInvalid, errcode.DataPermsUnenforceable},
		Run: func(ctx context.Context, cc CommandContext, in QueryRunInput) (CompiledQuery, error) {
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
			compiled, err := compileAQL(ctx, cc, in.Dataset, aql, in.run())
			if err != nil {
				return CompiledQuery{}, err
			}
			return CompiledQuery{SQL: compiled.SQL, AQL: compiled.AQL, Columns: columnsFor(nil, compiled.Columns)}, nil
		},
	}),
	Define(Def[QueryInput, validate.QueryValidation]{
		Name:     "query.validate",
		Short:    "Validate a query: its diagnostics, without running it",
		ReadOnly: true,
		Timeout:  2 * time.Minute,
		Needs:    func(QueryInput) Sidecars { return Sidecars{Node: true} },
		Check: func(in QueryInput) error {
			if in.Lang == "sql" {
				// The intent is a dry run on the data source; not every dialect has
				// one through canal-query, and its errors carry no positions yet.
				return invalidArg("lang", "unsupported", "validating a SQL query is not supported yet; you can still run it directly")
			}
			return in.target()
		},
		Run: func(ctx context.Context, cc CommandContext, in QueryInput) (validate.QueryValidation, error) {
			aql, _, err := in.aql()
			if err != nil {
				return validate.QueryValidation{}, err
			}
			if err := requireSidecars(cc, Sidecars{Node: true}); err != nil {
				return validate.QueryValidation{}, err
			}
			return validate.AQL(ctx, cc.Clients.Node, cc.Repo, in.Dataset, aql)
		},
		Valid: func(r validate.QueryValidation) bool { return r.Valid },
	}),
	Define(Def[IngestInput, string]{
		Name:       "ingest",
		Short:      "Build the local search catalog from context sources",
		Idempotent: true, // rebuilds the same catalog
		Timeout:    30 * time.Minute,
		Needs:      func(IngestInput) Sidecars { return Sidecars{Node: true, CanalQuery: true} },
		Run: func(ctx context.Context, cc CommandContext, in IngestInput) (string, error) {
			if err := requireSidecars(cc, Sidecars{Node: true, CanalQuery: true}); err != nil {
				return "", err
			}
			return ingest.Run(ctx, cc.Clients.Node, cc.Clients.CanalQuery, cc.Repo, in.Source)
		},
	}),
	Define(Def[SearchInput, sidecar.CatalogSearchResult]{
		Name:     "search",
		Short:    "Search the local catalog",
		ReadOnly: true,
		Needs:    func(SearchInput) Sidecars { return Sidecars{Node: true, CanalQuery: true} },
		Run: func(ctx context.Context, cc CommandContext, in SearchInput) (sidecar.CatalogSearchResult, error) {
			if err := requireSidecars(cc, Sidecars{Node: true, CanalQuery: true}); err != nil {
				return nil, err
			}
			return searchcmd.Run(ctx, cc.Clients.Node, cc.Clients.CanalQuery, cc.Repo, strings.TrimSpace(strings.Join(in.Query, " ")))
		},
	}),
	Define(Def[ValidateInput, validate.RepoValidation]{
		Name:     "validate",
		Short:    "Validate the AML repo, optionally scoped to file globs",
		ReadOnly: true,
		Timeout:  5 * time.Minute,
		Needs:    func(ValidateInput) Sidecars { return Sidecars{Node: true} },
		Run: func(ctx context.Context, cc CommandContext, in ValidateInput) (validate.RepoValidation, error) {
			if err := requireSidecars(cc, Sidecars{Node: true}); err != nil {
				return validate.RepoValidation{}, err
			}
			return validate.Repo(ctx, cc.Clients.Node, cc.Repo, in.Globs)
		},
		Valid: func(r validate.RepoValidation) bool { return r.Valid },
	}),
}

// NoInput is the input of a command that takes no args.
type NoInput struct{}

// QueryInput is a query: its text, the language it is written in, and the
// target it runs against. Which target a language runs against is target's to
// say: AQL against a dataset, and SQL against a data source.
type QueryInput struct {
	Query      string `json:"query" cli:"positional,stdin" doc:"the query"`
	Lang       string `json:"lang,omitempty" short:"l" enum:"aql,sql" default:"aql" doc:"the language the query is written in"`
	Dataset    string `json:"dataset,omitempty" short:"d" doc:"the dataset an AQL query runs against"`
	DataSource string `json:"data_source,omitempty" short:"s" doc:"the data source a SQL query runs against"`
}

// TransformSchema publishes the required query (argsSchema). The target is not
// a group in the schema: which one is required depends on lang, which a group
// cannot say, so target checks it.
func (QueryInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return argsSchema[QueryInput](s)
}

// QueryRunInput is what query and query.compile take: a query, and what shapes
// its run. query.validate takes the query alone (QueryInput), since none of it
// applies to checking one.
type QueryRunInput struct {
	QueryInput
	Input    *QueryTransforms `json:"input,omitempty" doc:"the Query Input: filters, conditions, sorts and date drills applied to the AQL before it compiles"`
	Page     int              `json:"page,omitempty" minimum:"1" doc:"the 1-based page of rows to answer; needs a page size"`
	PageSize int              `json:"page_size,omitempty" minimum:"1" doc:"rows per page; alone, the first page"`
	Timezone string           `json:"timezone,omitempty" doc:"the IANA time zone relative dates and date truncation use, such as Asia/Ho_Chi_Minh"`
}

func (QueryRunInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return argsSchema[QueryRunInput](s)
}

// QueryTransforms is a query's Query Input: the structured transforms it
// carries on one run (a Data App's control filters, cross-filter conditions,
// sorts and date drills), applied by rewriting its AQL before it compiles.
// anfra-node checks the values (an operator, a grain); the engine passes them
// through.
type QueryTransforms struct {
	Filters    []QueryFilter    `json:"filters,omitempty" doc:"conditions on fields, ANDed with the query's own filters"`
	Conditions []QueryCondition `json:"conditions,omitempty" doc:"AQL conditions ANDed with the query's own filters"`
	Sorts      []QuerySort      `json:"sorts,omitempty" doc:"sorts by result column, replacing the query's own"`
	DateDrills []QueryDateDrill `json:"date_drills,omitempty" doc:"date fields redrawn at another grain, their columns' names kept"`
}

type QueryFilter struct {
	Field       string `json:"field" doc:"model.field for a dataset field, or the name of a dataset metric"`
	Operator    string `json:"operator" doc:"the operator, such as is, contains, between, last"`
	Values      []any  `json:"values,omitempty" doc:"the operator's values: strings, numbers or booleans"`
	Modifier    string `json:"modifier,omitempty" doc:"the date unit of a relative operator, such as day"`
	Aggregation string `json:"aggregation,omitempty" doc:"the condition applies to this aggregate of field, such as sum"`
}

type QueryCondition struct {
	Expr string `json:"expr" doc:"an AQL condition"`
}

type QuerySort struct {
	Field     string `json:"field" doc:"a result column's name"`
	Direction string `json:"direction" enum:"asc,desc" doc:"the direction"`
}

type QueryDateDrill struct {
	Field string `json:"field" doc:"model.field: a date field"`
	Grain string `json:"grain" doc:"the grain, such as month"`
}

// check refuses what the run cannot do: the target check, plus what applies to
// AQL alone, a page without its size, paging beside a limit: directive, and a
// time zone that is not one.
func (in QueryRunInput) check() error {
	if err := in.target(); err != nil {
		return err
	}
	if in.Lang == "sql" {
		for _, a := range []struct {
			name string
			set  bool
		}{{"input", in.Input != nil}, {"page", in.Page != 0}, {"page_size", in.PageSize != 0}, {"timezone", in.Timezone != ""}} {
			if a.set {
				return invalidArg(a.name, "unsupported", a.name+" applies to an AQL query only: a SQL query runs as it is written")
			}
		}
		return nil
	}
	if in.Page != 0 && in.PageSize == 0 {
		return invalidArg("page_size", "required", "a page needs a page size")
	}
	if in.PageSize != 0 {
		if _, limit, err := query.ExtractLimit(in.Query); err == nil && limit != query.NoLimit {
			return invalidArg("page_size", "invalid", "page the rows or end the query with a limit: directive, not both")
		}
	}
	if tz := in.Timezone; tz != "" {
		if _, err := time.LoadLocation(tz); err != nil || tz == "Local" {
			return invalidArg("timezone", "invalid", fmt.Sprintf("%q is not an IANA time zone, such as Asia/Ho_Chi_Minh", tz))
		}
	}
	return nil
}

// run is what shapes the query's run, as anfra-node takes it. A page size alone
// is the first page.
func (in QueryRunInput) run() query.Run {
	r := query.Run{Timezone: in.Timezone}
	if in.PageSize != 0 {
		r.Pagination = &sidecar.Pagination{Page: max(in.Page, 1), PageSize: in.PageSize}
	}
	if a := in.Input; a != nil {
		r.Input = &sidecar.QueryInput{}
		for _, f := range a.Filters {
			values := f.Values
			if values == nil {
				values = []any{}
			}
			r.Input.Filters = append(r.Input.Filters, sidecar.QueryInputFilter{Field: f.Field, Operator: f.Operator,
				Values: values, Modifier: f.Modifier, Aggregation: f.Aggregation})
		}
		for _, c := range a.Conditions {
			r.Input.Conditions = append(r.Input.Conditions, sidecar.QueryInputCondition(c))
		}
		for _, s := range a.Sorts {
			r.Input.Sorts = append(r.Input.Sorts, sidecar.QueryInputSort(s))
		}
		for _, d := range a.DateDrills {
			r.Input.DateDrills = append(r.Input.DateDrills, sidecar.QueryInputDateDrill(d))
		}
	}
	return r
}

// target refuses a query without the target its language runs against, or
// with the other one. AQL against a data source (an ad-hoc dataset declared
// inline) is refused as unsupported until the engine runs it.
func (in QueryInput) target() error {
	if in.Lang == "sql" {
		switch {
		case in.Dataset != "":
			return invalidArg("dataset", "invalid", "a SQL query runs against a data source, not a dataset")
		case strings.TrimSpace(in.DataSource) == "":
			return invalidArg("data_source", "required", "name the data source to run the SQL query against")
		}
		return nil
	}
	switch {
	case in.DataSource != "":
		return invalidArg("data_source", "unsupported", "an AQL query runs against a dataset")
	case strings.TrimSpace(in.Dataset) == "":
		return invalidArg("dataset", "required", "name the dataset to run the AQL query against")
	}
	return nil
}

// aql is an AQL query's text, with its `limit:` taken out.
//
// The AQL engine doesn't support `limit:`, so it is stripped here (see
// query.ExtractLimit) and applied at execution time as canal's truncate_rows.
func (in QueryInput) aql() (aql string, limit int, err error) {
	aql, limit, err = query.ExtractLimit(in.Query)
	if err != nil {
		return "", 0, apperr.EncapsulateWith(err, apperr.ValidationFailed, err.Error(),
			apperr.Violate(apperr.Violation{Field: "query", Code: "invalid", Message: err.Error()}))
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

// dataSource is a SQL query's data source. Nothing restricts SQL, so it runs
// only when the host stated the caller is unrestricted (it authorizes the
// caller on the data source itself).
func (in QueryInput) dataSource(cc CommandContext) (datasource.DataSource, error) {
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
	return apperr.NewWith(apperr.ValidationFailed, msg, apperr.Violate(apperr.Violation{Field: field, Code: code, Message: msg}))
}

// IngestInput is ingest's input.
type IngestInput struct {
	Source string `json:"source,omitempty" short:"s" doc:"optional context source key to ingest"`
}

func (IngestInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return argsSchema[IngestInput](s)
}

// SearchInput is search's input.
type SearchInput struct {
	Query []string `json:"query,omitempty" cli:"positional" doc:"search query"`
}

func (SearchInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return argsSchema[SearchInput](s)
}

// ValidateInput is validate's input.
type ValidateInput struct {
	Globs []string `json:"globs,omitempty" cli:"positional" doc:"optional file globs; report only diagnostics for matching files"`
}

func (ValidateInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return argsSchema[ValidateInput](s)
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
func compileAQL(ctx context.Context, cc CommandContext, dataset, aql string, run query.Run) (sidecar.CompileToSQLResult, error) {
	compiled, err := query.Compile(ctx, cc.Clients.Node, cc.Repo, dataset, aql, run)
	if err == nil {
		return compiled, nil
	}
	if v, ok := runViolation(err); ok {
		return sidecar.CompileToSQLResult{}, apperr.EncapsulateWith(err, apperr.ValidationFailed, v.Message, apperr.Violate(v))
	}
	if diags, verr := validate.AQL(ctx, cc.Clients.Node, cc.Repo, dataset, aql); verr == nil && !diags.Valid {
		return sidecar.CompileToSQLResult{}, apperr.EncapsulateWith(err, validate.QueryInvalid, "", diags)
	}
	return sidecar.CompileToSQLResult{}, err
}

// runViolation is anfra-node's refusal of what shapes a run, as the violation of
// the arg at fault: a Query Input entry, under input (its path in anfra-node's
// names, the API's here), or the paging of a query that cannot be paged.
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
	if rest, ok := strings.CutPrefix(path, "dateDrills"); ok {
		path = "date_drills" + rest
	}
	return apperr.Violation{Field: "input." + path, Code: "invalid", Message: msg}, true
}

// columnsFor is one Column per field, in order, as anfra-node described them.
// A field it did not describe (any query but an explore, or SQL) is adhoc. With
// no fields (a query compiled, not run), the described columns as they are.
func columnsFor(fields []string, described []sidecar.ExploreColumn) []Column {
	byName := make(map[string]sidecar.ExploreColumn, len(described))
	for _, c := range described {
		byName[c.Name] = c
	}
	if fields == nil {
		for _, c := range described {
			fields = append(fields, c.Name)
		}
	}
	out := make([]Column, 0, len(fields))
	for _, name := range fields {
		c, ok := byName[name]
		if !ok {
			out = append(out, Column{Name: name, FieldName: name, Label: name, Adhoc: true})
			continue
		}
		out = append(out, Column{Name: c.Name, FieldName: c.FieldName, ModelID: c.ModelID, Label: c.Label,
			Adhoc: c.Adhoc, IsMeasure: c.IsMeasure, Aggregation: c.Aggregation})
	}
	return out
}

// VersionResult is the `version` result.
type VersionResult struct {
	Version string `json:"version"`
}

// StatusResult is the `status` result: the warm server's state and, when it is
// running, where it is and its sidecars' health.
type StatusResult struct {
	State    State          `json:"state" enum:"healthy,degraded,not_running"`
	Server   *ServerInfo    `json:"server,omitempty"`
	Sidecars *SidecarHealth `json:"sidecars,omitempty"`
}

// State is the warm server's state.
type State string

const (
	// StateHealthy: the server is running, and every sidecar answers.
	StateHealthy State = "healthy"
	// StateDegraded: the server is running, and a sidecar does not answer.
	StateDegraded State = "degraded"
	// StateNotRunning: no warm server.
	StateNotRunning State = "not_running"
)

// SidecarHealth is each sidecar's health: "ok" or the error message.
type SidecarHealth struct {
	Node       string `json:"node"`
	CanalQuery string `json:"canal-query"`
}

// checkStatus reports the warm server's health. status spawns no sidecars, so in
// one-shot mode (no warm server) both clients are nil → not_running; under a
// warm server the clients are live and get health-checked.
func checkStatus(ctx context.Context, c Clients) StatusResult {
	if c.Node == nil && c.CanalQuery == nil {
		return StatusResult{State: StateNotRunning}
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
	state := StateHealthy
	if sc.Node != "ok" || sc.CanalQuery != "ok" {
		state = StateDegraded
	}
	return StatusResult{State: state, Sidecars: sc}
}

// QueryResult is the `query` result: the SQL that ran, the AQL it was compiled
// from, what each column is, and the rows.
type QueryResult struct {
	SQL     string    `json:"sql"`
	AQL     string    `json:"aql,omitempty" doc:"the AQL that ran: the query with its Query Input applied; absent for a SQL query"`
	Columns []Column  `json:"columns" doc:"one per field of result, in order"`
	Result  QueryRows `json:"result"`
}

// Column is one column of a query's answer: the key its values come back under,
// and the dataset field it draws, in the names AQL uses in the dataset.
type Column struct {
	Name        string `json:"name" doc:"the column's key, as in fields"`
	FieldName   string `json:"field_name" doc:"the field it draws; for an adhoc column, its name"`
	ModelID     string `json:"model_id,omitempty" doc:"the model of field_name, as the dataset names it; absent for a dataset metric or an adhoc column"`
	Label       string `json:"label"`
	Adhoc       bool   `json:"adhoc" doc:"a query-local expression, not a field the dataset defines"`
	IsMeasure   bool   `json:"is_measure"`
	Aggregation string `json:"aggregation,omitempty" doc:"an aggregated field's aggregation, such as sum or count distinct"`
}

type QueryRows struct {
	Fields  []string `json:"fields"`
	Records [][]any  `json:"records"`
}

// CompiledQuery is the `query compile` result: the SQL the query compiles to.
type CompiledQuery struct {
	SQL     string   `json:"sql"`
	AQL     string   `json:"aql,omitempty" doc:"the AQL compiled: the query with its Query Input applied; absent for a SQL query"`
	Columns []Column `json:"columns,omitempty" doc:"what each column of an explore query's answer will be; absent for other queries"`
}
