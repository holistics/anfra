package query

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

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
	var in QueryRequestParams
	if err := jsonkit.Unmarshal([]byte(`{
		"query": "q", "dataset": "d",
		"input": {
			"filters": [{"field": "orders.status", "operator": "is_null", "values": []}, {"field": "orders.amount", "operator": "greater_than", "values": [10], "aggregation": "sum"}],
			"conditions": [{"expr": "orders.id > 1"}],
			"sorts": [{"field": "status", "direction": "desc"}],
			"dateDrills": [{"field": "orders.created_at", "grain": "month"}],
			"pagination": {"pageSize": 20},
			"timezone": "Asia/Ho_Chi_Minh",
			"seeds": {"referenceTime": "2026-11-12T09:41:27Z"}
		}
	}`), &in); err != nil {
		t.Fatal(err)
	}
	want := query.Run{
		Timezone:   "Asia/Ho_Chi_Minh",
		Pagination: &anfranode.Pagination{Page: 1, PageSize: 20},
		Seeds:      &anfranode.Seeds{ReferenceTime: "2026-11-12T09:41:27Z"},
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
	if got := in.normalize().run(); !reflect.DeepEqual(got, want) {
		t.Errorf("run =\n  %+v\nwant\n  %+v", got, want)
	}
	wire, _ := jsonkit.Marshal(want.Input)
	if !jsontext.Value(wire).IsValid() || !jsonHas(wire, "dateDrills") {
		t.Errorf("anfra-node's Query Input is %s, want its own names", wire)
	}

	// Nothing set: nothing sent.
	if got := (QueryRequestParams{QueryDefinition: QueryDefinition{Query: "q", Dataset: "d"}}).run(); !reflect.DeepEqual(got, query.Run{}) {
		t.Errorf("an unshaped run = %+v, want none", got)
	}
}

// The inputs beside input, deprecated, move into it, unless input sets them:
// input is what a caller replaying an answer means.
func TestDeprecatedInputsMoveIntoInput(t *testing.T) {
	q := QueryDefinition{Query: "q", Dataset: "d"}
	for _, tc := range []struct {
		name string
		in   QueryRequestParams
		want QueryInput
	}{
		{"beside input", QueryRequestParams{QueryDefinition: q, Page: 2, PageSize: 20, Timezone: "Asia/Ho_Chi_Minh"},
			QueryInput{Pagination: &Pagination{Page: 2, PageSize: 20}, Timezone: "Asia/Ho_Chi_Minh"}},
		{"both ways: input's", QueryRequestParams{QueryDefinition: q, Page: 2, PageSize: 20, Timezone: "Asia/Ho_Chi_Minh",
			Input: QueryInput{Pagination: &Pagination{Page: 3, PageSize: 50}, Timezone: "Europe/Paris"}},
			QueryInput{Pagination: &Pagination{Page: 3, PageSize: 50}, Timezone: "Europe/Paris"}},
		{"neither", QueryRequestParams{QueryDefinition: q}, QueryInput{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.normalize()
			if !reflect.DeepEqual(got.Input, tc.want) || got.Page != 0 || got.PageSize != 0 || got.Timezone != "" {
				t.Errorf("normalized = %+v, want input %+v and nothing beside it", got, tc.want)
			}
		})
	}
}

// A refusal names what the caller sent: input's field, or the deprecated one
// it came from.
func TestRunInputRefusals(t *testing.T) {
	aql := QueryDefinition{Query: "q", Dataset: "d"}
	sql := QueryDefinition{Query: "select 1", Lang: "sql", DataSource: "s"}
	for _, tc := range []struct {
		name  string
		in    QueryRequestParams
		field string
	}{
		{"a page without its size", QueryRequestParams{QueryDefinition: aql, Input: QueryInput{Pagination: &Pagination{Page: 2}}}, "input.pagination"},
		{"a page without its size, the old way", QueryRequestParams{QueryDefinition: aql, Page: 2}, "page_size"},
		{"not a time zone", QueryRequestParams{QueryDefinition: aql, Input: QueryInput{Timezone: "Mars/Olympus"}}, "input.timezone"},
		{"not a time zone, the old way", QueryRequestParams{QueryDefinition: aql, Timezone: "Mars/Olympus"}, "timezone"},
		{"not a reference time", QueryRequestParams{QueryDefinition: aql, Input: QueryInput{Seeds: &Seeds{ReferenceTime: "yesterday"}}}, "input.seeds.referenceTime"},
		{"a page for SQL", QueryRequestParams{QueryDefinition: sql, Input: QueryInput{Pagination: &Pagination{PageSize: 10}}}, "input.pagination"},
		{"seeds for SQL", QueryRequestParams{QueryDefinition: sql, Input: QueryInput{Seeds: &Seeds{}}}, "input.seeds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.normalize().check()
			v, ok := apperr.DetailsOf(err, apperr.ValidationFailed)
			if !ok || len(v.Violations) != 1 || v.Violations[0].Field != tc.field {
				t.Errorf("refused %v, want a violation on %s", err, tc.field)
			}
		})
	}
}

// Cache options anfra can't take, or that could never do what they ask, are
// refused on the field at fault; those it can, for query and query.export
// alike, are not.
func TestCacheOptionsRefusals(t *testing.T) {
	aql := QueryDefinition{Query: "q", Dataset: "d"}
	for _, tc := range []struct {
		name  string
		cache CacheOptions
		field string // empty: accepted
	}{
		{"a duration", CacheOptions{Duration: "1h"}, ""},
		{"0s", CacheOptions{Duration: "0s"}, ""},
		{"7 days", CacheOptions{Duration: "168h"}, ""},

		{"not a duration", CacheOptions{Duration: "1 hour"}, "cache.duration"},
		{"a day unit, which Go has none of", CacheOptions{Duration: "1d"}, "cache.duration"},
		{"negative", CacheOptions{Duration: "-1m"}, "cache.duration"},
		{"over 7 days", CacheOptions{Duration: "169h"}, "cache.duration"},
		{"refresh and cached only", CacheOptions{Refresh: true, CachedOnly: true}, "cache.cached_only"},
		{"cached only for 0s", CacheOptions{CachedOnly: true, Duration: "0s"}, "cache.cached_only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for op, err := range map[string]error{
				"query":        QueryRequestParams{QueryDefinition: aql, Cache: tc.cache}.normalize().check(),
				"query.export": QueryExportRequestParams{QueryDefinition: aql, Cache: tc.cache}.check(),
			} {
				if got := field(err); got != tc.field || (tc.field == "") != (err == nil) {
					t.Errorf("%s: %v, want a violation on %q", op, err, tc.field)
				}
			}
		})
	}
	if err := (QueryRequestParams{QueryDefinition: aql, Cache: CacheOptions{CachedOnly: true}}).normalize().check(); err != nil {
		t.Errorf("cached only, for query: %v", err)
	}
	if err := (QueryExportRequestParams{QueryDefinition: aql, Cache: CacheOptions{CachedOnly: true}}).check(); field(err) != "cache.cached_only" {
		t.Errorf("cached only, for an export, which doesn't read the cache yet: %v", err)
	}
	if d, set := (CacheOptions{Duration: "90m"}).duration(); !set || d != 90*time.Minute {
		t.Errorf("duration() = %v, %v", d, set)
	}
}

// query.compile checks a query as query does, and compiles it as query would.
func TestCompileTakesQuerysParams(t *testing.T) {
	in := QueryCompileRequestParams{QueryDefinition: QueryDefinition{Query: "q", Dataset: "d"}, Page: 2, PageSize: 5}
	p := in.queryParams()
	if p.Input.Pagination == nil || p.Input.Pagination.Page != 2 || p.Input.Pagination.PageSize != 5 || p.Page != 0 {
		t.Errorf("the deprecated page didn't move into input: %+v", p)
	}
	in.PageSize = 0
	if err := in.queryParams().check(); field(err) != "page_size" {
		t.Errorf("a page without its size: %v", err)
	}
}

// An answer's input is the input as the query ran: the first page made
// explicit, the default time zone, and anfra-node's seeds.
func TestInputAsItRan(t *testing.T) {
	in := QueryInput{Pagination: &Pagination{PageSize: 20}}
	got := in.filled(&anfranode.Seeds{ReferenceTime: "2026-11-12T09:41:27.123Z"})
	want := &QueryInput{Pagination: &Pagination{Page: 1, PageSize: 20}, Timezone: "Etc/UTC", Seeds: &Seeds{ReferenceTime: "2026-11-12T09:41:27.123Z"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ran = %+v, want %+v", got, want)
	}
	if in.Pagination.Page != 0 {
		t.Error("ran changed the input it was given")
	}
	// An anfra-node that returns no seeds: the ones sent, if any.
	if got := (QueryInput{Seeds: &Seeds{ReferenceTime: "2026-01-01T00:00:00Z"}}).filled(nil); got.Seeds.ReferenceTime != "2026-01-01T00:00:00Z" {
		t.Errorf("seeds = %+v, want the ones sent", got.Seeds)
	}
}

func jsonHas(raw []byte, key string) bool {
	var m map[string]any
	_ = jsonkit.Unmarshal(raw, &m)
	_, ok := m[key]
	return ok
}

// anfra-node refuses a dataset, a Query Input entry or a page as a public error,
// with the param at fault: it is the arg at fault's violation, in the API's names.
// Anything else stays as it is.
func TestRunViolation(t *testing.T) {
	refusal := func(path, msg string) error {
		data, _ := jsonkit.Marshal(map[string]string{"code": anfranode.InvalidInput, "path": path})
		return &anfranode.RPCError{Method: "aql.compile_to_sql", Code: anfranode.RPCInvalidParams, Message: msg, Data: data}
	}
	for _, tc := range []struct {
		name string
		err  error
		want *apperr.Violation
	}{
		{"a dataset that cannot be used", fmt.Errorf("compile: %w", refusal("datasetFqn", `There's no dataset "x" in the repo.`)),
			&apperr.Violation{Field: "dataset", Code: "invalid", Message: `There's no dataset "x" in the repo.`}},
		{"a filter entry", refusal("input.filters[2].operator", "Unknown operator \"nearly\"."),
			&apperr.Violation{Field: "input.filters[2].operator", Code: "invalid", Message: "Unknown operator \"nearly\"."}},
		{"a date drill", refusal("input.dateDrills[0].grain", "Unknown grain."),
			&apperr.Violation{Field: "input.dateDrills[0].grain", Code: "invalid", Message: "Unknown grain."}},
		{"the input as a whole", refusal("input", "Query Input can only be applied to a query with exactly one `explore { }` block."),
			&apperr.Violation{Field: "input", Code: "invalid", Message: "Query Input can only be applied to a query with exactly one `explore { }` block."}},
		{"paging a pivot", refusal("pagination", "Pagination for pivot queries isn't supported yet."),
			&apperr.Violation{Field: "input.pagination", Code: "unsupported", Message: "Pagination for pivot queries isn't supported yet."}},
		{"a seed", refusal("seeds.referenceTime", "Invalid referenceTime."),
			&apperr.Violation{Field: "input.seeds.referenceTime", Code: "invalid", Message: "Invalid referenceTime."}},
		{"a param the engine builds", refusal("options.weekStartDay", `Invalid weekStartDay "x".`), nil},
		{"a param that only shares a prefix", refusal("inputs", "bad"), nil},
		{"a public error with no path", &anfranode.RPCError{Code: anfranode.RPCInvalidParams, Message: "bad", Data: jsontext.Value(`{"code":"invalid_input"}`)}, nil},
		{"invalid params with no code", &anfranode.RPCError{Code: anfranode.RPCInvalidParams, Message: "bad", Data: jsontext.Value(`{"path":"datasetFqn"}`)}, nil},
		{"another RPC error", &anfranode.RPCError{Code: -32603, Message: "boom"}, nil},
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
	if aql, limit, err := (QueryDefinition{Query: "explore { products } limit: 5", Lang: "aql", Dataset: "d"}).aql(); err != nil || limit != 5 || strings.Contains(aql, "limit") {
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
