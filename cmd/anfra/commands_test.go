package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
	"github.com/holistics/anfra/shared/jsonkit"
)

func registered(t *testing.T, name string) command.Command {
	t.Helper()
	c, ok := app.Find(name)
	if !ok {
		t.Fatalf("no command %s", name)
	}
	return c
}

// An answer carries its verdict: an invalid one is printed, then exits 1
// silently, since the answer says why.
func TestPresentExitsOneOnAnInvalidVerdict(t *testing.T) {
	var out bytes.Buffer
	err := presentTo(registered(t, "query.validate"), []byte(`{"valid":false,"diagnostics":[{"severity":"error","message":"no such field"}]}`), &out)
	var ec *exitCodeError
	if !errors.As(err, &ec) || ec.code != 1 {
		t.Errorf("an invalid verdict: got %v, want exit code 1", err)
	}
	if out.Len() == 0 {
		t.Error("the invalid answer was not printed")
	}
	if err := presentTo(registered(t, "query.validate"), []byte(`{"valid":true,"diagnostics":[]}`), &out); err != nil {
		t.Errorf("a valid verdict: got %v", err)
	}
}

func TestPresentSearchRendersCompactResults(t *testing.T) {
	body := []byte(`{
		"results": [
			{"display_name": "Orders", "source": "local_aml_repo", "type": "aml.model"},
			{"display_name": "Revenue", "source": "local_aml_repo", "type": "aml.metric"}
		],
		"meta": {"total": 2},
		"sql": "SELECT 1"
	}`)
	var out bytes.Buffer

	if err := presentTo(registered(t, "search"), body, &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	want := "local_aml_repo | aml.model | Orders\nlocal_aml_repo | aml.metric | Revenue\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestPresentSearchRendersEmptyOutputForNoResults(t *testing.T) {
	body := []byte(`{"results":[],"meta":{"total":0}}`)
	var out bytes.Buffer

	if err := presentTo(registered(t, "search"), body, &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	if out.String() != "" {
		t.Fatalf("output = %q, want empty", out.String())
	}
}

func TestPresentSearchRendersEmptyDisplayName(t *testing.T) {
	body := []byte(`{
		"results": [
			{"display_name": null, "source": "warehouse", "type": "database.table"},
			{"source": "warehouse", "type": "database.column"}
		]
	}`)
	var out bytes.Buffer

	if err := presentTo(registered(t, "search"), body, &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	want := "warehouse | database.table | \nwarehouse | database.column | \n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestPresentNonSearchUsesYAML(t *testing.T) {
	body := []byte(`{"version":"1.2.3"}`)
	var out bytes.Buffer

	if err := presentTo(registered(t, "version"), body, &out); err != nil {
		t.Fatalf("presentTo returned error: %v", err)
	}

	want := "version: 1.2.3\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

// A flag reaches the op as its arg's type: an int as a number, an object's JSON
// as the object; JSON that is not an object is refused before anything runs.
func TestFlagValues(t *testing.T) {
	c := registered(t, "query")
	cmd := buildCobraCommand(c)
	if err := cmd.ParseFlags([]string{"-d", "sales", "--page-size", "20", "--input", ` {"filters":[]} `}); err != nil {
		t.Fatal(err)
	}
	flagArg := map[string]string{}
	for _, a := range c.Args() {
		flagArg[a.Flag()] = a.Name
	}
	values, err := flagValues(cmd.Flags(), c.Args(), flagArg)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := jsonkit.Marshal(values)
	if want := `{"dataset":"sales","input":{"filters":[]},"page_size":20}`; string(got) != want {
		t.Errorf("input = %s, want %s", got, want)
	}

	for _, bad := range []string{"filters", "[1]", "null"} {
		cmd := buildCobraCommand(c)
		if err := cmd.ParseFlags([]string{"--input", bad}); err != nil {
			t.Fatal(err)
		}
		if _, err := flagValues(cmd.Flags(), c.Args(), flagArg); err == nil {
			t.Errorf("--input %s was accepted", bad)
		}
	}
}

type dottedOptions struct {
	Header string   `json:"header,omitempty" enum:"labels,names,none" doc:"the header row"`
	Rows   int      `json:"rows,omitempty" doc:"rows to write"`
	BOM    bool     `json:"bom,omitempty" doc:"start with a byte-order mark"`
	Tags   []string `json:"tags,omitempty" doc:"tags"`
}

type dottedInput struct {
	Options *dottedOptions `json:"format_options,omitempty" doc:"the format's options"`
}

func (dottedInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[dottedInput](s)
}

var dotted = command.Define(command.Def[dottedInput, struct{}]{
	Name: "dotted", Short: "Take an object by its fields.",
	Run: func(context.Context, command.CommandContext, dottedInput) (struct{}, error) { return struct{}{}, nil },
})

// dottedValues parses argv as dotted's flags, into the op's input.
func dottedValues(t *testing.T, argv ...string) (map[string]any, error) {
	t.Helper()
	cmd := buildCobraCommand(dotted)
	if err := cmd.ParseFlags(argv); err != nil {
		t.Fatal(err)
	}
	flagArg := map[string]string{"format-options": "format_options"}
	for _, f := range dotted.Args()[0].Fields {
		flagArg["format-options."+f.Flag()] = "format_options." + f.Name
	}
	return flagValues(cmd.Flags(), dotted.Args(), flagArg)
}

// An object arg's fields are flags of their own, named by their path, and make
// the object as its JSON would; the JSON still works alone, and the two forms
// together are refused rather than merged.
func TestDottedFlags(t *testing.T) {
	values, err := dottedValues(t, "--format-options.header", "names", "--format-options.rows", "5",
		"--format-options.bom", "--format-options.tags", "a,b")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := jsonkit.Marshal(values)
	if want := `{"format_options":{"bom":true,"header":"names","rows":5,"tags":["a","b"]}}`; string(got) != want {
		t.Errorf("input = %s, want %s", got, want)
	}

	values, err = dottedValues(t, "--format-options", `{"header":"none"}`)
	if got, _ := jsonkit.Marshal(values); err != nil || string(got) != `{"format_options":{"header":"none"}}` {
		t.Errorf("JSON alone = %s %v", got, err)
	}

	if _, err := dottedValues(t, "--format-options", `{"header":"none"}`, "--format-options.rows", "5"); err == nil ||
		!strings.Contains(err.Error(), "not both") {
		t.Errorf("both forms: %v, want refused", err)
	}

	help := buildCobraCommand(dotted).Flags().FlagUsages()
	for _, want := range []string{"--format-options.header string", "(one of: labels, names, none)", "or set its fields with --format-options.<field>"} {
		if !strings.Contains(help, want) {
			t.Errorf("help does not say %q:\n%s", want, help)
		}
	}
}

// query.export's flags: the query's, the format's options by their path, and the
// CLI's own, where the file goes or the link alone.
func TestExportFlags(t *testing.T) {
	cmd := buildCobraCommand(registered(t, "query.export"))
	for _, name := range []string{"dataset", "input", "format", "format-options", "format-options.header", "format-options.bom", "output", "link"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("no --%s", name)
		}
	}
	if f := cmd.Flags().Lookup("output"); f == nil || f.Shorthand != "o" {
		t.Error("--output has no -o")
	}
	for _, name := range []string{"page", "page-size"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Errorf("--%s on an export, which is the whole result", name)
		}
	}
	if buildCobraCommand(registered(t, "query")).Flags().Lookup("output") != nil {
		t.Error("--output on a command that answers no export")
	}
}

// An export's file is downloaded from its link, the CLI's own (file://) or a
// server's (http://), to stdout or --output; the CLI's own file, once written
// out, is removed, since it was made for that; a download that fails leaves no
// file behind, and a server's error is shown as any of its errors is.
func TestDownload(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "local.csv")
	if err := os.WriteFile(local, []byte("a\n1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exports/x" {
			httpkit.WriteError(w, r, apperr.New(apperr.NotFound, "No such export, or its link has expired."))
			return
		}
		_, _ = w.Write([]byte("b\n2\n"))
	}))
	defer srv.Close()

	for link, want := range map[string]string{
		(&url.URL{Scheme: "file", Path: local}).String(): "a\n1\n",
		srv.URL + "/exports/x":                           "b\n2\n",
	} {
		out := filepath.Join(dir, "out.csv")
		if err := download(context.Background(), link, out); err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		if b, _ := os.ReadFile(out); string(b) != want {
			t.Errorf("%s: wrote %q, want %q", link, b, want)
		}
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Error("the CLI's own export was left after it was written out")
	}

	gone := filepath.Join(dir, "gone.csv")
	var remote *remoteError
	if err := download(context.Background(), srv.URL+"/exports/gone", gone); !errors.As(err, &remote) || remote.resp.Code != "not_found" {
		t.Errorf("an expired link: %v, want the server's not_found", err)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Error("a failed download left a file")
	}
}

// How the CLI shows an answer is decided by what the answer is.
func TestPresentationOf(t *testing.T) {
	for name, want := range map[string]presentation{
		"query.export": presentDownload,
		"search":       presentSearchList,
		"query":        presentYAML,
		"version":      presentYAML,
	} {
		if got := presentationOf(registered(t, name)); got != want {
			t.Errorf("%s: %d, want %d", name, got, want)
		}
	}
}
