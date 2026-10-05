package apperr_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/holistics/anfra/shared/apperr"
)

func withoutEncapsulation(t *testing.T) {
	t.Helper()
	apperr.DisableErrorEncapsulation()
	t.Cleanup(apperr.RestoreErrorEncapsulation)
}

// With encapsulation disabled, a client sees the cause: what a local user
// needs where the error shows, since they are the one who can fix it.
func TestWithoutEncapsulation(t *testing.T) {
	withoutEncapsulation(t)
	load := apperr.ContextEntry{Step: "host.load_dataset", Params: map[string]any{"name": "orders"}, Text: "load dataset 'orders'"}
	for _, tc := range []struct {
		name    string
		err     func() error
		code    apperr.Code
		message string
		context []apperr.ContextEntry
	}{
		{
			name: "Encapsulate discloses the inner steps, and keeps its own message",
			err: func() error {
				return apperr.Encapsulate(step(errNoRows, loadDataset, "name", "orders"), apperr.NotFound, "No such dataset.")
			},
			code:    apperr.NotFound,
			message: "No such dataset.",
			context: []apperr.ContextEntry{load},
		},
		{
			name: "with no message of its own, Encapsulate shows the inner error's",
			err: func() error {
				inner := step(apperr.New(validationFailed, "The address is not valid."), loadDataset, "name", "orders")
				return apperr.Encapsulate(inner, apperr.NotFound, "")
			},
			code:    apperr.NotFound,
			message: "The address is not valid.",
			context: []apperr.ContextEntry{load},
		},
		{
			name:    "an unclassified error shows its text",
			err:     func() error { return errDial },
			code:    apperr.InternalServerError,
			message: "dial tcp 10.0.3.7:5432: connection refused",
		},
		{
			name:    "an internal_server_error shows its diagnosis",
			err:     func() error { return apperr.New(brokenReference, "") },
			code:    apperr.InternalServerError,
			message: "dashboard refers to a dataset that no longer exists",
		},
		{
			name:    "encapsulating an unclassified error with no message shows its text",
			err:     func() error { return apperr.Encapsulate(fmt.Errorf("load: %w", errDial), unavailable, "") },
			code:    unavailable.Code(),
			message: "load: dial tcp 10.0.3.7:5432: connection refused",
		},
		{
			name:    "Translate carries over an internal_server_error's text",
			err:     func() error { return apperr.Translate(step(errDial, loadDataset, "name", "orders"), unavailable) },
			code:    unavailable.Code(),
			message: "dial tcp 10.0.3.7:5432: connection refused",
			context: []apperr.ContextEntry{load},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := apperr.From(tc.err()).Response("req_1")
			want := apperr.Response{Code: tc.code.String(), Scope: tc.code.Scope(), Context: tc.context, Message: tc.message, RequestID: "req_1"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("client receives\n  %+v\nwant\n  %+v", got, want)
			}
		})
	}
}

// By default, the same errors hide their causes.
func TestEncapsulationIsTheDefault(t *testing.T) {
	err := apperr.Encapsulate(step(errDial, loadDataset, "name", "orders"), apperr.InternalServerError, "")
	got := apperr.From(err).Response("req_1")
	if got.Message != generic || got.Context != nil {
		t.Errorf("client receives %+v, want only the generic message", got)
	}
}
