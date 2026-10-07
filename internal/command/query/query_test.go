package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/apperr"
)

// A run reaches anfra-node as the caller sent it, in the names they share: a
// page size alone is the first page.
func TestQueryRunAsAnfraNodeTakesIt(t *testing.T) {
	var in QueryRunInput
	if err := json.Unmarshal([]byte(`{
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
		Pagination: &sidecar.Pagination{Page: 1, PageSize: 20},
		Input: &sidecar.QueryTransforms{
			Filters: []sidecar.QueryFilter{
				{Field: "orders.status", Operator: "is_null", Values: []any{}},
				{Field: "orders.amount", Operator: "greater_than", Values: []any{float64(10)}, Aggregation: "sum"},
			},
			Conditions: []sidecar.QueryCondition{{Expr: "orders.id > 1"}},
			Sorts:      []sidecar.QuerySort{{Field: "status", Direction: "desc"}},
			DateDrills: []sidecar.QueryDateDrill{{Field: "orders.created_at", Grain: "month"}},
		},
	}
	if got := in.run(); !reflect.DeepEqual(got, want) {
		t.Errorf("run =\n  %+v\nwant\n  %+v", got, want)
	}
	wire, _ := json.Marshal(want.Input)
	if !json.Valid(wire) || !jsonHas(wire, "dateDrills") {
		t.Errorf("anfra-node's Query Input is %s, want its own names", wire)
	}

	// Nothing set: nothing sent.
	if got := (QueryRunInput{QueryInput: QueryInput{Query: "q", Dataset: "d"}}).run(); !reflect.DeepEqual(got, query.Run{}) {
		t.Errorf("an unshaped run = %+v, want none", got)
	}
}

func jsonHas(raw []byte, key string) bool {
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	_, ok := m[key]
	return ok
}

// anfra-node refuses a Query Input entry or a page as invalid params, with the
// path of what is wrong: it is the arg at fault's violation, in the API's names.
// Anything else stays as it is.
func TestRunViolation(t *testing.T) {
	refusal := func(path, msg string) error {
		data, _ := json.Marshal(map[string]string{"path": path})
		return &sidecar.RPCError{Method: "aql.compile_to_sql", Code: sidecar.RPCInvalidParams, Message: msg, Data: data}
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
		{"invalid params with no path", &sidecar.RPCError{Code: sidecar.RPCInvalidParams, Message: `Dataset "x" not found`}, nil},
		{"another RPC error", &sidecar.RPCError{Code: -32603, Message: "boom", Data: json.RawMessage(`{"path":"filters[0]"}`)}, nil},
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
// it, or adhoc when it did not.
func TestDescribeFields(t *testing.T) {
	described := []sidecar.ExploreColumn{
		{Name: "total", FieldName: "amount", ModelID: "orders", Label: "Total", IsMeasure: true, Aggregation: "sum"},
		{Name: "status", FieldName: "status", ModelID: "orders", Label: "Status"},
	}
	got := describeFields([]string{"status", "total", "ratio"}, described)
	want := []sidecar.ExploreColumn{
		{Name: "status", FieldName: "status", ModelID: "orders", Label: "Status"},
		{Name: "total", FieldName: "amount", ModelID: "orders", Label: "Total", IsMeasure: true, Aggregation: "sum"},
		{Name: "ratio", FieldName: "ratio", Label: "ratio", Adhoc: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("columns =\n  %+v\nwant\n  %+v", got, want)
	}
	if got := describeFields([]string{}, nil); got == nil || len(got) != 0 {
		t.Errorf("no fields: %#v, want an empty list", got)
	}
}

// A `limit:` directive is taken out of the AQL, and becomes the run's row limit.
func TestAQLTakesTheLimitOut(t *testing.T) {
	if aql, limit, err := (QueryInput{Query: "explore { products } limit: 5", Lang: "aql", Dataset: "d"}).aql(); err != nil || limit != 5 || strings.Contains(aql, "limit") {
		t.Errorf("AQL against a dataset: %q, %d, %v", aql, limit, err)
	}
}

// canal-query's failures are query_failed, except a client-scope one: a request
// the engine built wrong is the engine's bug, and stays unclassified.
func TestFailed(t *testing.T) {
	for scope, want := range map[string]bool{"User": true, "Server": true, "Client": false} {
		err := failed(fmt.Errorf("execute: %w", &sidecar.CanalQueryError{Message: "boom", Scope: scope}))
		if got := errors.Is(err, errcode.QueryFailed); got != want {
			t.Errorf("canal scope %s: query_failed = %v, want %v", scope, got, want)
		}
	}
}
