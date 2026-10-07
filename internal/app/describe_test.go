package app

import (
	"github.com/danielgtaylor/huma/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/validate"
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
		{Name: "query", Type: ArgString, Required: true, Usage: "the query"},
		{Name: "lang", Type: ArgString, Enum: []string{"aql", "sql"}, Default: "aql", Usage: "the language the query is written in"},
		{Name: "dataset", Type: ArgString, Usage: "the dataset an AQL query runs against"},
		{Name: "data_source", Type: ArgString, Usage: "the data source a SQL query runs against"},
	}
	// query.validate checks the query alone; query and query.compile also take
	// what shapes its run.
	if v := byName["query.validate"].Args; !reflect.DeepEqual(v, want) {
		t.Errorf("query.validate's args =\n  %+v\nwant\n  %+v", v, want)
	}
	want = append(want,
		ArgSpec{Name: "input", Type: ArgObject, Usage: "the Query Input: filters, conditions, sorts and date drills applied to the AQL before it compiles"},
		ArgSpec{Name: "page", Type: ArgInt, Usage: "the 1-based page of rows to answer; needs a page size"},
		ArgSpec{Name: "page_size", Type: ArgInt, Usage: "rows per page; alone, the first page"},
		ArgSpec{Name: "timezone", Type: ArgString, Usage: "the IANA time zone relative dates and date truncation use, such as Asia/Ho_Chi_Minh"},
	)
	for _, name := range []string{"query", "query.compile"} {
		if args := byName[name].Args; !reflect.DeepEqual(args, want) {
			t.Errorf("%s's args =\n  %+v\nwant\n  %+v", name, args, want)
		}
	}
	// Which target is required depends on lang, which a group cannot say.
	if len(q.ExactlyOne) != 0 {
		t.Errorf("query.ExactlyOne = %v, want none", q.ExactlyOne)
	}

	for name, out := range map[string]reflect.Type{
		"query":          reflect.TypeFor[QueryResult](),
		"query.compile":  reflect.TypeFor[CompiledQuery](),
		"query.validate": reflect.TypeFor[validate.QueryValidation](),
		"validate":       reflect.TypeFor[validate.RepoValidation](),
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
	for _, c := range []apperr.Code{errcode.SidecarUnavailable, validate.QueryInvalid.Code()} {
		if !slices.Contains(byName["query"].ErrorCodes, c) {
			t.Errorf("query cannot fail with %s", c)
		}
	}
	if slices.Contains(byName["query.validate"].ErrorCodes, validate.QueryInvalid.Code()) {
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
		o, _ := reg.Lookup(OpName(s.Name))
		res := &huma.ValidateResult{}
		huma.Validate(rt.Schemas, o.InSchema(rt), huma.NewPathBuffer(nil, 0), huma.ModeWriteToServer, valid, res)
		if len(res.Errors) > 0 {
			t.Errorf("%s: %v refused: %v", s.Name, valid, res.Errors)
		}
	}
}
