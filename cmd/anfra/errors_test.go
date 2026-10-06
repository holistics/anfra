package main

import (
	"bytes"
	"testing"

	"github.com/holistics/anfra/shared/apperr"
)

// The args a command got wrong are named as the user passes them, its flags
// and its positional, with a pointer to its help; a remote server's alike.
func TestPrintErrorNamesTheArgs(t *testing.T) {
	args := command(t, "query").Args()
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"one violation", apperr.NewWith(apperr.ValidationFailed, "name the data source to run the SQL query against",
			apperr.Violate(apperr.Violation{Field: "data_source", Code: "required", Message: "name the data source to run the SQL query against"})),
			"Error: --data-source: name the data source to run the SQL query against\nRun `anfra query --help` for usage.\n"},
		{"several, a schema's", apperr.NewWith(apperr.InvalidRequest, "", apperr.Violate(
			apperr.Violation{Field: "lang", Code: "invalid", Message: `Must be one of: aql, sql; got "cobol".`},
			apperr.Violation{Field: "query", Code: "required", Message: "Required."},
		)),
			"Error: these arguments are not valid:\n" +
				"  --lang  must be one of: aql, sql; got \"cobol\"\n" +
				"  query   required: pass it as an argument, or pipe it on stdin\n" +
				"Run `anfra query --help` for usage.\n"},
		{"a remote server's", &remoteError{resp: apperr.Response{Code: "validation_failed", Message: "x",
			Details: map[string]any{"violations": []any{map[string]any{"field": "dataset", "code": "invalid", "message": "x"}}}}},
			"Error: --dataset: x\nRun `anfra query --help` for usage.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			printError(&out, &commandError{path: "anfra query", args: args, err: tc.err})
			if out.String() != tc.want {
				t.Errorf("got\n%s\nwant\n%s", out.String(), tc.want)
			}
		})
	}

	// Details that are not violations are shown as they are.
	var out bytes.Buffer
	printError(&out, &commandError{path: "anfra query", args: args, err: &remoteError{resp: apperr.Response{
		Code: "query_invalid", Message: "The query is not valid.", Details: map[string]any{"diagnostics": []any{}}}}})
	if want := "Error: The query is not valid.\ndiagnostics: []\n"; out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}
