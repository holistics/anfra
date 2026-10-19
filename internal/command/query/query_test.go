package query

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// A run reaches anfra-node as the caller sent it, in the names they share: a
// page size alone is the first page.
func TestQueryRunAsAnfraNodeTakesIt(t *testing.T) {
	var in QueryRunInput
	if err := jsonkit.Unmarshal([]byte(`{
		"query": "q", "dataset": "d", "page_size": 20, "timezone": "Asia/Ho_Chi_Minh",
		"input": {
			"filters": [{"field": "orders.status", "operator": "is_null", "values": []}, {"field": "orders.amount", "operator": "greater_than", "values": [10], "aggregation": "sum"}],
			"conditions": [{"expr": "orders.id > 1"}],
			"sorts": [{"field": "status", "direction": "desc"}],
			"dateDrills": [{"field": "orders.created_at", "grain": "month"}]
		}
	}`), &in); err != nil {
		t.Fatal(err)
	}
	want := query.Run{
		Timezone:   "Asia/Ho_Chi_Minh",
		Pagination: &anfranode.Pagination{Page: 1, PageSize: 20},
		Input: &anfranode.QueryTransforms{
			Filters: []anfranode.QueryFilter{
				{Field: "orders.status", Operator: "is_null", Values: []any{}},
				{Field: "orders.amount", Operator: "greater_than", Values: []any{float64(10)}, Aggregation: "sum"},
			},
			Conditions: []anfranode.QueryCondition{{Expr: "orders.id > 1"}},
			Sorts:      []anfranode.QuerySort{{Field: "status", Direction: "desc"}},
			DateDrills: []anfranode.QueryDateDrill{{Field: "orders.created_at", Grain: "month"}},
		},
	}
	if got := in.run(); !reflect.DeepEqual(got, want) {
		t.Errorf("run =\n  %+v\nwant\n  %+v", got, want)
	}
	wire, _ := jsonkit.Marshal(want.Input)
	if !jsontext.Value(wire).IsValid() || !jsonHas(wire, "dateDrills") {
		t.Errorf("anfra-node's Query Input is %s, want its own names", wire)
	}

	// Nothing set: nothing sent.
	if got := (QueryRunInput{QueryInput: QueryInput{Query: "q", Dataset: "d"}}).run(); !reflect.DeepEqual(got, query.Run{}) {
		t.Errorf("an unshaped run = %+v, want none", got)
	}
}

func jsonHas(raw []byte, key string) bool {
	var m map[string]any
	_ = jsonkit.Unmarshal(raw, &m)
	_, ok := m[key]
	return ok
}

// anfra-node refuses a Query Input entry or a page as invalid params, with the
// path of what is wrong: it is the arg at fault's violation, in the API's names.
// Anything else stays as it is.
func TestRunViolation(t *testing.T) {
	refusal := func(path, msg string) error {
		data, _ := jsonkit.Marshal(map[string]string{"path": path})
		return &anfranode.RPCError{Method: "aql.compile_to_sql", Code: anfranode.RPCInvalidParams, Message: msg, Data: data}
	}
	for _, tc := range []struct {
		name string
		err  error
		want *apperr.Violation
	}{
		{"a filter entry", refusal("filters[2].operator", "filters[2].operator: Unknown operator \"nearly\"."),
			&apperr.Violation{Field: "input.filters[2].operator", Code: "invalid", Message: "Unknown operator \"nearly\"."}},
		{"a date drill", refusal("dateDrills[0].grain", "dateDrills[0].grain: Unknown grain."),
			&apperr.Violation{Field: "input.dateDrills[0].grain", Code: "invalid", Message: "Unknown grain."}},
		{"the input as a whole", refusal("", "Query Input can only be applied to a query with exactly one `explore { }` block."),
			&apperr.Violation{Field: "input", Code: "invalid", Message: "Query Input can only be applied to a query with exactly one `explore { }` block."}},
		{"paging a pivot", refusal("pagination", "Pagination for pivot queries isn't supported yet."),
			&apperr.Violation{Field: "page_size", Code: "unsupported", Message: "Pagination for pivot queries isn't supported yet."}},
		{"invalid params with no path", &anfranode.RPCError{Code: anfranode.RPCInvalidParams, Message: `Dataset "x" not found`}, nil},
		{"another RPC error", &anfranode.RPCError{Code: -32603, Message: "boom", Data: jsontext.Value(`{"path":"filters[0]"}`)}, nil},
		{"not an RPC error", errors.New("boom"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := runViolation(tc.err)
			switch {
			case tc.want == nil && ok:
				t.Errorf("got %+v, want it left as it is", got)
			case tc.want != nil && (!ok || got != *tc.want):
				t.Errorf("got %+v (%v), want %+v", got, ok, *tc.want)
			}
		})
	}
}

// A query's answer has one column per field, in order: as anfra-node described
// it, or adhoc when it did not; with the header's types. Its semantic type is
// what AMQL typed it, never one guessed from its Arrow or database type.
func TestDescribeColumns(t *testing.T) {
	described := []anfranode.ExploreColumn{
		{Name: "total", FieldName: "amount", ModelID: "orders", Label: "Total", IsMeasure: true, Aggregation: "sum", Type: "number"},
		{Name: "status", FieldName: "status", ModelID: "orders", Label: "Status", Type: "text"},
		{Name: "month", FieldName: "created_at", ModelID: "orders", Label: "Month"}, // an anfra-node before 0.4.0
	}
	decimal := canalquery.ArrowType(`{"type":"decimal","precision":38,"scale":2,"bit_width":128,"label":"decimal(38, 2)"}`)
	utf8 := canalquery.ArrowType(`{"type":"utf8","label":"utf8"}`)
	date := canalquery.ArrowType(`{"type":"date","unit":"DAY","label":"date32"}`)
	float := canalquery.ArrowType(`{"type":"floating_point","precision":"DOUBLE","label":"float64"}`)
	header := canalquery.Header{
		Fields:     []string{"status", "total", "month", "ratio"},
		Types:      []string{"text", "numeric", "date", "float8"},
		ArrowTypes: []canalquery.ArrowType{utf8, decimal, date, float},
	}
	got := describeColumns(header, described)
	want := []Column{
		{Name: "status", FieldName: "status", ModelID: "orders", Label: "Status", SemanticType: "text", ArrowType: utf8, DBType: "text"},
		{Name: "total", FieldName: "amount", ModelID: "orders", Label: "Total", IsMeasure: true, Aggregation: "sum",
			SemanticType: "number", ArrowType: decimal, DBType: "numeric"},
		{Name: "month", FieldName: "created_at", ModelID: "orders", Label: "Month", ArrowType: date, DBType: "date"},
		{Name: "ratio", FieldName: "ratio", Label: "ratio", Adhoc: true, ArrowType: float, DBType: "float8"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("columns =\n  %+v\nwant\n  %+v", got, want)
	}

	// AMQL's type stands where the Arrow type says otherwise: a number canal-query
	// could only read as text is a number.
	got = describeColumns(canalquery.Header{Fields: []string{"total"}, ArrowTypes: []canalquery.ArrowType{utf8}}, described)
	if got[0].SemanticType != "number" {
		t.Errorf("semantic type = %q, want AMQL's number", got[0].SemanticType)
	}
	if got := describeColumns(canalquery.Header{}, nil); got == nil || len(got) != 0 {
		t.Errorf("no fields: %#v, want an empty list", got)
	}
}

// Each of AMQL's types (packages/amql/src/types.ts, SqlType, as of anfra-node
// 0.4.0) has its semantic type: AML's own types and bit as they are, the rest
// unknown until AML says what they mean. A type AMQL adds is unknown: list it
// here, and decide what it means.
func TestSemanticTypes(t *testing.T) {
	for amql, want := range map[string]SemanticType{
		"text": "text", "number": "number", "date": "date", "datetime": "datetime", "truefalse": "truefalse",
		"json": "json", "bit": "bit", "unknown": "unknown",
		"time": "unknown", "duration": "unknown", "binary": "unknown", "composite": "unknown", "range": "unknown",
		"": "",
	} {
		if got := semanticOf(amql); got != want {
			t.Errorf("%q: %q, want %q", amql, got, want)
		}
	}
	if got := semanticOf("some_new_type"); got != SemanticUnknown {
		t.Errorf("a type AMQL adds: %q, want unknown", got)
	}
}

// The schema lists the semantic types the core answers, each once.
func TestSemanticTypeSchema(t *testing.T) {
	got := SemanticType("").Schema(nil).Enum
	want := []any{"bit", "date", "datetime", "json", "number", "text", "truefalse", "unknown"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("enum = %v, want %v", got, want)
	}
}

// A compiled query's columns have AMQL's semantic type, and no type from a run.
func TestCompiledColumns(t *testing.T) {
	got := compiledColumns([]anfranode.ExploreColumn{{Name: "n", Type: "number"}, {Name: "r", Type: "range"}, {Name: "x"}})
	if got[0].SemanticType != "number" || got[1].SemanticType != "unknown" || got[2].SemanticType != "" ||
		got[0].ArrowType != nil || got[0].DBType != "" {
		t.Errorf("columns = %+v", got)
	}
	if compiledColumns(nil) != nil {
		t.Error("no columns described: want none")
	}
}

// A `limit:` directive is taken out of the AQL, and becomes the run's row limit.
func TestAQLTakesTheLimitOut(t *testing.T) {
	if aql, limit, err := (QueryInput{Query: "explore { products } limit: 5", Lang: "aql", Dataset: "d"}).aql(); err != nil || limit != 5 || strings.Contains(aql, "limit") {
		t.Errorf("AQL against a dataset: %q, %d, %v", aql, limit, err)
	}
}

// canal-query's user-scope failures are query_failed: the data source's, the
// user's to fix. Its server- and client-scope ones are the server's — canal
// failing, or a request the engine built wrong — and stay unclassified.
func TestAsQueryFailed(t *testing.T) {
	for scope, want := range map[string]bool{"User": true, "Server": false, "Client": false} {
		err := asQueryFailed(fmt.Errorf("execute: %w", &canalquery.Error{Message: "boom", Scope: scope}))
		if got := errors.Is(err, errcode.QueryFailed); got != want {
			t.Errorf("canal scope %s: query_failed = %v, want %v", scope, got, want)
		}
	}
}
