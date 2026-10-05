package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/apperr"
)

// decode reads /call's JSON and the CLI's values alike, folds aliases, applies
// defaults, and refuses everything the In does not take, all at once.
func TestDecode(t *testing.T) {
	specs, err := parseArgs(reflect.TypeFor[QueryInput]())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		want QueryInput
		bad  apperr.Violations
	}{
		{name: "the default language", args: map[string]any{"query": "q", "dataset": "d"},
			want: QueryInput{Query: "q", Lang: "aql", Dataset: "d"}},
		{name: "an alias folds into its name", args: map[string]any{"query": "q", "ds": "warehouse", "lang": "sql"},
			want: QueryInput{Query: "q", Lang: "sql", DataSource: "warehouse"}},
		{name: "help is every command's", args: map[string]any{"query": "q", "dataset": "d", "help": true},
			want: QueryInput{Query: "q", Lang: "aql", Dataset: "d"}},
		{name: "every refusal at once", args: map[string]any{"lang": "cobol", "bogus": 1},
			bad: apperr.Violations{
				{Field: "bogus", Code: "unknown", Message: "Not an arg of query."},
				{Field: "query", Code: "required", Message: "query is required"},
				{Field: "lang", Code: "invalid", Message: "lang must be one of: aql, sql"},
				{Field: "dataset", Code: "required", Message: "one of dataset, data_source is required"},
			}},
		{name: "two of a group", args: map[string]any{"query": "q", "dataset": "d", "data_source": "w"},
			bad: apperr.Violations{{Field: "data_source", Code: "invalid", Message: "only one of dataset, data_source may be set"}}},
		{name: "a value of the wrong type", args: map[string]any{"query": 42, "dataset": "d"},
			bad: apperr.Violations{{Field: "query", Code: "invalid", Message: "query must be a string"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decode[QueryInput]("query", specs, tc.args)
			if tc.bad == nil {
				if err != nil || got != tc.want {
					t.Errorf("got %+v, %v; want %+v", got, err, tc.want)
				}
				return
			}
			v, _ := apperr.DetailsOf(err, errcode.InvalidArgs)
			if !reflect.DeepEqual(v, tc.bad) {
				t.Errorf("violations =\n  %+v\nwant\n  %+v", v, tc.bad)
			}
		})
	}

	list, _ := parseArgs(reflect.TypeFor[SearchInput]())
	if got, err := decode[SearchInput]("search", list, map[string]any{"query": []any{"a", "b"}}); err != nil || !reflect.DeepEqual(got.Query, []string{"a", "b"}) {
		t.Errorf("a JSON list decodes to %v, %v", got.Query, err)
	}
}

// A malformed In is refused when the command is defined, not when it runs.
func TestParseArgsRefusesMistakes(t *testing.T) {
	for name, in := range map[string]any{
		"an unsupported type": struct {
			N int `arg:"n" usage:"x"`
		}{},
		"no usage": struct {
			S string `arg:"s"`
		}{},
		"a required bool": struct {
			B bool `arg:"b" required:"true" usage:"x"`
		}{},
		"a default outside its enum": struct {
			S string `arg:"s" enum:"a,b" default:"c" usage:"x"`
		}{},
		"a group of one": struct {
			S string `arg:"s" group:"g" usage:"x"`
		}{},
		"a repeated name": struct {
			A string `arg:"a" usage:"x"`
			B string `arg:"b" alias:"a" usage:"x"`
		}{},
		"a long shorthand": struct {
			S string `arg:"s" short:"ss" usage:"x"`
		}{},
		"two positionals": struct {
			A string `arg:"a" cli:"positional" usage:"x"`
			B string `arg:"b" cli:"positional" usage:"x"`
		}{},
		"help, which every command has": struct {
			H bool `arg:"help" usage:"x"`
		}{},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseArgs(reflect.TypeOf(in)); err == nil {
				t.Errorf("%s was accepted", name)
			}
		})
	}
}

// AQL runs against a dataset and SQL against a data source; the rest is refused
// before anything runs, on the arg that makes it so.
func TestQueryInputs(t *testing.T) {
	fixture, err := filepath.Abs("testdata/repo")
	if err != nil {
		t.Fatal(err)
	}
	cc := CommandContext{Repo: repo.Resolve(fixture), DataPerms: dataperm.Unrestricted()}
	for _, tc := range []struct {
		name  string
		cmd   string
		in    QueryInput
		cc    CommandContext
		field string
		code  string
	}{
		{"AQL against a data source, for now", "query", QueryInput{Query: "q", Lang: "aql", DataSource: "demo"}, cc, "data_source", "unsupported"},
		{"SQL against a dataset", "query", QueryInput{Query: "q", Lang: "sql", Dataset: "ecommerce"}, cc, "dataset", "invalid"},
		{"SQL against an unknown data source", "query.compile", QueryInput{Query: "q", Lang: "sql", DataSource: "nosuch"}, cc, "data_source", "invalid"},
		{"validating SQL, for now", "query.validate", QueryInput{Query: "q", Lang: "sql", DataSource: "demo"}, cc, "lang", "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := run(t, tc.cmd, tc.cc, tc.in)
			v, _ := apperr.DetailsOf(err, errcode.InvalidArgs)
			if len(v) != 1 || v[0].Field != tc.field || v[0].Code != tc.code {
				t.Errorf("got %v, %+v; want %s on %s", err, v, tc.code, tc.field)
			}
		})
	}

	// Nothing restricts raw SQL, so a restricted caller cannot run it at all.
	restricted := cc
	restricted.DataPerms = dataperm.Restricted(nil)
	for _, cmd := range []string{"query", "query.compile"} {
		if _, err := run(t, cmd, restricted, QueryInput{Query: "select 1", Lang: "sql", DataSource: "demo"}); !errors.Is(err, errcode.DataPermsUnenforceable) {
			t.Errorf("%s: a restricted caller's SQL got %v, want data_perms_unenforceable", cmd, err)
		}
	}

	// Compiling SQL returns it as it is, and needs no sidecar.
	res, err := run(t, "query.compile", cc, QueryInput{Query: "select 1", Lang: "sql", DataSource: "demo"})
	if err != nil || res.Data != (CompiledQuery{SQL: "select 1"}) {
		t.Errorf("compiling SQL: %+v, %v", res, err)
	}
	c, _ := Find("query")
	if got := c.Needs(map[string]any{"query": "select 1", "lang": "sql", "ds": "demo"}); got != (Sidecars{CanalQuery: true}) {
		t.Errorf("SQL needs %+v, want canal-query alone", got)
	}

	if aql, limit, err := (QueryInput{Query: "explore { products } limit: 5", Lang: "aql", Dataset: "d"}).aql(); err != nil || limit != 5 || strings.Contains(aql, "limit") {
		t.Errorf("AQL against a dataset: %q, %d, %v", aql, limit, err)
	}
}

// run dispatches cmd with in, as /call would send it.
func run(t *testing.T, cmd string, cc CommandContext, in QueryInput) (Response, error) {
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
