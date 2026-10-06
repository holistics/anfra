package apperr_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"
)

// Stand-ins for what lower layers produce.
var (
	// errNoRows is a driver sentinel, like pgx.ErrNoRows.
	errNoRows = errors.New("no rows in result set")
	// errDial is an infrastructure failure whose text must never reach a caller.
	errDial = errors.New("dial tcp 10.0.3.7:5432: connection refused")
)

// A host's own codes, as a host defines them: apperr has only
// the generic ones.
var host = apperr.DefineNamespace("host")

var (
	validationFailed = apperr.ValidationFailed
	unauthenticated  = apperr.DefinePublicCode(host, "unauthenticated", apperr.User, "Sign in to continue.")
	forbidden        = apperr.DefinePublicCode(host, "forbidden", apperr.User, "You do not have permission to do this.")
	unavailable      = apperr.DefinePublicCodeWith[retryAfter](host, "unavailable", apperr.Server, "The service is temporarily unavailable. Try again shortly.")
	rateLimited      = apperr.DefinePublicCodeWith[retryAfter](host, "rate_limited", apperr.Client, "Too many requests. Try again shortly.")
)

type retryAfter struct {
	Seconds int `json:"retry_after_seconds"`
}

// Internal codes, as a module would define them for its own callers.
var (
	slugTaken = apperr.DefineInternalCodeWith(host, "slug_taken", validationFailed,
		"This address is already in use.")
	brokenReference = apperr.DefineInternalCode(host, "broken_reference", apperr.InternalServerError,
		"dashboard refers to a dataset that no longer exists")
)

// thirdPartyError is another package's wrapper: not ours, so unclassified.
type thirdPartyError struct{ err error }

func (e *thirdPartyError) Error() string { return "third party: " + e.err.Error() }
func (e *thirdPartyError) Unwrap() error { return e.err }

// Step definitions, made once as a module would. step adds an occurrence of one
// to an error with WithStep, as apptracing does when a failing step ends;
// apptracing's own tests cover Start and End, and appstep's cover a step in
// isolation.
var (
	importDashboard   = appstep.Define(host, "import_dashboard", appstep.Public("import dashboard {name}"))
	validateDataset   = appstep.Define(host, "validate_dataset", appstep.Public("validate dataset {name}"))
	loadDataset       = appstep.Define(host, "load_dataset", appstep.Public("load dataset {name}"))
	connectDatasource = appstep.Define(host, "connect_datasource", appstep.Public("connect to data source {name}"))
	invite            = appstep.Define(host, "invite", appstep.Public("invite {email}"), appstep.Sensitive("email"))
	checkReferences   = appstep.Define(host, "check_references")
	setup             = appstep.Define(host, "setup")
	renderDashboard   = appstep.Define(host, "render_dashboard")
	resolve           = appstep.Define(host, "resolve")
	load              = appstep.Define(host, "load")
	anyStep           = appstep.Define(host, "x")
)

func step(err error, def appstep.Def, kv ...any) error {
	return apperr.WithStep(err, appstep.NewStep(def, kv...))
}

const generic = "Something went wrong on our side."

// The use cases, each a chain of errors — innermost first
// — and what comes out: the whole response a client receives, and the log line.
// The log carries the full chain on purpose; it never reaches a client.
func TestCases(t *testing.T) {
	cases := []struct {
		name  string
		chain func() error

		// What the client receives.
		code    apperr.Code
		message string
		details any
		context []apperr.ContextEntry

		// What only the server log receives.
		log string
	}{
		// ---- Classifying where the failure starts.
		{
			name:    "a formal error with its code's default message",
			chain:   func() error { return apperr.New(apperr.NotFound, "") },
			code:    apperr.NotFound,
			message: "Not found.",
			log:     "apperr.not_found: Not found.",
		},
		{
			name:    "a formal error with its own message",
			chain:   func() error { return apperr.New(apperr.NotFound, "No such tenant.") },
			code:    apperr.NotFound,
			message: "No such tenant.",
			log:     "apperr.not_found: No such tenant.",
		},
		{
			name:    "a bare code is a formal error with the default message",
			chain:   func() error { return unauthenticated },
			code:    unauthenticated,
			message: "Sign in to continue.",
			log:     "host.unauthenticated: Sign in to continue.",
		},
		{
			name: "typed details reach the client",
			chain: func() error {
				return apperr.NewWith(validationFailed, "The address is not valid.",
					apperr.Violations{{Field: "slug", Code: "invalid", Message: "not a valid address"}})
			},
			code:    validationFailed.Code(),
			message: "The address is not valid.",
			details: apperr.Violations{{Field: "slug", Code: "invalid", Message: "not a valid address"}},
			log:     "apperr.validation_failed: The address is not valid.",
		},
		{
			name: "a structural error carries the caller's field paths",
			chain: func() error {
				return apperr.NewWith(apperr.InvalidRequest, "",
					apperr.Violations{{Field: "items[1].role", Code: "invalid", Message: "Must be one of: admin, analyst."}})
			},
			code:    apperr.InvalidRequest.Code(),
			message: "The request is not valid.",
			details: apperr.Violations{{Field: "items[1].role", Code: "invalid", Message: "Must be one of: admin, analyst."}},
			log:     "apperr.invalid_request: The request is not valid.",
		},
		{
			name:    "a structural error with no field to name has no details",
			chain:   func() error { return apperr.New(apperr.InvalidRequest, "The body is not valid JSON.") },
			code:    apperr.InvalidRequest.Code(),
			message: "The body is not valid JSON.",
			log:     "apperr.invalid_request: The body is not valid JSON.",
		},

		// ---- Internal codes: handled inside, mapped at the boundary.
		{
			name: "an internal code renders as its public code, message and details carried over",
			chain: func() error {
				return apperr.NewWith(slugTaken, "", apperr.Violations{{Field: "slug", Code: "taken", Message: "already in use"}})
			},
			code:    validationFailed.Code(),
			message: "This address is already in use.",
			details: apperr.Violations{{Field: "slug", Code: "taken", Message: "already in use"}},
			log:     "host.slug_taken: This address is already in use.",
		},
		{
			name:    "an internal code mapped to internal_server_error shows only the generic message",
			chain:   func() error { return apperr.New(brokenReference, "") },
			code:    apperr.InternalServerError,
			message: generic,
			log:     "host.broken_reference: dashboard refers to a dataset that no longer exists",
		},

		// ---- Unclassified: anything that is not ours.
		{
			name:    "a plain error is internal_server_error; its text stays in the log",
			chain:   func() error { return errDial },
			code:    apperr.InternalServerError,
			message: generic,
			log:     "dial tcp 10.0.3.7:5432: connection refused",
		},
		{
			name:    "a fmt.Errorf wrap is not ours: even over a formal error, the response is internal_server_error",
			chain:   func() error { return fmt.Errorf("load tenant: %w", apperr.New(apperr.NotFound, "No such tenant.")) },
			code:    apperr.InternalServerError,
			message: generic,
			log:     "load tenant: apperr.not_found: No such tenant.",
		},
		{
			name:    "a third-party wrapper is not ours either",
			chain:   func() error { return &thirdPartyError{apperr.New(apperr.NotFound, "")} },
			code:    apperr.InternalServerError,
			message: generic,
			log:     "third party: apperr.not_found: Not found.",
		},
		{
			name: "a join is unsupported, so internal_server_error, whatever it contains",
			chain: func() error {
				return errors.Join(apperr.New(apperr.NotFound, "No such tenant."), errDial)
			},
			code:    apperr.InternalServerError,
			message: generic,
			log:     "apperr.not_found: No such tenant.\ndial tcp 10.0.3.7:5432: connection refused",
		},

		// ---- Encapsulating: hide the cause, show a friendlier error.
		{
			name:    "encapsulating a driver error shows the new message and hides the driver's",
			chain:   func() error { return apperr.Encapsulate(errNoRows, apperr.NotFound, "No such tenant.") },
			code:    apperr.NotFound,
			message: "No such tenant.",
			log:     "apperr.not_found: No such tenant.: no rows in result set",
		},
		{
			name: "encapsulating with the default message",
			chain: func() error {
				return apperr.EncapsulateWith(errDial, unavailable, "", retryAfter{Seconds: 5})
			},
			code:    unavailable.Code(),
			message: "The service is temporarily unavailable. Try again shortly.",
			details: retryAfter{Seconds: 5},
			log:     "host.unavailable: The service is temporarily unavailable. Try again shortly.: dial tcp 10.0.3.7:5432: connection refused",
		},
		{
			name: "encapsulating a formal error: the outer one is shown, the inner one and its details are hidden",
			chain: func() error {
				inner := apperr.NewWith(validationFailed, "The address is not valid.",
					apperr.Violations{{Field: "slug", Code: "invalid", Message: "not valid"}})
				return apperr.Encapsulate(inner, apperr.NotFound, "This invitation is no longer valid.")
			},
			code:    apperr.NotFound,
			message: "This invitation is no longer valid.",
			log:     "apperr.not_found: This invitation is no longer valid.: apperr.validation_failed: The address is not valid.",
		},

		// ---- Context: the chain from the top down to the formal error.
		{
			name: "public steps above the formal error become context, outermost first; private steps only log",
			chain: func() error {
				err := apperr.New(apperr.NotFound, "No such dataset.")
				err = step(err, validateDataset, "name", "orders")
				err = step(err, checkReferences)
				return step(err, importDashboard, "name", "Sales")
			},
			code:    apperr.NotFound,
			message: "No such dataset.",
			context: []apperr.ContextEntry{
				{Step: "host.import_dashboard", Params: map[string]any{"name": "Sales"}, Text: "import dashboard 'Sales'"},
				{Step: "host.validate_dataset", Params: map[string]any{"name": "orders"}, Text: "validate dataset 'orders'"},
			},
			log: "host.import_dashboard(name=Sales): host.check_references: host.validate_dataset(name=orders): apperr.not_found: No such dataset.",
		},
		{
			name: "context shows over an internal_server_error: it says what was being done, which stays true",
			chain: func() error {
				return step(errDial, importDashboard, "name", "Sales")
			},
			code:    apperr.InternalServerError,
			message: generic,
			context: []apperr.ContextEntry{
				{Step: "host.import_dashboard", Params: map[string]any{"name": "Sales"}, Text: "import dashboard 'Sales'"},
			},
			log: "host.import_dashboard(name=Sales): apperr.internal_server_error: unclassified: dial tcp 10.0.3.7:5432: connection refused",
		},
		{
			name: "steps beneath an encapsulation are hidden with the inner chain",
			chain: func() error {
				err := step(errNoRows, loadDataset, "name", "orders")
				err = apperr.Encapsulate(err, apperr.NotFound, "No such dataset.")
				return step(err, importDashboard, "name", "Sales")
			},
			code:    apperr.NotFound,
			message: "No such dataset.",
			context: []apperr.ContextEntry{
				{Step: "host.import_dashboard", Params: map[string]any{"name": "Sales"}, Text: "import dashboard 'Sales'"},
			},
			log: "host.import_dashboard(name=Sales): apperr.not_found: No such dataset.: host.load_dataset(name=orders): apperr.internal_server_error: unclassified: no rows in result set",
		},
		{
			name: "a step over a foreign wrap formalises it as internal_server_error, keeping its own context",
			chain: func() error {
				err := step(apperr.New(apperr.NotFound, ""), loadDataset, "name", "orders")
				err = fmt.Errorf("resolve: %w", err)
				return step(err, importDashboard, "name", "Sales")
			},
			code:    apperr.InternalServerError,
			message: generic,
			context: []apperr.ContextEntry{
				{Step: "host.import_dashboard", Params: map[string]any{"name": "Sales"}, Text: "import dashboard 'Sales'"},
			},
			log: "host.import_dashboard(name=Sales): apperr.internal_server_error: unclassified: resolve: host.load_dataset(name=orders): apperr.not_found: Not found.",
		},
		{
			name: "a sensitive parameter is redacted in the log and shown in context",
			chain: func() error {
				return step(apperr.New(apperr.NotFound, "No such tenant."), invite, "email", "carol@example.com")
			},
			code:    apperr.NotFound,
			message: "No such tenant.",
			context: []apperr.ContextEntry{
				{Step: "host.invite", Params: map[string]any{"email": "carol@example.com"}, Text: "invite 'carol@example.com'"},
			},
			log: "host.invite(email=[redacted]): apperr.not_found: No such tenant.",
		},
		{
			name: "a public step naming a parameter it was not given is left out, never shown half-filled",
			chain: func() error {
				return step(apperr.New(apperr.NotFound, ""), importDashboard)
			},
			code:    apperr.NotFound,
			message: "Not found.",
			log:     "host.import_dashboard: apperr.not_found: Not found.",
		},
		{
			name: "only the parameters the template names reach the context",
			chain: func() error {
				return step(apperr.New(apperr.NotFound, ""),
					connectDatasource, "name", "warehouse", "host", "db.internal")
			},
			code:    apperr.NotFound,
			message: "Not found.",
			context: []apperr.ContextEntry{
				{Step: "host.connect_datasource", Params: map[string]any{"name": "warehouse"}, Text: "connect to data source 'warehouse'"},
			},
			log: "host.connect_datasource(name=warehouse, host=db.internal): apperr.not_found: Not found.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.chain()

			got := apperr.From(err).Response("req_1")
			want := apperr.Response{
				Code:      tc.code.String(),
				Scope:     tc.code.Scope(),
				Context:   tc.context,
				Message:   tc.message,
				Details:   tc.details,
				RequestID: "req_1",
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("client receives\n  %+v\nwant\n  %+v", got, want)
			}
			if log := err.Error(); log != tc.log {
				t.Errorf("log =\n  %s\nwant\n  %s", log, tc.log)
			}
		})
	}
}

// errors.Is answers "caused by": a code anywhere in the chain, through steps
// and encapsulation, matched by the code itself or by the public code an
// internal code maps to.
func TestIsAnswersCausedBy(t *testing.T) {
	inner := apperr.NewWith(slugTaken, "", apperr.Violations{})
	err := step(apperr.Encapsulate(inner, apperr.InternalServerError, "setup found a leftover tenant"), setup)

	for _, c := range []apperr.AnyCode{apperr.InternalServerError, slugTaken, validationFailed} {
		if !errors.Is(err, c) {
			t.Errorf("errors.Is(err, %s) = false; the chain was caused by it", c)
		}
	}
	if errors.Is(err, apperr.NotFound) {
		t.Error("errors.Is matched a code that is nowhere in the chain")
	}
	if !errors.Is(step(apperr.NotFound, anyStep), apperr.NotFound) {
		t.Error("errors.Is does not match a bare code")
	}
	if !errors.Is(apperr.Encapsulate(errNoRows, apperr.NotFound, ""), errNoRows) {
		t.Error("errors.Is cannot see a plain cause through a formal error")
	}
}

// errors.As on *Error answers "what is it": the outermost formal error.
func TestAsAnswersWhatItIs(t *testing.T) {
	inner := apperr.New(apperr.NotFound, "No such dataset.")
	err := step(apperr.Encapsulate(inner, brokenReference, ""), renderDashboard)

	got, ok := errors.AsType[*apperr.Error](err)
	if !ok || got.Code != brokenReference {
		t.Fatalf("errors.As found %v, want the outer broken_reference", got)
	}

	bare, ok := errors.AsType[*apperr.Error](step(forbidden, anyStep))
	if !ok || bare.Code != forbidden || bare.Message != "You do not have permission to do this." {
		t.Errorf("errors.As on a bare code found %+v, want forbidden with its default message", bare)
	}

	// Documented divergence: errors.As looks through a foreign wrap, the
	// response does not. The linter keeps such wraps out of our code.
	wrapped := fmt.Errorf("load: %w", inner)
	if got, ok := errors.AsType[*apperr.Error](wrapped); !ok || got.Code != apperr.NotFound {
		t.Error("errors.As should still find the formal error beneath a fmt.Errorf wrap")
	}
	if apperr.From(wrapped).Code != apperr.InternalServerError {
		t.Error("the response should be internal beneath a fmt.Errorf wrap")
	}

	// Once a step passes over the foreign wrap, it is formalised, and "what is
	// it" agrees with the response again.
	stepped := step(wrapped, resolve)
	if got, ok := errors.AsType[*apperr.Error](stepped); !ok || got.Code != apperr.InternalServerError {
		t.Errorf("errors.As after a step over a foreign wrap found %v, want internal", got)
	}
}

// Nothing below the formal error reaches the wire, whatever the chain.
func TestResponseCarriesOnlyWhatWasWrittenForTheClient(t *testing.T) {
	err := step(
		apperr.EncapsulateWith(errDial, unavailable, "Try again shortly.", retryAfter{Seconds: 5}),
		connectDatasource, "name", "warehouse", "host", "10.0.3.7")

	b, jerr := json.Marshal(apperr.Envelope{Error: apperr.From(err).Response("req_1")})
	if jerr != nil {
		t.Fatal(jerr)
	}
	want := `{"error":{"code":"unavailable","scope":"server",` +
		`"context":[{"step":"host.connect_datasource","params":{"name":"warehouse"},"text":"connect to data source 'warehouse'"}],` +
		`"message":"Try again shortly.","details":{"retry_after_seconds":5},"request_id":"req_1"}}`
	if string(b) != want {
		t.Errorf("body =\n  %s\nwant\n  %s", b, want)
	}
	for _, leak := range []string{"10.0.3.7", "dial", "refused"} {
		if strings.Contains(string(b), leak) {
			t.Errorf("body contains %q", leak)
		}
	}
}

// From returns a copy: other code may hold the formal error it found.
// WithStep copies: the formal error it was given may still be held by other
// code, and is never mutated.
func TestWithStepDoesNotMutate(t *testing.T) {
	err := apperr.New(apperr.NotFound, "")
	original, _ := errors.AsType[*apperr.Error](err)

	first := step(err, load)
	second := step(first, invite)

	if len(original.Steps) != 0 {
		t.Errorf("WithStep wrote onto the original: %v", original.Steps)
	}
	if got := apperr.From(first).Steps; len(got) != 1 {
		t.Errorf("an earlier copy changed when a later step was added: %v", got)
	}
	if got := apperr.From(second).Steps; len(got) != 2 || got[0].String() != "host.invite" {
		t.Errorf("steps = %v, want [invite load]", got)
	}
}

func TestNil(t *testing.T) {
	if apperr.From(nil) != nil {
		t.Error("From(nil) is not nil")
	}
	if apperr.WithStep(nil, appstep.NewStep(anyStep)) != nil {
		t.Error("adding a step to a nil error made it non-nil")
	}
}

// Public codes are complete. Internal codes are not part of the public catalog.
// Whether a status agrees with a scope is the HTTP transport's test: it is the
// one that assigns statuses.
func TestCatalog(t *testing.T) {
	for _, ns := range []apperr.Namespace{apperr.Generic, host} {
		for _, c := range apperr.Codes(ns) {
			if !c.IsPublic() || c.Namespace() != ns {
				t.Errorf("%s is internal or not of %s, but listed", c.Qualified(), ns)
			}
			if c.Scope() == "" || c.Message() == "" {
				t.Errorf("incomplete public code: %s", c)
			}
		}
	}
	if got := apperr.Codes(apperr.Generic); len(got) != 4 {
		t.Errorf("generic codes = %v, want internal_server_error, invalid_request, not_found, validation_failed", got)
	}
	for _, c := range apperr.Codes(host) {
		if c == slugTaken.Code() {
			t.Error("an internal code is listed as public")
		}
	}
	if slugTaken.IsPublic() || slugTaken.Public() != validationFailed.Code() {
		t.Error("an internal code should map to its public code")
	}
}

// Code() reaches the code underneath a TypedCode, which is otherwise unreachable
// from outside the package, and is the identity on a Code.
func TestCodeAccessor(t *testing.T) {
	if apperr.NotFound.Code() != apperr.NotFound {
		t.Error("Code() on a Code is not the identity")
	}
	e, ok := errors.AsType[*apperr.Error](apperr.NewWith(validationFailed, "", apperr.Violations{}))
	if !ok || e.Code != validationFailed.Code() {
		t.Errorf("an error made from a TypedCode holds %v, want its Code()", e.Code)
	}
	if slugTaken.Code().Public() != validationFailed.Code() {
		t.Error("an internal TypedCode's Code() does not map to its public code")
	}
	// Both kinds satisfy AnyCode, which is what lets callers hold them together.
	for _, c := range []apperr.AnyCode{apperr.NotFound, apperr.InvalidRequest, slugTaken} {
		if c.Code().String() == "" {
			t.Errorf("%v: empty Code()", c)
		}
	}
}

func TestDefiningCodesRefusesMistakes(t *testing.T) {
	for name, define := range map[string]func(){
		"a duplicate name":            func() { _ = apperr.DefineInternalCode(host, "slug_taken", apperr.NotFound, "x") },
		"mapping to an internal code": func() { _ = apperr.DefineInternalCode(host, "nested", slugTaken.Code(), "x") },
		"no default message":          func() { _ = apperr.DefineInternalCode(host, "silent", apperr.NotFound, "") },
		"no scope":                    func() { _ = apperr.DefinePublicCode(host, "scopeless", "", "x") },
		"no namespace":                func() { _ = apperr.DefinePublicCode(apperr.Namespace{}, "orphan", apperr.User, "x") },
		"a generic code's name":       func() { _ = apperr.DefinePublicCode(host, "not_found", apperr.User, "x") },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("defining a code with %s did not panic", name)
				}
			}()
			define()
		})
	}
}

// The catalog records each typed code's details type, so the published contract
// types them; an internal code reports its public code's.
func TestDetailsType(t *testing.T) {
	for _, tc := range []struct {
		code apperr.Code
		want reflect.Type
	}{
		{validationFailed.Code(), reflect.TypeFor[apperr.Violations]()},
		{apperr.InvalidRequest.Code(), reflect.TypeFor[apperr.Violations]()},
		{unavailable.Code(), reflect.TypeFor[retryAfter]()},
		{rateLimited.Code(), reflect.TypeFor[retryAfter]()},
		{slugTaken.Code(), reflect.TypeFor[apperr.Violations]()},
		{apperr.NotFound, nil},
		{brokenReference, nil},
	} {
		if got := tc.code.DetailsType(); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.code, got, tc.want)
		}
	}
}

// A name is unique within its namespace, not across them: a library and its
// host name their codes without knowing each other. A client sees the bare
// name; the log, the qualified one.
func TestNamespaces(t *testing.T) {
	other := apperr.DefineNamespace("other")
	c := apperr.DefinePublicCode(other, "slug_taken", apperr.User, "Taken elsewhere.")
	if c == slugTaken.Code() || c.String() != "slug_taken" || c.Qualified() != "other.slug_taken" {
		t.Errorf("a same-named code of another namespace is %s (%s)", c, c.Qualified())
	}
	if got := apperr.From(c).Response("req_1").Code; got != "slug_taken" {
		t.Errorf("a client sees %q, want the bare name", got)
	}
	if log := c.Error(); log != "other.slug_taken: Taken elsewhere." {
		t.Errorf("log = %q, want the qualified name", log)
	}
	// An internal code may reuse a generic code's name: it never reaches a client.
	if internal := apperr.DefineInternalCode(other, "not_found", apperr.NotFound, "x"); internal.Public() != apperr.NotFound {
		t.Error("an internal code named like a generic one does not map to it")
	}
}
