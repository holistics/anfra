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
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// QueryInput is a query: its text, the language it is written in, and the
// target it runs against. Which target a language runs against is target's to
// say: AQL against a dataset, and SQL against a data source.
type QueryInput struct {
	Query      string `json:"query" cli:"positional,stdin" doc:"the query"`
	Lang       string `json:"lang,omitempty" short:"l" enum:"aql,sql" default:"aql" doc:"the language the query is written in"`
	Dataset    string `json:"dataset,omitempty" short:"d" doc:"the dataset an AQL query runs against"`
	DataSource string `json:"data_source,omitempty" short:"s" doc:"the data source a SQL query runs against"`
}

// TransformSchema publishes the required query (command.ArgsSchema). The target is not
// a group in the schema: which one is required depends on lang, which a group
// cannot say, so target checks it.
func (QueryInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[QueryInput](s)
}

// RunInput is a query's input: everything a caller chooses about running a
// query, other than its text. Kept in one object, in camelCase, so a caller
// runs the query again (another page, an export, a replay) by sending the
// input its answer returned (docs/designs/query-input.md).
type RunInput struct {
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
func (in RunInput) IsZero() bool {
	return in.QueryTransforms.IsZero() && in.Pagination == nil && in.Timezone == "" && in.Seeds == nil
}

// pageSize is the page's size, 0 without one.
func (in RunInput) pageSize() int {
	if in.Pagination == nil {
		return 0
	}
	return in.Pagination.PageSize
}

// defaultTimezone is the time zone anfra-node compiles in when none is given.
const defaultTimezone = "Etc/UTC"

// ran is the input as the query ran: every default filled in, and the seeds
// it compiled with, when anfra-node returned them.
func (in RunInput) ran(seeds *anfranode.Seeds) *RunInput {
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

// QueryRunInput is what query and query.compile take: a query, and its input.
// query.validate takes the query alone (QueryInput), since none of the input
// applies to checking one.
type QueryRunInput struct {
	QueryInput
	Input RunInput `json:"input,omitempty" doc:"the query's input, for AQL only: the Query Input (filters, conditions, sorts and date drills applied to the AQL before it compiles), a page of rows, the time zone, and the seeds. The answer returns it as the query ran: send it back with the same query to get the same answer again."`

	// These three moved into input, and are kept until they're removed. A
	// request that sets one both ways takes input's.
	Page     int    `json:"page,omitempty" minimum:"1" deprecated:"true" doc:"deprecated: use input.pagination.page. Ignored when input.pagination is set."`
	PageSize int    `json:"page_size,omitempty" minimum:"1" maximum:"10000" deprecated:"true" doc:"deprecated: use input.pagination.pageSize. Ignored when input.pagination is set."`
	Timezone string `json:"timezone,omitempty" deprecated:"true" doc:"deprecated: use input.timezone. Ignored when input.timezone is set."`

	// The names the page and the time zone were set by, for a refusal to name
	// what the caller sent: a deprecated input's, or "" for input's.
	paginationArg, timezoneArg string
}

func (in QueryRunInput) paginationName() string { return cmp.Or(in.paginationArg, "input.pagination") }
func (in QueryRunInput) timezoneName() string   { return cmp.Or(in.timezoneArg, "input.timezone") }

func (QueryRunInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[QueryRunInput](s)
}

// normalize moves the deprecated inputs into input, unless input sets them.
func (in QueryRunInput) normalize() QueryRunInput {
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
func (in QueryRunInput) check() error {
	if err := in.target(); err != nil {
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
func (in QueryRunInput) run() query.Run {
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
func (in QueryInput) target() error {
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
func (in QueryInput) aql() (aql string, limit int, err error) {
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
func (in QueryInput) ungoverned(ctx context.Context, cc command.CommandContext) (query.UngovernedSQL, error) {
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
func (in QueryInput) checkDataSource(ctx context.Context, cc command.CommandContext) error {
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
func (in QueryInput) sqlSources(cc command.CommandContext) (datasource.DataSources, error) {
	if err := query.CheckUngoverned(cc.DataPerms); err != nil {
		return nil, err
	}
	return cc.ResolveDataSources()
}

func (in QueryInput) unknownDataSource() error {
	return command.InvalidArg("data_source", "invalid", fmt.Sprintf("data source %q is not defined in this repo", in.DataSource))
}
