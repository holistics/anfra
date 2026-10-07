package query

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // the time zone check, on a machine without a zone database

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/errcode"
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

// QueryRunInput is what query and query.compile take: a query, and what shapes
// its run. query.validate takes the query alone (QueryInput), since none of it
// applies to checking one.
type QueryRunInput struct {
	QueryInput
	Input    *anfranode.QueryTransforms `json:"input,omitempty" doc:"the Query Input: filters, conditions, sorts and date drills applied to the AQL before it compiles"`
	Page     int                        `json:"page,omitempty" minimum:"1" doc:"the 1-based page of rows to answer; needs a page size"`
	PageSize int                        `json:"page_size,omitempty" minimum:"1" doc:"rows per page; alone, the first page"`
	Timezone string                     `json:"timezone,omitempty" doc:"the IANA time zone relative dates and date truncation use, such as Asia/Ho_Chi_Minh"`
}

func (QueryRunInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[QueryRunInput](s)
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
				return command.InvalidArg(a.name, "unsupported", a.name+" applies to an AQL query only: a SQL query runs as it is written")
			}
		}
		return nil
	}
	if in.Page != 0 && in.PageSize == 0 {
		return command.InvalidArg("page_size", "required", "a page needs a page size")
	}
	if in.PageSize != 0 {
		if _, limit, err := query.ExtractLimit(in.Query); err == nil && limit != query.NoLimit {
			return command.InvalidArg("page_size", "invalid", "page the rows or end the query with a limit: directive, not both")
		}
	}
	if tz := in.Timezone; tz != "" {
		if _, err := time.LoadLocation(tz); err != nil || tz == "Local" {
			return command.InvalidArg("timezone", "invalid", fmt.Sprintf("%q is not an IANA time zone, such as Asia/Ho_Chi_Minh", tz))
		}
	}
	return nil
}

// run is what shapes the query's run, as anfra-node takes it. A page size alone
// is the first page; the Query Input is passed through as it is.
func (in QueryRunInput) run() query.Run {
	r := query.Run{Input: in.Input, Timezone: in.Timezone}
	if in.PageSize != 0 {
		r.Pagination = &anfranode.Pagination{Page: max(in.Page, 1), PageSize: in.PageSize}
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
		return command.InvalidArg("data_source", "unsupported", "an AQL query runs against a dataset")
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

// dataSource is a SQL query's data source. Nothing restricts SQL, so it runs
// only when the host stated the caller is unrestricted (it authorizes the
// caller on the data source itself).
func (in QueryInput) dataSource(cc command.CommandContext) (datasource.DataSource, error) {
	if cc.DataPerms.Restricted() {
		return datasource.DataSource{}, apperr.New(errcode.DataPermsUnenforceable,
			"a SQL query runs only for a caller with unrestricted data permissions: no restriction can be applied to it")
	}
	ds, ok, err := query.DataSource(cc.Repo, in.DataSource)
	if err != nil {
		return datasource.DataSource{}, err
	}
	if !ok {
		return datasource.DataSource{}, command.InvalidArg("data_source", "invalid", "no data source "+in.DataSource+" in data_sources.yml")
	}
	return ds, nil
}
