package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/command/query"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/apperr"
)

// An op's input is checked against its schema — huma's reading of the In's
// tags, with ArgsSchema's groups and required strings — before the command
// runs: everything a correct client could have got right, at once, as
// invalid_request.
func TestInputSchema(t *testing.T) {
	cc := command.CommandContext{DataPerms: dataperm.Unrestricted()}
	for _, tc := range []struct {
		name, body string
		bad        apperr.Violations
	}{
		{"every refusal at once", `{"lang":"cobol","bogus":1}`, apperr.Violate(
			apperr.Violation{Field: "lang", Code: "invalid", Message: `Must be one of: aql, sql; got "cobol".`},
			apperr.Violation{Field: "query", Code: "required", Message: "Required."},
			apperr.Violation{Field: "bogus", Code: "unknown", Message: "Not a field of this input. Field names are snake_case and case-sensitive."},
		)},
		{"a value of the wrong type", `{"query":42,"dataset":"d"}`, apperr.Violate(
			apperr.Violation{Field: "query", Code: "invalid", Message: "Must be a string; got a number."})},
		{"a blank required string", `{"query":"","dataset":"d"}`, apperr.Violate(
			apperr.Violation{Field: "query", Code: "too_short", Message: "Must be at least 1 character long."})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Invoke(context.Background(), cc, "query.compile", []byte(tc.body))
			v, _ := apperr.DetailsOf(err, apperr.InvalidRequest)
			if !reflect.DeepEqual(v, tc.bad) {
				t.Errorf("got %v, violations =\n  %+v\nwant\n  %+v", err, v, tc.bad)
			}
		})
	}

	// A default the input leaves unset is applied before the command runs: lang
	// is aql, so AQL against a data source is refused as such.
	_, err := Invoke(context.Background(), cc, "query.compile", []byte(`{"query":"q","data_source":"demo"}`))
	if d, _ := apperr.DetailsOf(err, apperr.ValidationFailed); len(d.Violations) != 1 || d.Violations[0].Field != "data_source" || d.Violations[0].Code != "unsupported" {
		t.Errorf("the default language was not applied: %v", err)
	}
}

// AQL runs against a dataset and SQL against a data source; the rest is refused
// before anything runs, on the arg that makes it so: the target a query's
// language needs, missing, or the other one, set.
func TestQueryInputs(t *testing.T) {
	fixture, err := filepath.Abs("testdata/repo")
	if err != nil {
		t.Fatal(err)
	}
	cc := command.CommandContext{Repo: repo.Resolve(fixture), DataPerms: dataperm.Unrestricted()}
	for _, tc := range []struct {
		name  string
		cmd   string
		in    query.QueryInput
		cc    command.CommandContext
		field string
		code  string
	}{
		{"AQL with no dataset", "query", query.QueryInput{Query: "q", Lang: "aql"}, cc, "dataset", "required"},
		{"AQL against a data source, for now", "query", query.QueryInput{Query: "q", Lang: "aql", DataSource: "demo"}, cc, "data_source", "unsupported"},
		{"AQL against both", "query.compile", query.QueryInput{Query: "q", Lang: "aql", Dataset: "d", DataSource: "demo"}, cc, "data_source", "unsupported"},
		{"SQL with no data source", "query", query.QueryInput{Query: "q", Lang: "sql"}, cc, "data_source", "required"},
		{"SQL with a blank data source", "query", query.QueryInput{Query: "q", Lang: "sql", DataSource: " "}, cc, "data_source", "required"},
		{"SQL against a dataset", "query", query.QueryInput{Query: "q", Lang: "sql", Dataset: "ecommerce"}, cc, "dataset", "invalid"},
		{"SQL against both", "query.compile", query.QueryInput{Query: "q", Lang: "sql", Dataset: "ecommerce", DataSource: "demo"}, cc, "dataset", "invalid"},
		{"SQL against an unknown data source", "query.compile", query.QueryInput{Query: "q", Lang: "sql", DataSource: "nosuch"}, cc, "data_source", "invalid"},
		{"validating SQL, for now", "query.validate", query.QueryInput{Query: "q", Lang: "sql", DataSource: "demo"}, cc, "lang", "unsupported"},
		{"validating SQL, before its target", "query.validate", query.QueryInput{Query: "q", Lang: "sql"}, cc, "lang", "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(t, tc.cmd, tc.cc, tc.in)
			d, _ := apperr.DetailsOf(err, apperr.ValidationFailed)
			v := d.Violations
			if len(v) != 1 || v[0].Field != tc.field || v[0].Code != tc.code {
				t.Errorf("got %v, %+v; want %s on %s", err, v, tc.code, tc.field)
			}
		})
	}

	// Nothing restricts raw SQL, so a restricted caller cannot run it at all.
	restricted := cc
	restricted.DataPerms = dataperm.Restricted(nil)
	for _, cmd := range []string{"query", "query.compile"} {
		if _, err := run(t, cmd, restricted, query.QueryInput{Query: "select 1", Lang: "sql", DataSource: "demo"}); !errors.Is(err, errcode.DataPermsUnenforceable) {
			t.Errorf("%s: a restricted caller's SQL got %v, want data_perms_unenforceable", cmd, err)
		}
	}

	// Compiling SQL returns it as it is, and needs no sidecar.
	res, err := run(t, "query.compile", cc, query.QueryInput{Query: "select 1", Lang: "sql", DataSource: "demo"})
	if err != nil || !reflect.DeepEqual(res.Data, query.CompiledQuery{SQL: "select 1"}) {
		t.Errorf("compiling SQL: %+v, %v", res, err)
	}
	c, _ := Find("query")
	if got := c.Needs([]byte(`{"query":"select 1","lang":"sql","data_source":"demo"}`)); got != (command.Sidecars{CanalQuery: true}) {
		t.Errorf("SQL needs %+v, want canal-query alone", got)
	}
	if got := c.Needs([]byte(`{"query":"q","dataset":"d"}`)); got != (command.Sidecars{Node: true, CanalQuery: true}) {
		t.Errorf("AQL, by default, needs %+v, want both", got)
	}
	// One the command refuses needs none: the one-shot CLI spawns nothing to refuse it.
	if got := c.Needs([]byte(`{"query":"select 1","lang":"sql"}`)); got != (command.Sidecars{}) {
		t.Errorf("SQL with no data source needs %+v, want none", got)
	}

}

// run dispatches cmd with in, as a host would.
func run(t *testing.T, cmd string, cc command.CommandContext, in query.QueryInput) (Response, error) {
	t.Helper()
	args := map[string]any{"query": in.Query, "lang": in.Lang}
	if in.Dataset != "" {
		args["dataset"] = in.Dataset
	}
	if in.DataSource != "" {
		args["data_source"] = in.DataSource
	}
	return Dispatch(context.Background(), cc, Request{Command: cmd, Args: args})
}
