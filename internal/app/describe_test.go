package app

import (
	"reflect"
	"slices"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/aml"
	"github.com/holistics/anfra/internal/command"
	querycmd "github.com/holistics/anfra/internal/command/query"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/shared/apperr"
)

// Every registered command is described, in registry order.
func TestDescribeCoversTheRegistry(t *testing.T) {
	specs := Describe()
	if len(specs) != len(Commands) {
		t.Fatalf("%d specs for %d commands", len(specs), len(Commands))
	}
	for i, c := range Commands {
		if specs[i].Name != c.Name() || specs[i].Short != c.Short() {
			t.Errorf("spec %d is %q, want %q", i, specs[i].Name, c.Name())
		}
	}
}

// A spec is derived from the definition: its args from In, its answer from Out,
// whether it can be invalid from Valid. The CLI's sugar stays out.
func TestDescribeIsDerivedAndTheAPIView(t *testing.T) {
	byName := map[string]CommandSpec{}
	for _, s := range Describe() {
		byName[s.Name] = s
	}
	q := byName["query"]
	want := []ArgSpec{
		{Name: "query", Type: command.ArgString, Required: true, Usage: "the query"},
		{Name: "lang", Type: command.ArgString, Enum: []string{"aql", "sql"}, Default: "aql", Usage: "the language the query is written in"},
		{Name: "dataset", Type: command.ArgString, Usage: "the dataset an AQL query runs against"},
		{Name: "data_source", Type: command.ArgString, Usage: "the data source a SQL query runs against"},
	}
	// query.validate checks the query alone; query and query.compile also take
	// what shapes its run.
	if v := byName["query.validate"].Args; !reflect.DeepEqual(v, want) {
		t.Errorf("query.validate's args =\n  %+v\nwant\n  %+v", v, want)
	}
	deprecated := []ArgSpec{
		{Name: "page", Type: command.ArgInt, Usage: "deprecated: use input.pagination.page. Ignored when input.pagination is set."},
		{Name: "page_size", Type: command.ArgInt, Usage: "deprecated: use input.pagination.pageSize. Ignored when input.pagination is set."},
		{Name: "timezone", Type: command.ArgString, Usage: "deprecated: use input.timezone. Ignored when input.timezone is set."},
	}
	input := func(answer string) ArgSpec {
		return ArgSpec{Name: "input", Type: command.ArgObject, Usage: "the query's input, for AQL only: the Query Input (filters, conditions, sorts and date drills applied to the AQL before it compiles), a page of rows, the time zone, and the seeds. " + answer}
	}
	// query also takes how the cache answers it; query.compile, which runs
	// nothing, doesn't.
	for name, args := range map[string][]ArgSpec{
		"query": slices.Concat(want, []ArgSpec{input("The answer returns it as the query ran: send it back with the same query to get the same answer again.")}, deprecated,
			[]ArgSpec{{Name: "cache", Type: command.ArgObject, Usage: "how to use the query cache for this request. A query uses it when it sets cache.duration, or when the repo sets a default (cache.query.default.duration). Otherwise it runs on the warehouse, uncached."}}),
		"query.compile": slices.Concat(want, []ArgSpec{input("The answer returns it as the query compiled: send it back with the same query to get the same SQL again.")}, deprecated),
	} {
		if got := byName[name].Args; !reflect.DeepEqual(got, args) {
			t.Errorf("%s's args =\n  %+v\nwant\n  %+v", name, got, args)
		}
	}
	// Which target is required depends on lang, which a group cannot say.
	if len(q.ExactlyOne) != 0 {
		t.Errorf("query.ExactlyOne = %v, want none", q.ExactlyOne)
	}

	for name, out := range map[string]reflect.Type{
		"query":          reflect.TypeFor[querycmd.QueryResult](),
		"query.compile":  reflect.TypeFor[querycmd.CompiledQuery](),
		"query.validate": reflect.TypeFor[query.QueryValidation](),
		"validate":       reflect.TypeFor[aml.RepoValidation](),
	} {
		if byName[name].Output != out {
			t.Errorf("%s answers %v, want %v", name, byName[name].Output, out)
		}
	}
	for name, invalid := range map[string]bool{"query": false, "query.compile": false, "query.validate": true, "validate": true, "status": true, "version": false} {
		if byName[name].CanBeInvalid != invalid {
			t.Errorf("%s: CanBeInvalid = %v", name, !invalid)
		}
	}

	if got, want := byName["version"].ErrorCodes, []apperr.Code{errcode.DataPermsMissing, apperr.ValidationFailed.Code()}; !reflect.DeepEqual(got, want) {
		t.Errorf("version can fail with %v, want %v", got, want)
	}
	for _, c := range []apperr.Code{errcode.SidecarUnavailable, query.QueryInvalid.Code()} {
		if !slices.Contains(byName["query"].ErrorCodes, c) {
			t.Errorf("query cannot fail with %s", c)
		}
	}
	if slices.Contains(byName["query.validate"].ErrorCodes, query.QueryInvalid.Code()) {
		t.Error("query.validate reports an invalid query; it does not fail with one")
	}
}

// A spec is a copy: changing it leaves the registry alone.
func TestDescribeReturnsCopies(t *testing.T) {
	for _, s := range Describe() {
		if s.Name == "query" {
			s.Args[1].Enum[0] = "clobbered"
		}
	}
	c, _ := Find("query")
	if c.Args()[1].Enum[0] == "clobbered" {
		t.Error("changing a spec changed the registry")
	}
}

// What a spec says is what the op's schema accepts: every command's args, set
// validly as the spec describes them, validate.
func TestSpecsAgreeWithTheSchema(t *testing.T) {
	reg, rt := NewRegistry(), NewRuntime()
	for _, s := range Describe() {
		valid := map[string]any{}
		for _, a := range s.Args {
			if a.Required {
				valid[a.Name] = "x"
			}
		}
		for _, g := range s.ExactlyOne {
			valid[g[0]] = "x"
		}
		o, _ := reg.Lookup(command.OpName(s.Name))
		res := &huma.ValidateResult{}
		huma.Validate(rt.Schemas, o.InSchema(rt), huma.NewPathBuffer(nil, 0), huma.ModeWriteToServer, valid, res)
		if len(res.Errors) > 0 {
			t.Errorf("%s: %v refused: %v", s.Name, valid, res.Errors)
		}
	}
}
