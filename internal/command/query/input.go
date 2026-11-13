package query

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // the time zone check, on a machine without a zone database

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// QueryDefinition is a query: its text, the language it is written in, and the
// target it runs against. Which target a language runs against is target's to
// say: AQL against a dataset, and SQL against a data source.
type QueryDefinition struct {
	Query      string `json:"query" cli:"positional,stdin" doc:"the query"`
	Lang       string `json:"lang,omitempty" short:"l" enum:"aql,sql" default:"aql" doc:"the language the query is written in"`
	Dataset    string `json:"dataset,omitempty" short:"d" doc:"the dataset an AQL query runs against"`
	DataSource string `json:"data_source,omitempty" short:"s" doc:"the data source a SQL query runs against"`
}

// TransformSchema publishes the required query (command.ArgsSchema). The target is not
// a group in the schema: which one is required depends on lang, which a group
// cannot say, so target checks it.
func (QueryDefinition) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[QueryDefinition](s)
}

// QueryInput is a query's input: everything a caller chooses about running a
// query, other than its text. Kept in one object, in camelCase, so a caller
// runs the query again (another page, an export, a replay) by sending the
// input its answer returned (docs/designs/query-input.md).
type QueryInput struct {
	anfranode.QueryTransforms
	Pagination *Pagination `json:"pagination,omitempty" doc:"which rows to return: pageSize rows, from the 1-based page. Without it, every row."`
	Timezone   string      `json:"timezone,omitempty" doc:"the IANA time zone that relative dates and date truncation use, such as Asia/Ho_Chi_Minh. UTC by default."`
	Seeds      *Seeds      `json:"seeds,omitempty" doc:"values the server generates for each request when they're unset. Generated seeds typically change with every request, and the response returns them. Set seeds to generate the same SQL again, for example to replay a query as of a given time."`
}

// Seeds are a query's seeds: values generated for each request when unset.
type Seeds struct {
	ReferenceTime string `json:"referenceTime,omitempty" format:"date-time" doc:"the time the query runs as of. Relative dates, such as last 7 days, are counted from this time's day."`
}

// Pagination is one page of a query's rows.
type Pagination struct {
	Page     int `json:"page,omitempty" minimum:"1" doc:"the 1-based page of rows to return, 1 by default"`
	PageSize int `json:"pageSize" minimum:"1" maximum:"10000" doc:"how many rows a page has"`
}

// IsZero reports an input that changes nothing about a query.
func (in QueryInput) IsZero() bool {
	return in.QueryTransforms.IsZero() && in.Pagination == nil && in.Timezone == "" && in.Seeds == nil
}

// pageSize is the page's size, 0 without one.
func (in QueryInput) pageSize() int {
	if in.Pagination == nil {
		return 0
	}
	return in.Pagination.PageSize
}

// defaultTimezone is the time zone anfra-node compiles in when none is given.
const defaultTimezone = "Etc/UTC"

// ran is the input as the query ran: every default filled in, and the seeds
// it compiled with, when anfra-node returned them.
func (in QueryInput) ran(seeds *anfranode.Seeds) *QueryInput {
	out := in
	if in.Pagination != nil {
		p := *in.Pagination
		p.Page = max(p.Page, 1)
		out.Pagination = &p
	}
	if out.Timezone == "" {
		out.Timezone = defaultTimezone
	}
	if seeds != nil {
		out.Seeds = &Seeds{ReferenceTime: seeds.ReferenceTime}
	}
	return &out
}

// QueryRequestParams is what query takes: a query, its input, and how the
// query cache answers it. query.compile takes the same but the cache
// (QueryCompileRequestParams), and query.validate the query alone
// (QueryDefinition), since none of the input applies to checking one.
type QueryRequestParams struct {
	QueryDefinition
	Input QueryInput `json:"input,omitempty" doc:"the query's input, for AQL only: the Query Input (filters, conditions, sorts and date drills applied to the AQL before it compiles), a page of rows, the time zone, and the seeds. The answer returns it as the query ran: send it back with the same query to get the same answer again."`

	// These three moved into input, and are kept until they're removed. A
	// request that sets one both ways takes input's.
	Page     int    `json:"page,omitempty" minimum:"1" deprecated:"true" doc:"deprecated: use input.pagination.page. Ignored when input.pagination is set."`
	PageSize int    `json:"page_size,omitempty" minimum:"1" maximum:"10000" deprecated:"true" doc:"deprecated: use input.pagination.pageSize. Ignored when input.pagination is set."`
	Timezone string `json:"timezone,omitempty" deprecated:"true" doc:"deprecated: use input.timezone. Ignored when input.timezone is set."`

	Cache CacheOptions `json:"cache,omitempty" doc:"how to use the query cache for this request. A query uses it when it sets cache.duration, or when the repo sets a default (cache.query.default.duration). Otherwise it runs on the warehouse, uncached."`

	// The names the page and the time zone were set by, for a refusal to name
	// what the caller sent: a deprecated input's, or "" for input's.
	paginationArg, timezoneArg string
}

// QueryCompileRequestParams is what query.compile takes: a query and its
// input, as query takes them, without the cache, since compiling runs
// nothing.
type QueryCompileRequestParams struct {
	QueryDefinition
	Input QueryInput `json:"input,omitempty" doc:"the query's input, for AQL only: the Query Input (filters, conditions, sorts and date drills applied to the AQL before it compiles), a page of rows, the time zone, and the seeds. The answer returns it as the query compiled: send it back with the same query to get the same SQL again."`

	// These three moved into input, and are kept until they're removed. A
	// request that sets one both ways takes input's.
	Page     int    `json:"page,omitempty" minimum:"1" deprecated:"true" doc:"deprecated: use input.pagination.page. Ignored when input.pagination is set."`
	PageSize int    `json:"page_size,omitempty" minimum:"1" maximum:"10000" deprecated:"true" doc:"deprecated: use input.pagination.pageSize. Ignored when input.pagination is set."`
	Timezone string `json:"timezone,omitempty" deprecated:"true" doc:"deprecated: use input.timezone. Ignored when input.timezone is set."`
}

func (QueryCompileRequestParams) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[QueryCompileRequestParams](s)
}

// queryParams is the query as query takes it, so that query.compile checks and
// compiles it as query does.
func (in QueryCompileRequestParams) queryParams() QueryRequestParams {
	return QueryRequestParams{QueryDefinition: in.QueryDefinition, Input: in.Input, Page: in.Page, PageSize: in.PageSize, Timezone: in.Timezone}.normalize()
}

// CacheOptions are how the query cache answers a request
// (docs/designs/query-cache.md#a-querys-input).
type CacheOptions struct {
	Duration   string `json:"duration,omitempty" doc:"how old a cached result the query accepts, and how long the result it caches is kept, such as 10m or 1h, at most 168h (7 days). 0s turns the cache off for this query. The repo's cache.query.default.duration by default."`
	Refresh    bool   `json:"refresh,omitempty" doc:"true runs the query on the warehouse whatever is cached, and caches its result."`
	ReadOnly   bool   `json:"read_only,omitempty" doc:"true answers from a cached result if there is one, and otherwise runs the query without caching its result."`
	CachedOnly bool   `json:"cached_only,omitempty" doc:"true answers from a cached result, or fails with cache_miss without running the query."`
}

// duration is cache.duration, and whether the request sets it. check refused
// one that isn't a duration.
func (c CacheOptions) duration() (time.Duration, bool) {
	if c.Duration == "" {
		return 0, false
	}
	d, _ := time.ParseDuration(c.Duration)
	return d, true
}

// check refuses cache options that aren't a duration anfra takes, or that
// could never do what they ask.
func (c CacheOptions) check() error {
	if c.Duration != "" {
		d, err := time.ParseDuration(c.Duration)
		switch {
		case err != nil:
			return command.InvalidArg("cache.duration", "invalid", fmt.Sprintf("%q isn't a duration, such as 30s, 10m or 2h.", c.Duration))
		case d < 0:
			return command.InvalidArg("cache.duration", "invalid", "cache.duration can't be negative. Use 0s to run the query without the cache.")
		case d > querycache.MaxDuration:
			return command.InvalidArg("cache.duration", "invalid", "cache.duration can be at most 168h (7 days).")
		}
	}
	if c.CachedOnly && !c.Refresh {
		// Until a hit can be read: canal-query can't read an entry from
		// canal-lake yet (docs/designs/query-cache.md#implementation-plan).
		return command.InvalidArg("cache.cached_only", "unsupported", "cache.cached_only isn't supported yet, because anfra can't answer a query from the cache yet.")
	}
	if c.CachedOnly && c.Refresh {
		return command.InvalidArg("cache.cached_only", "invalid", "cache.cached_only can't be used with cache.refresh, because refresh never answers from a cached result.")
	}
	if d, set := c.duration(); c.CachedOnly && set && d == 0 {
		return command.InvalidArg("cache.cached_only", "invalid", "cache.cached_only needs a cache.duration above 0s, because no cached result is younger than 0s.")
	}
	return nil
}

func (in QueryRequestParams) paginationName() string {
	return cmp.Or(in.paginationArg, "input.pagination")
}
func (in QueryRequestParams) timezoneName() string { return cmp.Or(in.timezoneArg, "input.timezone") }

func (QueryRequestParams) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[QueryRequestParams](s)
}

// normalize moves the deprecated inputs into input, unless input sets them.
func (in QueryRequestParams) normalize() QueryRequestParams {
	if in.Input.Pagination == nil && (in.Page != 0 || in.PageSize != 0) {
		in.Input.Pagination = &Pagination{Page: in.Page, PageSize: in.PageSize}
		in.paginationArg = "page_size"
	}
	if in.Input.Timezone == "" && in.Timezone != "" {
		in.Input.Timezone = in.Timezone
		in.timezoneArg = "timezone"
	}
	in.Page, in.PageSize, in.Timezone = 0, 0, ""
	return in
}

// check refuses what the run cannot do: the target check, plus what applies to
// AQL alone, a page without its size, paging beside a limit: directive, and a
// time zone that is not one.
func (in QueryRequestParams) check() error {
	if err := in.target(); err != nil {
		return err
	}
	if err := in.Cache.check(); err != nil {
		return err
	}
	// Each said by what it is, not its field's name, which each surface writes
	// its own way (input.pagination, page_size, --page-size).
	if in.Lang == "sql" {
		for _, a := range []struct {
			name, what string
			set        bool
		}{
			{"input", "a Query Input", !in.Input.QueryTransforms.IsZero()},
			{in.paginationName(), "a page", in.Input.Pagination != nil},
			{in.timezoneName(), "a time zone", in.Input.Timezone != ""},
			{"input.seeds", "seeds", in.Input.Seeds != nil},
		} {
			if a.set {
				return command.InvalidArg(a.name, "unsupported", "only AQL queries can have "+a.what)
			}
		}
		return nil
	}
	if p := in.Input.Pagination; p != nil {
		if p.PageSize == 0 {
			return command.InvalidArg(in.paginationName(), "required", "a page needs a page size")
		}
		if _, limit, err := query.ExtractLimit(in.Query); err == nil && limit != query.NoLimit {
			return command.InvalidArg(in.paginationName(), "invalid", "a query can't have both a page size and a `limit:` directive")
		}
	}
	if tz := in.Input.Timezone; tz != "" {
		if _, err := time.LoadLocation(tz); err != nil || tz == "Local" {
			return command.InvalidArg(in.timezoneName(), "invalid", fmt.Sprintf("%q is not an IANA time zone, such as Asia/Ho_Chi_Minh", tz))
		}
	}
	if s := in.Input.Seeds; s != nil && s.ReferenceTime != "" {
		if _, err := time.Parse(time.RFC3339Nano, s.ReferenceTime); err != nil {
			return command.InvalidArg("input.seeds.referenceTime", "invalid", fmt.Sprintf("%q is not an ISO 8601 date-time, such as 2026-11-12T09:41:27Z", s.ReferenceTime))
		}
	}
	return nil
}

// run is what shapes the query's run, as anfra-node takes it. A page without
// its number is the first; the Query Input is passed through as it is.
func (in QueryRequestParams) run() query.Run {
	r := query.Run{Timezone: in.Input.Timezone}
	if s := in.Input.Seeds; s != nil {
		r.Seeds = &anfranode.Seeds{ReferenceTime: s.ReferenceTime}
	}
	if !in.Input.QueryTransforms.IsZero() {
		r.Input = &in.Input.QueryTransforms
	}
	if p := in.Input.Pagination; p != nil {
		r.Pagination = &anfranode.Pagination{Page: max(p.Page, 1), PageSize: p.PageSize}
	}
	return r
}

// target refuses a query without the target its language runs against, or
// with the other one. AQL against a data source (an ad-hoc dataset declared
// inline) is refused as unsupported until the engine runs it.
func (in QueryDefinition) target() error {
	if in.Lang == "sql" {
		switch {
		case in.Dataset != "":
			return command.InvalidArg("dataset", "invalid", "a SQL query runs against a data source, not a dataset")
		case strings.TrimSpace(in.DataSource) == "":
			return command.InvalidArg("data_source", "required", "name the data source to run the SQL query against")
		}
		return nil
	}
	switch {
	case in.DataSource != "":
		return command.InvalidArg("data_source", "unsupported", "an AQL query runs against a dataset, not a data source")
	case strings.TrimSpace(in.Dataset) == "":
		return command.InvalidArg("dataset", "required", "name the dataset to run the AQL query against")
	}
	return nil
}

// aql is an AQL query's text, with its `limit:` taken out.
//
// The AQL engine doesn't support `limit:`, so it is stripped here (see
// query.ExtractLimit) and applied at execution time as canal's truncate_rows.
func (in QueryDefinition) aql() (aql string, limit int, err error) {
	aql, limit, err = query.ExtractLimit(in.Query)
	if err != nil {
		return "", 0, apperr.EncapsulateWith(err, apperr.ValidationFailed, err.Error(),
			apperr.Violate(apperr.Violation{Field: "query", Code: "invalid", Message: err.Error()}))
	}
	return aql, limit, nil
}

// ungoverned is a SQL query, as written, on the data source it names,
// connection included: only that one is asked for, and only for an ungoverned
// caller.
func (in QueryDefinition) ungoverned(ctx context.Context, cc command.CommandContext) (query.UngovernedSQL, error) {
	sources, err := in.sqlSources(cc)
	if err != nil {
		return query.UngovernedSQL{}, err
	}
	sql, err := query.ConnectUngoverned(ctx, cc.DataPerms, sources, in.DataSource, in.Query)
	if errors.Is(err, datasource.ErrUnknown) {
		return query.UngovernedSQL{}, in.unknownDataSource()
	}
	return sql, err
}

// checkDataSource checks a SQL query names a data source of the repo, without
// asking for its connection: compiling SQL is returning it as it is.
func (in QueryDefinition) checkDataSource(ctx context.Context, cc command.CommandContext) error {
	sources, err := in.sqlSources(cc)
	if err != nil {
		return err
	}
	dialects, err := sources.Dialects(ctx)
	if err != nil {
		return fmt.Errorf("data source dialects: %w", err)
	}
	if !slices.ContainsFunc(dialects, func(d datasource.Dialect) bool { return d.Name == in.DataSource }) {
		return in.unknownDataSource()
	}
	return nil
}

// sqlSources is the repo's data sources, for a SQL query. Nothing governs SQL,
// so it is answered only when the host stated the caller is ungoverned (it
// authorizes the caller on the data source itself).
func (in QueryDefinition) sqlSources(cc command.CommandContext) (datasource.DataSources, error) {
	if err := query.CheckUngoverned(cc.DataPerms); err != nil {
		return nil, err
	}
	return cc.ResolveDataSources()
}

func (in QueryDefinition) unknownDataSource() error {
	return command.InvalidArg("data_source", "invalid", fmt.Sprintf("data source %q is not defined in this repo", in.DataSource))
}
