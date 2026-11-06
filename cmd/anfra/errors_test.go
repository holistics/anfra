package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
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
				"  code: validation_failed\n" +
				"  scope: user\n" +
				"  message: name the data source to run the SQL query against\n" +
				"  details:\n" +
				"    violations:\n" +
				"      - field: data_source\n" +
				"        code: required\n" +
				"        message: name the data source to run the SQL query against\n" +
				"        arg: --data-source\n" +
				"  request_id: \"\"\n" +
				"  help: Run `anfra query --help` for usage.\n"},
		{"several, a schema's", apperr.NewWith(apperr.InvalidRequest, "", apperr.Violate(
			apperr.Violation{Field: "lang", Code: "invalid", Message: `Must be one of: aql, sql; got "cobol".`},
			apperr.Violation{Field: "query", Code: "required", Message: "Required."},
		)),
			"---\nerror:\n" +
				"  code: invalid_request\n" +
				"  scope: client\n" +
				"  message: The request is not valid.\n" +
				"  details:\n" +
				"    violations:\n" +
				"      - field: lang\n" +
				"        code: invalid\n" +
				"        message: 'Must be one of: aql, sql; got \"cobol\".'\n" +
				"        arg: --lang\n" +
				"      - field: query\n" +
				"        code: required\n" +
				"        message: Required.\n" +
				"        arg: query\n" +
				"  request_id: \"\"\n" +
				"  help: Run `anfra query --help` for usage.\n"},
		{"a remote server's, in its order", remote(t, `{"error": {"code": "validation_failed", "scope": "user", "status": 422, "message": "x",
			"details": {"violations": [{"field": "dataset", "code": "invalid", "message": "x"}]}, "request_id": "req_1"}}`),
			"---\nerror:\n" +
				"  code: validation_failed\n" +
				"  scope: user\n" +
				"  status: 422\n" +
				"  message: x\n" +
				"  details:\n" +
				"    violations:\n" +
				"      - field: dataset\n" +
				"        code: invalid\n" +
				"        message: x\n" +
				"        arg: --dataset\n" +
				"  request_id: req_1\n" +
				"  help: Run `anfra query --help` for usage.\n"},
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
	if want := "---\nerror:\n  code: query_invalid\n  scope: user\n  message: The query is not valid.\n  details:\n    diagnostics: []\n  request_id: \"\"\n"; out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}

// remote is the error a server's error answer, body, carries.
func remote(t *testing.T, body string) *remoteError {
	t.Helper()
	re, ok := remoteErrorFrom([]byte(body))
	if !ok {
		t.Fatalf("not an error answer: %s", body)
	}
	return re
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
	if want := "  help: Run `anfra agent setup`.\n"; !strings.HasSuffix(out.String(), want) {
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

// repo_invalid is anfra-cloud's code, with its details: validate's answer as
// validation, so a repo refused here and one refused by the org's server read
// the same.
func TestRepoInvalidAsTheCloudHasIt(t *testing.T) {
	var out bytes.Buffer
	printError(&out, apperr.NewWith(clierr.RepoInvalid, "Not published, because the repo has errors. Fix them, and publish again.",
		clierr.RepoProblems{Validation: jsontext.Value(`{"valid":false,"diagnostics":[],"reports":[]}`)}), "")
	want := "---\nerror:\n" +
		"  code: repo_invalid\n" +
		"  scope: user\n" +
		"  message: Not published, because the repo has errors. Fix them, and publish again.\n" +
		"  details:\n" +
		"    validation:\n" +
		"      valid: false\n" +
		"      diagnostics: []\n" +
		"      reports: []\n" +
		"  request_id: \"\"\n"
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}

// cobra's refusals are the caller's mistakes: a flag as invalid_request on the
// flag, an unknown command as unknown_command, the args as invalid_request.
func TestCobraRefusalsAreCoded(t *testing.T) {
	t.Cleanup(resetOutput)
	for args, want := range map[string]struct{ code, field string }{
		"version --nope":        {"invalid_request", "--nope"},
		"query --page-size x q": {"invalid_request", "--page-size"},
		"nope":                  {"unknown_command", ""},
		"openapi extra":         {"invalid_request", "args"},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(strings.Fields(args))
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		err := runError(context.Background(), cmd.Execute())
		got := apperr.From(err).Response("")
		if got.Code != want.code {
			t.Errorf("%s: %s, want %s (%v)", args, got.Code, want.code, err)
			continue
		}
		if want.field != "" {
			if v, ok := got.Details.(apperr.Violations); !ok || len(v.Violations) != 1 || v.Violations[0].Field != want.field {
				t.Errorf("%s: details %+v, want a violation on %s", args, got.Details, want.field)
			}
		}
	}
}

// A run the user interrupted is interrupted, whatever failed as it stopped.
func TestInterruptedRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := apperr.From(runError(ctx, errors.New("read: connection reset"))).Code; got != clierr.Interrupted.Code() {
		t.Errorf("code %v, want interrupted", got)
	}
}

// The org's server's errors are shown as it answered them.
func TestCloudErrorsPassThrough(t *testing.T) {
	var out bytes.Buffer
	printError(&out, &cloudapi.Error{Status: 403, Response: apperr.Response{Code: "forbidden", Scope: apperr.User, Message: "Only an admin can do that."}}, "")
	if !strings.HasPrefix(out.String(), "---\nerror:\n  code: forbidden\n  scope: user\n  message: Only an admin can do that.\n") {
		t.Errorf("got\n%s", out.String())
	}
}

// In a console, an error naming a flag says how to set it for the session,
// and the usage as typed there, without the anfra.
func TestConsoleErrorHelp(t *testing.T) {
	t.Cleanup(func() { output.inConsole = false })
	err := apperr.NewWith(apperr.ValidationFailed, "x", apperr.Violate(apperr.Violation{Field: "data_source", Code: "required", Message: "x"}))
	args := []command.Arg{{Name: "data_source", Type: command.ArgString}}
	for _, tc := range []struct {
		inConsole bool
		want      string
	}{
		{false, "help: Run `anfra query --help` for usage."},
		{true, "Help:  `\\--data-source <value>` sets it for every command here. Run `query --help` for usage."},
	} {
		output.inConsole = tc.inConsole
		var out bytes.Buffer
		printError(&out, &commandError{path: "anfra query", args: args, err: err}, "")
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("inConsole=%v:\n%s\nwant %s", tc.inConsole, &out, tc.want)
		}
	}
}

// For a person, an error is pretty: a ✗ and its message, then its code, its
// details by the conventions, and its help; a warning a ! and its message.
func TestPrettyError(t *testing.T) {
	saved := output
	t.Cleanup(func() { output = saved })
	output = saved
	output.inConsole, output.json, output.yaml, output.jq = true, false, false, ""
	err := apperr.NewWith(apperr.ValidationFailed, "Some of the input is not valid.", apperr.Violate(apperr.Violation{Field: "data_source", Code: "required", Message: "name the data source"}))
	var out bytes.Buffer
	printError(&out, &commandError{path: "anfra query", args: []command.Arg{{Name: "data_source", Type: command.ArgString}}, err: err}, "")
	for _, want := range []string{"✗ Some of the input is not valid.\n", "  Code:  validation_failed", "  Violations:", "FIELD", "data_source", "--data-source", "Help:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, &out)
		}
	}
	for _, unwanted := range []string{"scope", "Request ID", "---"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("%q in:\n%s", unwanted, &out)
		}
	}
	out.Reset()
	printWarning(&out, "telemetry_off", "Telemetry is off.")
	if out.String() != "! Telemetry is off. (telemetry_off)\n" {
		t.Errorf("warning %q", &out)
	}
}

// A message is split into its chain of causes at ": ", a short part staying
// with the one before it.
func TestMessageParts(t *testing.T) {
	for msg, want := range map[string][]string{
		"resolve anfra-node binary: unpack anfra-node into anfra's folder (ANFRA_HOME): mkdir /home/ana/.anfra/sidecars: permission denied": {"resolve anfra-node binary", "unpack anfra-node into anfra's folder (ANFRA_HOME): mkdir /home/ana/.anfra/sidecars: permission denied"}, // 2 words: joined
		`apps/a.html: line 3: "x.png" is a file beside the app`:                                                                             {"apps/a.html: line 3", `"x.png" is a file beside the app`},
		"download the export: exit status 1":                                                                                                {"download the export: exit status 1"},
		"Some of the input is not valid.":                                                                                                   {"Some of the input is not valid."},
		"see https://example.com/a at 12:30 today":                                                                                          {"see https://example.com/a at 12:30 today"},
	} {
		if got := messageParts(msg); !slices.Equal(got, want) {
			t.Errorf("messageParts(%q) = %q, want %q", msg, got, want)
		}
	}
}

// A message split into causes keeps its colons, and is set apart by a blank
// line; one that isn't split isn't.
func TestPrettyErrorChain(t *testing.T) {
	saved := output
	t.Cleanup(func() { output = saved })
	output = saved
	output.inConsole, output.json, output.yaml, output.jq = true, false, false, ""
	var out bytes.Buffer
	printError(&out, apperr.New(apperr.NotFound, `apps/a.html: line 3: "x.png" is a file beside the app`), "")
	if !strings.HasPrefix(out.String(), "✗ apps/a.html: line 3:\n    \"x.png\" is a file beside the app\n\n  Code:") {
		t.Errorf("split: %q", &out)
	}
	out.Reset()
	printError(&out, apperr.New(apperr.NotFound, "No such dataset."), "")
	if !strings.HasPrefix(out.String(), "✗ No such dataset.\n  Code:") {
		t.Errorf("whole: %q", &out)
	}
}

// A sidecar that's unavailable points to the log file its logs are in.
func TestPrintErrorSidecarHelp(t *testing.T) {
	var out bytes.Buffer
	printError(&out, apperr.New(errcode.SidecarUnavailable, "anfra-node didn't start within 10s"), "")
	if !strings.Contains(out.String(), "help: The sidecar's logs are in the repo's log file") {
		t.Errorf("got:\n%s", out.String())
	}
}
