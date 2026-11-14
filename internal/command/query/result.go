package query

import (
	"slices"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// QueryResult is the `query` result answered whole: the SQL that ran, the AQL it
// was compiled from, what each column is, and one page of rows — page_size's,
// or the first MaxPageSize without one.
//
// Its pretty view (docs/designs/cli-pretty.md) is its rows, labelled by each
// column's label, then has_more and how long each step took; the SQL, the AQL
// and what each column is are for --yaml.
type QueryResult struct {
	SQL     string      `json:"sql" pretty:"-"`
	AQL     string      `json:"aql,omitempty" pretty:"-" doc:"the final AQL (with the Query Input applied). Absent for a SQL query."`
	Input   *QueryInput `json:"input,omitempty" pretty:"-" doc:"the query's input as it ran, every default filled in, its seeds included. Send it back with the same query to get the same answer again. Absent for a SQL query."`
	Columns []Column    `json:"columns" pretty:"-" doc:"one per field of result, in order"`
	Result  QueryRows   `json:"result" pretty:",inline"`
	HasMore bool        `json:"has_more" doc:"whether there may be rows after this page. To get them, ask for the next page, or export the query."`
	Timings Timings     `json:"timings" pretty:",view=timings" doc:"the query's steps, and when each started. The steps are compiling_query (for AQL only), connecting_to_dw, executing_query, fetching_query_result, and retrying_query, which is a wait before the query is tried again. connecting_to_dw, executing_query, and fetching_query_result may have child steps."`
	Cache   CacheResult `json:"cache,omitzero" pretty:"-" doc:"what the query cache did for this query. Absent when the query doesn't use the cache: it sets no cache.duration and the repo no default, or 0s."`
}

// CacheResult is what the query cache did for a query
// (docs/designs/query-cache.md#a-querys-answer).
type CacheResult struct {
	Key       string       `json:"key,omitempty" doc:"the query's key in the cache. Absent when the cache couldn't make one."`
	Hit       bool         `json:"hit" doc:"whether the rows came from the cache. false: they came from the warehouse."`
	CachedAt  time.Time    `json:"cached_at,omitzero" doc:"when the cached rows were read from the warehouse. Absent when the result wasn't cached."`
	ExpiresAt time.Time    `json:"expires_at,omitzero" doc:"when the cached result stops being used. Absent when the result wasn't cached."`
	Warning   CacheWarning `json:"warning,omitzero" doc:"why the query ran without the cache, though it asked for it"`
}

// CacheWarning is why a query that asked for the cache ran without it. It
// doesn't fail the query.
type CacheWarning struct {
	Code    string `json:"code" enum:"cache_unavailable,cache_disabled" doc:"cache_unavailable: the cache isn't working. cache_disabled: the repo, or whoever runs anfra, turned it off."`
	Message string `json:"message"`
}

type QueryRows struct {
	Fields  []string         `json:"fields" pretty:"-"`
	Records []canalquery.Row `json:"records" pretty:"Result,rowlabels=columns.label" doc:"one array per row, its values in fields' order. Read each value by its column's types. Every number is a string with all its digits, because a JSON number loses some in JavaScript. A float's NaN and infinities are \"NaN\", \"Infinity\", and \"-Infinity\". The same holds at any depth of a list, map, or struct. A database NULL is null."`
}

// CompiledQuery is the `query compile` result: the SQL the query compiles to.
type CompiledQuery struct {
	SQL     string      `json:"sql"`
	AQL     string      `json:"aql,omitempty" doc:"the final AQL (with the Query Input applied). Absent for a SQL query."`
	Input   *QueryInput `json:"input,omitempty" doc:"the query's input as it compiled, every default filled in, its seeds included. Send it back with the same query to compile it the same way again. Absent for a SQL query."`
	Columns []Column    `json:"columns,omitempty" doc:"what each column of an explore query's result will be, with its semantic type. Absent for other queries."`
}

// Column is one column of a query's answer: what it draws from the dataset, as
// anfra-node describes it, in amql's names (camelCase); and its types, which
// the core adds, in the API's (snake_case). See docs/designs/result-values.md.
type Column struct {
	Name        string `json:"name" doc:"the column's key, as in fields"`
	FieldName   string `json:"fieldName" doc:"the field it draws, or for an adhoc column, its name"`
	ModelID     string `json:"modelId,omitempty" doc:"the model of fieldName, as the dataset names it. Absent for a dataset metric or an adhoc column."`
	Label       string `json:"label"`
	Adhoc       bool   `json:"adhoc" doc:"a query-local expression, not a field the dataset defines"`
	IsMeasure   bool   `json:"isMeasure"`
	Aggregation string `json:"aggregation,omitempty" doc:"an aggregated field's aggregation, such as sum or count distinct"`

	SemanticType SemanticType         `json:"semantic_type,omitempty" doc:"the column's (database-agnostic) type in the semantic layer, such as number, date, or truefalse. Read the values by it. \"bit\" is a true or false stored as a bit, written as true and false or as \"1\" and \"0\", depending on how the database produced it. \"unknown\" is a type that has no meaning in AML, such as time or duration. Absent when no AQL typed the column, as for a SQL query or AQL that isn't an explore, so read arrow_type instead."`
	ArrowType    canalquery.ArrowType `json:"arrow_type,omitzero" doc:"the column's Arrow type, which says how its values are written. Absent before the query runs."`
	DBType       string               `json:"db_type,omitempty" doc:"the database's own name for the column's type, for debugging only. Don't base decisions on it, because it varies by database and driver. Absent before the query runs."`
}

// SemanticType is what a column's values mean: AML's types, plus bit.
type SemanticType string

const (
	SemanticText      SemanticType = "text"
	SemanticNumber    SemanticType = "number"
	SemanticDate      SemanticType = "date"
	SemanticDatetime  SemanticType = "datetime"
	SemanticTruefalse SemanticType = "truefalse"
	SemanticJSON      SemanticType = "json"
	SemanticBit       SemanticType = "bit"
	SemanticUnknown   SemanticType = "unknown"
)

// semanticTypes are what each of AMQL's types means as a semantic type: AML's
// own types (data_model.aml's DimensionType) as they are, and bit, AMQL's
// truefalse where a database has no boolean, which may be written as digits.
// Every other AMQL type is unknown until AML says what it means: time,
// duration, binary, composite, and range, which is AQL's own and never a
// column's. An AMQL type not here is unknown too; TestSemanticTypes lists
// AMQL's types, to be revisited when AMQL adds one.
//
// It is the one list of semantic types: the schema's enum is its values.
var semanticTypes = map[string]SemanticType{
	"text": SemanticText, "number": SemanticNumber, "date": SemanticDate, "datetime": SemanticDatetime,
	"truefalse": SemanticTruefalse, "json": SemanticJSON, "bit": SemanticBit, "unknown": SemanticUnknown,
}

// Schema publishes the semantic types semanticTypes gives, and no other, so the
// enum can neither miss one the core answers nor list one it never does.
func (SemanticType) Schema(huma.Registry) *huma.Schema {
	values := make([]string, 0, len(semanticTypes))
	for _, t := range semanticTypes {
		values = append(values, string(t))
	}
	slices.Sort(values)
	values = slices.Compact(values)
	enum := make([]any, len(values))
	for i, v := range values {
		enum[i] = v
	}
	return &huma.Schema{Type: huma.TypeString, Enum: enum}
}

// semanticOf is the semantic type of a column AMQL typed amql, or "" when it
// did not type it.
func semanticOf(amql string) SemanticType {
	if amql == "" {
		return ""
	}
	if s, ok := semanticTypes[amql]; ok {
		return s
	}
	return SemanticUnknown
}

// describeColumns is one column per field a query answered, in order: as
// anfra-node described it when it did, and adhoc when it did not (any query but
// an explore, or SQL), named by the field alone; with its types from the
// result's header.
func describeColumns(header canalquery.Header, described []anfranode.ExploreColumn) []Column {
	byName := make(map[string]anfranode.ExploreColumn, len(described))
	for _, c := range described {
		byName[c.Name] = c
	}
	out := make([]Column, 0, len(header.Fields))
	for i, name := range header.Fields {
		e, ok := byName[name]
		if !ok {
			e = anfranode.ExploreColumn{Name: name, FieldName: name, Label: name, Adhoc: true}
		}
		c := columnOf(e)
		if i < len(header.ArrowTypes) {
			c.ArrowType = header.ArrowTypes[i]
		}
		if i < len(header.Types) {
			c.DBType = header.Types[i]
		}
		out = append(out, c)
	}
	return out
}

// columnOf is the column anfra-node described, before it runs.
func columnOf(e anfranode.ExploreColumn) Column {
	return Column{
		Name: e.Name, FieldName: e.FieldName, ModelID: e.ModelID, Label: e.Label,
		Adhoc: e.Adhoc, IsMeasure: e.IsMeasure, Aggregation: e.Aggregation,
		SemanticType: semanticOf(e.Type),
	}
}

// compiledColumns are the columns of a compiled query, before it runs.
func compiledColumns(described []anfranode.ExploreColumn) []Column {
	if described == nil {
		return nil
	}
	out := make([]Column, len(described))
	for i, e := range described {
		out[i] = columnOf(e)
	}
	return out
}
