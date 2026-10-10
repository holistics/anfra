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
		{"undecided permissions", engine.Invocation{}, engine.Request{Command: "version"}, engine.DataPermsMissing, apperr.Violations{}},
		{"an unknown command", unrestricted, engine.Request{Command: "no-such-command"}, engine.UnknownCommand, apperr.Violations{}},
		{"an unknown arg", unrestricted, engine.Request{Command: "version", Args: map[string]any{"bogus": true}},
			apperr.InvalidRequest, apperr.Violate(apperr.Violation{Field: "bogus", Code: "unknown",
				Message: "Not a field of this input. Field names are snake_case and case-sensitive."})},
		{"missing args", unrestricted, engine.Request{Command: "query.compile", Args: map[string]any{}},
			apperr.InvalidRequest, apperr.Violate(apperr.Violation{Field: "query", Code: "required", Message: "Required."})},
		{"no target", unrestricted, engine.Request{Command: "query.compile", Args: map[string]any{"query": "select 1", "lang": "sql"}},
			apperr.ValidationFailed, apperr.Violate(apperr.Violation{Field: "data_source", Code: "required", Message: "name the data source to run the SQL query against"})},
		{"an unsupported input", unrestricted,
			engine.Request{Command: "query", Args: map[string]any{"query": "x", "data_source": "w"}},
			apperr.ValidationFailed, apperr.Violate(apperr.Violation{Field: "data_source", Code: "unsupported", Message: "an AQL query runs against a dataset"})},
		{"SQL for a restricted caller", engine.Invocation{DataPerms: engine.Restricted(nil)},
			engine.Request{Command: "query", Args: map[string]any{"query": "select 1", "lang": "sql", "data_source": "w"}},
			engine.DataPermsUnenforceable, apperr.Violations{}},
		{"no sidecars to run on", unrestricted, engine.Request{Command: "query", Args: map[string]any{"dataset": "d", "query": "x"}},
			engine.SidecarUnavailable, apperr.Violations{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Dispatch(context.Background(), tc.inv, tc.req)
			e := apperr.From(err)
			if e == nil || e.Code != tc.code.Code() || !errors.Is(err, tc.code) {
				t.Fatalf("got %v, want %s", err, tc.code.Code().Qualified())
			}
			got, _ := e.Details.(apperr.Violations)
			if !reflect.DeepEqual(got, tc.violations) {
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
	want := []string{"data_perms_missing", "data_perms_unenforceable", "exports_unavailable", "query_failed", "query_invalid", "sidecar_unavailable", "unknown_command"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("ErrorCodes() = %v, want %v", names, want)
	}
	if apperr.ValidationFailed.DetailsType() != reflect.TypeFor[apperr.Violations]() {
		t.Error("validation_failed does not carry violations")
	}
}
