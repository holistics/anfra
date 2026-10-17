package query

import (
	"slices"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// QueryResult is the `query` result answered whole: the SQL that ran, the AQL it
// was compiled from, what each column is, and one page of rows — page_size's,
// or the first MaxPageSize without one.
type QueryResult struct {
	SQL     string    `json:"sql"`
	AQL     string    `json:"aql,omitempty" doc:"the AQL that ran: the query with its Query Input applied; absent for a SQL query"`
	Columns []Column  `json:"columns" doc:"one per field of result, in order"`
	Result  QueryRows `json:"result"`
	HasMore bool      `json:"has_more" doc:"there may be rows after this page: ask for the next page, or stream the query (without page_size) for every row"`
	Timings Timings   `json:"timings" doc:"the query's steps: compiling_query, for AQL; canal-query's own (connecting_to_dw, executing_query, fetching_query_result, and their children, as <parent>.<child>); retrying_query, the wait before canal-query attempts the query again"`
}

type QueryRows struct {
	Fields  []string         `json:"fields"`
	Records []canalquery.Row `json:"records" doc:"one array per row, its values in fields' order, as its column's types say: every number a string, with every digit (a JSON number loses some in JavaScript), a float's NaN and infinities as \"NaN\", \"Infinity\" and \"-Infinity\", and so at any depth of a list, map or struct; null as null"`
}

// CompiledQuery is the `query compile` result: the SQL the query compiles to.
type CompiledQuery struct {
	SQL     string   `json:"sql"`
	AQL     string   `json:"aql,omitempty" doc:"the AQL compiled: the query with its Query Input applied; absent for a SQL query"`
	Columns []Column `json:"columns,omitempty" doc:"what each column of an explore query's answer will be, and its semantic type; absent for other queries"`
}

// Column is one column of a query's answer: what it draws from the dataset, as
// anfra-node describes it, in amql's names (camelCase); and its types, which
// the core adds, in the API's (snake_case). See docs/designs/result-values.md.
type Column struct {
	Name        string `json:"name" doc:"the column's key, as in fields"`
	FieldName   string `json:"fieldName" doc:"the field it draws; for an adhoc column, its name"`
	ModelID     string `json:"modelId,omitempty" doc:"the model of fieldName, as the dataset names it; absent for a dataset metric or an adhoc column"`
	Label       string `json:"label"`
	Adhoc       bool   `json:"adhoc" doc:"a query-local expression, not a field the dataset defines"`
	IsMeasure   bool   `json:"isMeasure"`
	Aggregation string `json:"aggregation,omitempty" doc:"an aggregated field's aggregation, such as sum or count distinct"`

	SemanticType SemanticType         `json:"semantic_type,omitempty" doc:"what the column's values mean, in AML's types, plus bit: a true or false stored as a bit, written as true and false or as \"1\" and \"0\" by how the database produced it. Decide by it. Absent when no AQL types the column (a SQL query, or AQL that is not an explore): see arrow_type. Unknown when its AQL type has no meaning in AML (time, duration, ...)"`
	ArrowType    canalquery.ArrowType `json:"arrow_type,omitzero" doc:"the Arrow type canal-query read the values as, by which they are written; absent before the query runs"`
	DBType       string               `json:"db_type,omitempty" doc:"the database's own name of the column's type: for debugging, never to decide on, since it varies by database and driver; absent before the query runs"`
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
