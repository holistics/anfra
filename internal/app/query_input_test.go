package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/apperr"
)

// What shapes a run is refused before anything runs, on the arg at fault: what
// applies to AQL alone, a page without its size, paging beside a limit:
// directive, a time zone that is not one. query.validate takes none of it.
func TestQueryRunChecks(t *testing.T) {
	cc := CommandContext{DataPerms: dataperm.Unrestricted()}
	for _, tc := range []struct {
		name, cmd, body string
		code            apperr.AnyCode
		field, vcode    string
	}{
		{"input on SQL", "query", `{"query":"select 1","lang":"sql","data_source":"demo","input":{}}`, apperr.ValidationFailed, "input", "unsupported"},
		{"paging SQL", "query.compile", `{"query":"select 1","lang":"sql","data_source":"demo","page_size":10}`, apperr.ValidationFailed, "page_size", "unsupported"},
		{"a time zone on SQL", "query", `{"query":"select 1","lang":"sql","data_source":"demo","timezone":"UTC"}`, apperr.ValidationFailed, "timezone", "unsupported"},
		{"a page without its size", "query", `{"query":"q","dataset":"d","page":2}`, apperr.ValidationFailed, "page_size", "required"},
		{"paging and a limit: directive", "query", `{"query":"explore { products } limit: 5","dataset":"d","page_size":10}`, apperr.ValidationFailed, "page_size", "invalid"},
		{"a time zone that is not one", "query.compile", `{"query":"q","dataset":"d","timezone":"Mars/Olympus"}`, apperr.ValidationFailed, "timezone", "invalid"},
		{"the local time zone, which names none", "query", `{"query":"q","dataset":"d","timezone":"Local"}`, apperr.ValidationFailed, "timezone", "invalid"},
		{"a page of 0, sent", "query", `{"query":"q","dataset":"d","page":0,"page_size":10}`, apperr.InvalidRequest, "page", "invalid"},
		{"a page size of 0, sent", "query", `{"query":"q","dataset":"d","page_size":0}`, apperr.InvalidRequest, "page_size", "invalid"},
		{"a negative page", "query", `{"query":"q","dataset":"d","page":-1,"page_size":10}`, apperr.InvalidRequest, "page", "invalid"},
		{"a shaped run, checked", "query", `{"query":"q","dataset":"d","page":2,"page_size":10,"timezone":"UTC","input":{"filters":[{"field":"f","operator":"is"}]}}`, nil, "", ""},
		{"a sort direction that is not one", "query", `{"query":"q","dataset":"d","input":{"sorts":[{"field":"f","direction":"up"}]}}`, apperr.InvalidRequest, "input.sorts[0].direction", "invalid"},
		{"a filter with no operator", "query", `{"query":"q","dataset":"d","input":{"filters":[{"field":"f"}]}}`, apperr.InvalidRequest, "input.filters[0].operator", "required"},
		{"input on query.validate", "query.validate", `{"query":"q","dataset":"d","input":{}}`, apperr.InvalidRequest, "input", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Invoke(context.Background(), cc, tc.cmd, []byte(tc.body))
			if tc.code == nil {
				// Not refused: it goes on to the sidecar, and there is none.
				if !errors.Is(err, errcode.SidecarUnavailable) {
					t.Errorf("got %v, want it to reach the sidecar", err)
				}
				return
			}
			var v apperr.Violations
			switch tc.code {
			case apperr.InvalidRequest:
				v, _ = apperr.DetailsOf(err, apperr.InvalidRequest)
			default:
				d, _ := apperr.DetailsOf(err, apperr.ValidationFailed)
				v = d
			}
			if len(v.Violations) != 1 || v.Violations[0].Field != tc.field || v.Violations[0].Code != tc.vcode {
				t.Errorf("got %v, %+v; want %s on %s", err, v.Violations, tc.vcode, tc.field)
			}
		})
	}
}

// A run reaches anfra-node in its names: a page size alone is the first page,
// a filter's values are a list even when the caller sent none.
func TestQueryRunAsAnfraNodeTakesIt(t *testing.T) {
	var in QueryRunInput
	if err := json.Unmarshal([]byte(`{
		"query": "q", "dataset": "d", "page_size": 20, "timezone": "Asia/Ho_Chi_Minh",
		"input": {
			"filters": [{"field": "orders.status", "operator": "is_null"}, {"field": "orders.amount", "operator": "greater_than", "values": [10], "aggregation": "sum"}],
			"conditions": [{"expr": "orders.id > 1"}],
			"sorts": [{"field": "status", "direction": "desc"}],
			"date_drills": [{"field": "orders.created_at", "grain": "month"}]
		}
	}`), &in); err != nil {
		t.Fatal(err)
	}
	want := query.Run{
		Timezone:   "Asia/Ho_Chi_Minh",
		Pagination: &sidecar.Pagination{Page: 1, PageSize: 20},
		Input: &sidecar.QueryInput{
			Filters: []sidecar.QueryInputFilter{
				{Field: "orders.status", Operator: "is_null", Values: []any{}},
				{Field: "orders.amount", Operator: "greater_than", Values: []any{float64(10)}, Aggregation: "sum"},
			},
			Conditions: []sidecar.QueryInputCondition{{Expr: "orders.id > 1"}},
			Sorts:      []sidecar.QueryInputSort{{Field: "status", Direction: "desc"}},
			DateDrills: []sidecar.QueryInputDateDrill{{Field: "orders.created_at", Grain: "month"}},
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
		{"a date drill, in the API's name", refusal("dateDrills[0].grain", "dateDrills[0].grain: Unknown grain."),
			&apperr.Violation{Field: "input.date_drills[0].grain", Code: "invalid", Message: "Unknown grain."}},
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
// it, or adhoc when it did not. A compiled query has the described ones.
func TestColumnsFor(t *testing.T) {
	described := []sidecar.ExploreColumn{
		{Name: "total", FieldName: "amount", ModelID: "orders", Label: "Total", IsMeasure: true, Aggregation: "sum"},
		{Name: "status", FieldName: "status", ModelID: "orders", Label: "Status"},
	}
	got := columnsFor([]string{"status", "total", "ratio"}, described)
	want := []Column{
		{Name: "status", FieldName: "status", ModelID: "orders", Label: "Status"},
		{Name: "total", FieldName: "amount", ModelID: "orders", Label: "Total", IsMeasure: true, Aggregation: "sum"},
		{Name: "ratio", FieldName: "ratio", Label: "ratio", Adhoc: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("columns =\n  %+v\nwant\n  %+v", got, want)
	}
	if got := columnsFor([]string{}, nil); got == nil || len(got) != 0 {
		t.Errorf("no fields: %#v, want an empty list", got)
	}
	if got := columnsFor(nil, described); len(got) != 2 || got[0].Name != "total" {
		t.Errorf("compiled: %+v, want the described columns", got)
	}
}
