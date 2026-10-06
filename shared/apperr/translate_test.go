package apperr_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"
)

// A library's codes and steps, as the engine defines them: its own contract,
// which cannot name its host's codes. Its host translates its errors where it
// calls it.
var lib = apperr.DefineNamespace("lib")

type libDiagnostic struct {
	Line, Col int
	Text      string
}

var (
	libArgsInvalid  = apperr.DefinePublicCodeWith[apperr.Violations](lib, "args_invalid", apperr.Client, "The arguments are invalid.")
	libQueryInvalid = apperr.DefinePublicCodeWith[[]libDiagnostic](lib, "query_invalid", apperr.User, "The query is invalid.")
	libSidecarDown  = apperr.DefinePublicCode(lib, "sidecar_down", apperr.Server, "A sidecar is unavailable.")
	// An internal code of the library's, mapped to one of its own codes.
	libQueryUnparsed = apperr.DefineInternalCodeWith(lib, "query_unparsed", libQueryInvalid, "The query does not parse.")

	libCompile = appstep.Define(lib, "compile", appstep.Public("compile dataset {name}"))
	libParse   = appstep.Define(lib, "parse")

	// The host's own type for what the library's diagnostics say: it decides
	// what it publishes.
	hostQueryInvalid = apperr.DefinePublicCodeWith[hostDiagnostics](host, "query_invalid", apperr.User, "The query is invalid.")
)

type hostDiagnostics struct {
	Diagnostics []string `json:"diagnostics"`
	Dataset     string   `json:"dataset"`
}

// libFails is a library call failing inside its own steps.
func libFails(err error) error {
	err = step(err, libParse)
	return step(err, libCompile, "name", "sales")
}

// Translate re-codes the library's error as the host's: the code, scope and
// details are the host's; the message and the context, the library's.
func TestTranslate(t *testing.T) {
	compile := apperr.ContextEntry{Step: "lib.compile", Params: map[string]any{"name": "sales"}, Text: "compile dataset 'sales'"}
	ds := []libDiagnostic{{Line: 1, Col: 7, Text: "unknown field"}}
	for _, tc := range []struct {
		name string
		err  error
		want apperr.Response
	}{
		{
			name: "without details",
			err:  apperr.Translate(libFails(libSidecarDown), unavailable),
			want: apperr.Response{Code: "unavailable", Scope: apperr.Server, Context: []apperr.ContextEntry{compile},
				Message: "A sidecar is unavailable."},
		},
		{
			name: "with the host's details, made from the library's",
			err: func() error {
				err := libFails(apperr.NewWith(libQueryInvalid, "", ds))
				got, _ := apperr.DetailsOf(err, libQueryInvalid)
				return apperr.TranslateWith(err, hostQueryInvalid, hostDiagnostics{
					Diagnostics: []string{fmt.Sprintf("%d:%d %s", got[0].Line, got[0].Col, got[0].Text)}, Dataset: "sales"})
			}(),
			want: apperr.Response{Code: "query_invalid", Scope: apperr.User, Context: []apperr.ContextEntry{compile},
				Message: "The query is invalid.", Details: hostDiagnostics{Diagnostics: []string{"1:7 unknown field"}, Dataset: "sales"}},
		},
		{
			name: "the host re-decides scope: a client error to the library is the host's bug",
			err:  apperr.Translate(libFails(apperr.NewWith(libArgsInvalid, "", apperr.Violate())), apperr.InternalServerError),
			want: apperr.Response{Code: "internal_server_error", Scope: apperr.Server, Context: []apperr.ContextEntry{compile},
				Message: generic},
		},
		{
			name: "the host's own steps come first",
			err:  step(apperr.Translate(libFails(libSidecarDown), unavailable), importDashboard, "name", "Sales"),
			want: apperr.Response{Code: "unavailable", Scope: apperr.Server, Context: []apperr.ContextEntry{
				{Step: "host.import_dashboard", Params: map[string]any{"name": "Sales"}, Text: "import dashboard 'Sales'"}, compile},
				Message: "A sidecar is unavailable."},
		},
		{
			name: "an unclassified error's text is a diagnosis: the host's default message is used",
			err:  apperr.Translate(libFails(errDial), unavailable),
			want: apperr.Response{Code: "unavailable", Scope: apperr.Server, Context: []apperr.ContextEntry{compile},
				Message: "The service is temporarily unavailable. Try again shortly."},
		},
		{
			name: "what the library encapsulated stays hidden",
			err: apperr.Translate(libFails(apperr.Encapsulate(step(errDial, connectDatasource, "name", "warehouse"),
				libSidecarDown, "")), unavailable),
			want: apperr.Response{Code: "unavailable", Scope: apperr.Server, Context: []apperr.ContextEntry{compile},
				Message: "A sidecar is unavailable."},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.want.RequestID = "req_1"
			if got := apperr.From(tc.err).Response("req_1"); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("client receives\n  %+v\nwant\n  %+v", got, tc.want)
			}
		})
	}
}

// A translation of a translation keeps every layer's context, outermost first,
// down to the first encapsulation.
func TestNestedTranslations(t *testing.T) {
	hidden := step(errDial, connectDatasource, "name", "warehouse")
	err := libFails(apperr.Encapsulate(hidden, libSidecarDown, "")) // lib: compile > parse > sidecar_down, hiding the dial
	err = step(apperr.Translate(err, unavailable), loadDataset, "name", "orders")
	err = step(apperr.Translate(err, rateLimited), importDashboard, "name", "Sales")

	got := apperr.From(err).Response("req_1")
	var steps []string
	for _, c := range got.Context {
		steps = append(steps, c.Step)
	}
	want := []string{"host.import_dashboard", "host.load_dataset", "lib.compile"}
	if !reflect.DeepEqual(steps, want) || got.Code != "rate_limited" || got.Message != "A sidecar is unavailable." {
		t.Errorf("client receives %s %q with context %v, want rate_limited, the library's message, %v", got.Code, got.Message, steps, want)
	}
}

// Encapsulating instead hides the library's context and message.
func TestEncapsulateHidesWhatTranslateShows(t *testing.T) {
	got := apperr.From(apperr.Encapsulate(libFails(libSidecarDown), unavailable, "")).Response("req_1")
	if got.Context != nil || got.Message != "The service is temporarily unavailable. Try again shortly." {
		t.Errorf("encapsulating showed %+v", got)
	}
}

// The library's error stays whole beneath the translation: for errors.Is,
// DetailsOf and the log.
func TestTranslateKeepsTheLibrarysError(t *testing.T) {
	err := apperr.Translate(libFails(apperr.NewWith(libQueryUnparsed, "", []libDiagnostic{{Line: 2}})), hostQueryInvalid)
	for _, c := range []apperr.AnyCode{hostQueryInvalid, libQueryUnparsed, libQueryInvalid} {
		if !errors.Is(err, c) {
			t.Errorf("errors.Is(err, %s) = false", c.Code().Qualified())
		}
	}
	if ds, ok := apperr.DetailsOf(err, libQueryInvalid); !ok || len(ds) != 1 || ds[0].Line != 2 {
		t.Errorf("DetailsOf beneath a translation = %v, %v", ds, ok)
	}
	want := "host.query_invalid: The query does not parse.: lib.compile(name=sales): lib.parse: lib.query_unparsed: The query does not parse."
	if log := err.Error(); log != want {
		t.Errorf("log =\n  %s\nwant\n  %s", log, want)
	}
	if apperr.Translate(nil, unavailable) != nil {
		t.Error("translating nil made an error")
	}
}

// DetailsOf reads a code's own details, typed, anywhere in the chain.
func TestDetailsOf(t *testing.T) {
	ds := []libDiagnostic{{Line: 2, Col: 1, Text: "x"}}
	inner := apperr.NewWith(libQueryInvalid, "", ds)
	wrapped := step(apperr.Encapsulate(inner, apperr.InternalServerError, "compile"), anyStep)

	if got, ok := apperr.DetailsOf(wrapped, libQueryInvalid); !ok || !reflect.DeepEqual(got, ds) {
		t.Errorf("beneath an encapsulation: %v, %v", got, ok)
	}
	if got, ok := apperr.DetailsOf(apperr.NewWith(libQueryUnparsed, "", ds), libQueryInvalid); !ok || !reflect.DeepEqual(got, ds) {
		t.Errorf("an internal code mapped to it: %v, %v", got, ok)
	}
	if got, ok := apperr.DetailsOf(fmt.Errorf("x: %w", libQueryInvalid), libQueryInvalid); !ok || got != nil {
		t.Errorf("a bare code: %v, %v; want no details, found", got, ok)
	}
	if _, ok := apperr.DetailsOf(errors.Join(errDial, inner), libQueryInvalid); !ok {
		t.Error("not found in a join")
	}
	if _, ok := apperr.DetailsOf(apperr.New(apperr.NotFound, ""), libQueryInvalid); ok {
		t.Error("found a code that is not in the chain")
	}
}

// A library's codes are its own namespace's contract, never listed as its
// host's.
func TestLibraryCodesAreNotTheHosts(t *testing.T) {
	for _, c := range apperr.Codes(host) {
		if strings.HasPrefix(c.Qualified(), "lib.") {
			t.Errorf("%s listed as the host's", c.Qualified())
		}
	}
	if got := apperr.Codes(lib); len(got) != 3 {
		t.Errorf("lib's public codes = %v, want its three defined ones", got)
	}
}
