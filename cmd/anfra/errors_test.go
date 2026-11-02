package main

import (
	"bytes"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/apperr"
)

// A failure is its envelope as a YAML document, with the args it got wrong named
// as the user passes them, its flags and its positional, and a pointer to its
// help; a remote server's alike.
func TestPrintErrorNamesTheArgs(t *testing.T) {
	args := registered(t, "query").Args()
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"one violation", apperr.NewWith(apperr.ValidationFailed, "name the data source to run the SQL query against",
			apperr.Violate(apperr.Violation{Field: "data_source", Code: "required", Message: "name the data source to run the SQL query against"})),
			"---\nerror:\n" +
				"    code: validation_failed\n" +
				"    scope: user\n" +
				"    message: name the data source to run the SQL query against\n" +
				"    details:\n" +
				"        violations:\n" +
				"            - field: data_source\n" +
				"              code: required\n" +
				"              message: name the data source to run the SQL query against\n" +
				"              arg: --data-source\n" +
				"    help: Run `anfra query --help` for usage.\n"},
		{"several, a schema's", apperr.NewWith(apperr.InvalidRequest, "", apperr.Violate(
			apperr.Violation{Field: "lang", Code: "invalid", Message: `Must be one of: aql, sql; got "cobol".`},
			apperr.Violation{Field: "query", Code: "required", Message: "Required."},
		)),
			"---\nerror:\n" +
				"    code: invalid_request\n" +
				"    scope: client\n" +
				"    message: The request is not valid.\n" +
				"    details:\n" +
				"        violations:\n" +
				"            - field: lang\n" +
				"              code: invalid\n" +
				"              message: 'Must be one of: aql, sql; got \"cobol\".'\n" +
				"              arg: --lang\n" +
				"            - field: query\n" +
				"              code: required\n" +
				"              message: Required.\n" +
				"              arg: query\n" +
				"    help: Run `anfra query --help` for usage.\n"},
		{"a remote server's", &remoteError{resp: apperr.Response{Code: "validation_failed", Scope: apperr.User, Status: 422, Message: "x", RequestID: "req_1",
			Details: map[string]any{"violations": []any{map[string]any{"field": "dataset", "code": "invalid", "message": "x"}}}}},
			"---\nerror:\n" +
				"    code: validation_failed\n" +
				"    scope: user\n" +
				"    status: 422\n" +
				"    message: x\n" +
				"    details:\n" +
				"        violations:\n" +
				"            - code: invalid\n" +
				"              field: dataset\n" +
				"              message: x\n" +
				"              arg: --dataset\n" +
				"    request_id: req_1\n" +
				"    help: Run `anfra query --help` for usage.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			printError(&out, &commandError{path: "anfra query", args: args, err: tc.err}, "")
			if out.String() != tc.want {
				t.Errorf("got\n%s\nwant\n%s", out.String(), tc.want)
			}
		})
	}

	// Details that are not violations are shown as they are, with no help.
	var out bytes.Buffer
	printError(&out, &commandError{path: "anfra query", args: args, err: &remoteError{resp: apperr.Response{
		Code: "query_invalid", Scope: apperr.User, Message: "The query is not valid.", Details: map[string]any{"diagnostics": []any{}}}}}, "")
	if want := "---\nerror:\n    code: query_invalid\n    scope: user\n    message: The query is not valid.\n    details:\n        diagnostics: []\n"; out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}

// stderr is a stream of YAML documents: warnings, then the error, which a
// program reads apart with a YAML library.
func TestStderrIsAYAMLStream(t *testing.T) {
	var out bytes.Buffer
	printWarning(&out, "telemetry_off", "telemetry is off: no: such endpoint")
	printError(&out, apperr.New(apperr.NotFound, ""), "")

	dec := yaml.NewDecoder(&out)
	var kinds []string
	for {
		var doc map[string]map[string]any
		if err := dec.Decode(&doc); err != nil {
			break
		}
		for k, v := range doc {
			kinds = append(kinds, k+":"+v["code"].(string))
		}
	}
	if got := len(kinds); got != 2 || kinds[0] != "warning:telemetry_off" || kinds[1] != "error:not_found" {
		t.Errorf("documents %v, want a warning then the error", kinds)
	}
}

// A hint goes into help, after the usage line when there's one: one piece of
// advice a line, in one string, so help keeps its type.
func TestPrintErrorWithAHint(t *testing.T) {
	var out bytes.Buffer
	printError(&out, apperr.New(apperr.NotFound, "No such dataset."), "Run `anfra agent setup`.")
	if want := "    help: Run `anfra agent setup`.\n"; !strings.HasSuffix(out.String(), want) {
		t.Errorf("alone:\n%s", out.String())
	}

	out.Reset()
	args := []command.Arg{{Name: "dataset", Type: command.ArgString}}
	err := apperr.NewWith(apperr.ValidationFailed, "Some of the input is not valid.",
		apperr.Violate(apperr.Violation{Field: "dataset", Code: "invalid", Message: "unknown"}))
	printError(&out, &commandError{path: "anfra query", args: args, err: err}, "Run `anfra agent setup`.")
	var doc struct {
		Error struct {
			Help string `yaml:"help"`
		} `yaml:"error"`
	}
	if yerr := yaml.Unmarshal(bytes.TrimPrefix(out.Bytes(), []byte("---\n")), &doc); yerr != nil {
		t.Fatal(yerr)
	}
	if want := "Run `anfra query --help` for usage.\nRun `anfra agent setup`."; doc.Error.Help != want {
		t.Errorf("help = %q, want %q\n%s", doc.Error.Help, want, out.String())
	}
}
