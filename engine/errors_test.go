package engine_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/holistics/anfra/engine"
	"github.com/holistics/anfra/shared/apperr"
)

// Dispatch's refusals are classified: a host can tell a caller's mistake from
// its own and from an outage without reading messages.
func TestDispatchErrorsAreClassified(t *testing.T) {
	unrestricted := engine.Invocation{DataPerms: engine.Unrestricted()}
	for _, tc := range []struct {
		name       string
		inv        engine.Invocation
		req        engine.Request
		code       apperr.AnyCode
		violations apperr.Violations
	}{
		{"undecided permissions", engine.Invocation{}, engine.Request{Command: "version"}, engine.DataPermsMissing, nil},
		{"an unknown command", unrestricted, engine.Request{Command: "no-such-command"}, engine.UnknownCommand, nil},
		{"an unknown arg", unrestricted, engine.Request{Command: "version", Args: map[string]any{"bogus": true}},
			engine.InvalidArgs, apperr.Violations{{Field: "bogus", Code: "unknown", Message: "Not an arg of version."}}},
		{"two targets", unrestricted,
			engine.Request{Command: "query", Args: map[string]any{"query": "x", "dataset": "d", "data_source": "w"}},
			engine.InvalidArgs, apperr.Violations{
				{Field: "data_source", Code: "invalid", Message: "only one of dataset, data_source may be set"}}},
		{"missing args", unrestricted, engine.Request{Command: "query.compile", Args: map[string]any{}},
			engine.InvalidArgs, apperr.Violations{
				{Field: "query", Code: "required", Message: "query is required"},
				{Field: "dataset", Code: "required", Message: "one of dataset, data_source is required"}}},
		{"an unsupported input", unrestricted,
			engine.Request{Command: "query", Args: map[string]any{"query": "x", "ds": "w"}},
			engine.InvalidArgs, apperr.Violations{{Field: "data_source", Code: "unsupported", Message: "an AQL query against a data source is not supported yet"}}},
		{"SQL for a restricted caller", engine.Invocation{DataPerms: engine.Restricted(nil)},
			engine.Request{Command: "query", Args: map[string]any{"query": "select 1", "lang": "sql", "ds": "w"}},
			engine.DataPermsUnenforceable, nil},
		{"no sidecars to run on", unrestricted, engine.Request{Command: "query", Args: map[string]any{"dataset": "d", "query": "x"}},
			engine.SidecarUnavailable, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Dispatch(context.Background(), tc.inv, tc.req)
			e := apperr.From(err)
			if e == nil || e.Code != tc.code.Code() || !errors.Is(err, tc.code) {
				t.Fatalf("got %v, want %s", err, tc.code.Code().Qualified())
			}
			if got, _ := apperr.DetailsOf(err, engine.InvalidArgs); !reflect.DeepEqual(got, tc.violations) {
				t.Errorf("violations = %+v, want %+v", got, tc.violations)
			}
		})
	}
}

// ErrorCodes is the engine's whole catalog, in its own namespace.
func TestErrorCodes(t *testing.T) {
	var names []string
	for _, c := range engine.ErrorCodes() {
		if c.Namespace() != engine.Namespace {
			t.Errorf("%s is not the engine's", c.Qualified())
		}
		names = append(names, c.String())
	}
	want := []string{"data_perms_missing", "data_perms_unenforceable", "invalid_args", "query_failed", "query_invalid", "sidecar_unavailable", "unknown_command"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("ErrorCodes() = %v, want %v", names, want)
	}
	if engine.InvalidArgs.DetailsType() != reflect.TypeFor[apperr.Violations]() {
		t.Error("invalid_args does not carry violations")
	}
}
