package app

import (
	"context"
	"errors"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apperr"
)

// What shapes a run is refused before anything runs, on the arg at fault: what
// applies to AQL alone, a page without its size, paging beside a limit:
// directive, a time zone that is not one. query.validate takes none of it.
func TestQueryRunChecks(t *testing.T) {
	cc := command.CommandContext{DataPerms: dataperm.Unrestricted()}
	for _, tc := range []struct {
		name, cmd, body string
		code            apperr.AnyCode
		field, vcode    string
	}{
		{"input on SQL", "query", `{"query":"select 1","lang":"sql","data_source":"demo","input":{}}`, apperr.ValidationFailed, "input", "unsupported"},
		{"paging SQL", "query.compile", `{"query":"select 1","lang":"sql","data_source":"demo","page_size":10}`, apperr.ValidationFailed, "page_size", "unsupported"},
		{"a time zone on SQL", "query", `{"query":"select 1","lang":"sql","data_source":"demo","timezone":"UTC"}`, apperr.ValidationFailed, "timezone", "unsupported"},
		{"a page without its size", "query", `{"query":"q","dataset":"d","page":2}`, apperr.ValidationFailed, "page_size", "required"},
		{"paging and a limit: directive", "query", `{"query":"explore { products } limit: 5","dataset":"d","page_size":10}`, apperr.ValidationFailed, "page_size", "invalid"},
		{"a time zone that is not one", "query.compile", `{"query":"q","dataset":"d","timezone":"Mars/Olympus"}`, apperr.ValidationFailed, "timezone", "invalid"},
		{"the local time zone, which names none", "query", `{"query":"q","dataset":"d","timezone":"Local"}`, apperr.ValidationFailed, "timezone", "invalid"},
		{"a page of 0, sent", "query", `{"query":"q","dataset":"d","page":0,"page_size":10}`, apperr.InvalidRequest, "page", "invalid"},
		{"a page size of 0, sent", "query", `{"query":"q","dataset":"d","page_size":0}`, apperr.InvalidRequest, "page_size", "invalid"},
		{"a negative page", "query", `{"query":"q","dataset":"d","page":-1,"page_size":10}`, apperr.InvalidRequest, "page", "invalid"},
		{"a shaped run, checked", "query", `{"query":"q","dataset":"d","page":2,"page_size":10,"timezone":"UTC","input":{"filters":[{"field":"f","operator":"is","values":["x"]}]}}`, nil, "", ""},
		{"a sort direction that is not one", "query", `{"query":"q","dataset":"d","input":{"sorts":[{"field":"f","direction":"up"}]}}`, apperr.InvalidRequest, "input.sorts[0].direction", "invalid"},
		{"a filter with no operator", "query", `{"query":"q","dataset":"d","input":{"filters":[{"field":"f","values":[]}]}}`, apperr.InvalidRequest, "input.filters[0].operator", "required"},
		{"a filter with no values", "query", `{"query":"q","dataset":"d","input":{"filters":[{"field":"f","operator":"is_null"}]}}`, apperr.InvalidRequest, "input.filters[0].values", "required"},
		{"a type show does not know", "show", `{"type":"aml.model","fqn":"orders"}`, apperr.InvalidRequest, "type", "invalid"},
		{"input on query.validate", "query.validate", `{"query":"q","dataset":"d","input":{}}`, apperr.InvalidRequest, "input", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Invoke(context.Background(), cc, tc.cmd, []byte(tc.body))
			if tc.code == nil {
				// Not refused: it goes on to the sidecar, and there is none.
				if !errors.Is(err, errcode.SidecarUnavailable) {
					t.Errorf("got %v, want it to reach the sidecar", err)
				}
				return
			}
			var v apperr.Violations
			switch tc.code {
			case apperr.InvalidRequest:
				v, _ = apperr.DetailsOf(err, apperr.InvalidRequest)
			default:
				d, _ := apperr.DetailsOf(err, apperr.ValidationFailed)
				v = d
			}
			if len(v.Violations) != 1 || v.Violations[0].Field != tc.field || v.Violations[0].Code != tc.vcode {
				t.Errorf("got %v, %+v; want %s on %s", err, v.Violations, tc.vcode, tc.field)
			}
		})
	}
}
