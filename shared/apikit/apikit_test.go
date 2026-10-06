package apikit_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/shared/apikit"
)

// A stand-in host: a request carries a user name, and the handler's call
// context is that name. Only "ann" is admitted; "bob" is admitted but may not
// greet anyone named "Zed".

var (
	hostNS    = apperr.DefineNamespace("test_host")
	noEntry   = apperr.DefinePublicCode(hostNS, "no_entry", apperr.User, "Not admitted.")
	notAllow  = apperr.DefinePublicCode(hostNS, "not_allowed", apperr.User, "Not allowed.")
	tooSlow   = apperr.DefinePublicCode(hostNS, "too_slow", apperr.Server, "Too slow.")
	undeclare = apperr.DefinePublicCode(hostNS, "undeclared", apperr.User, "Undeclared.")
)

type request struct{ user string }

var admission = apikit.Admission[request, string]{
	Admit: func(_ context.Context, r request) (string, error) {
		if r.user == "" {
			return "", noEntry
		}
		return r.user, nil
	},
	Authorize: func(_ context.Context, _ request, user string, in any) error {
		if g, ok := in.(greetIn); ok && user == "bob" && g.Name == "Zed" {
			return notAllow
		}
		return nil
	},
}

type greetIn struct {
	Name string `json:"name" minLength:"1"`
}

type greetOut struct {
	Text string   `json:"text"`
	Tags []string `json:"tags"`
}

func greet(name string) *apikit.Def[string, greetIn, greetOut] {
	return &apikit.Def[string, greetIn, greetOut]{
		Name: name, Summary: "Greet someone.", HTTP: true, Errors: []apperr.AnyCode{notAllow},
		Handle: func(_ context.Context, user string, in greetIn) (greetOut, error) {
			return greetOut{Text: user + " greets " + in.Name, Tags: []string{}}, nil
		},
	}
}

func newRegistry() *apikit.Registry[request] {
	return apikit.NewRegistry(apikit.RegistryConfig[request]{
		Namespace:  hostNS,
		StepParams: func(r request) []any { return []any{"user", r.user} },
	})
}

func runtime() *apikit.Runtime {
	rt := apikit.NewRuntime()
	rt.Strict = true
	rt.Implied = []apperr.Code{noEntry}
	rt.Timeout = func(err error) error { return apperr.Encapsulate(err, tooSlow, "") }
	return rt
}

func one[In, Out any](t *testing.T, d *apikit.Def[string, In, Out]) apikit.Op[request] {
	t.Helper()
	reg := newRegistry()
	apikit.Register(reg, admission, d)
	o, _ := reg.Lookup(d.Name)
	return o
}

func code(err error) string { return apperr.From(err).Code.Public().String() }

func mustPanic(t *testing.T, contains string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Errorf("did not panic (want %q)", contains)
			return
		}
		if !strings.Contains(fmt.Sprint(r), contains) {
			t.Errorf("panic %q does not mention %q", r, contains)
		}
	}()
	f()
}

func TestRegisterRefusesMalformedDeclarations(t *testing.T) {
	handle := func(context.Context, string, struct{}) (struct{}, error) { return struct{}{}, nil }
	for want, d := range map[string]*apikit.Def[string, struct{}, struct{}]{
		"dotted":   {Name: "undotted", Summary: "x", Handle: handle},
		"summary":  {Name: "a.b", Handle: handle},
		"negative": {Name: "a.b", Summary: "x", Handle: handle, Timeout: -1},
	} {
		mustPanic(t, want, func() { apikit.Register(newRegistry(), admission, d) })
	}
	mustPanic(t, "admission", func() {
		apikit.Register(newRegistry(), apikit.Admission[request, string]{}, &apikit.Def[string, struct{}, struct{}]{Name: "a.b", Summary: "x", Handle: handle})
	})
	mustPanic(t, "anonymous", func() {
		apikit.Register(newRegistry(), admission, &apikit.Def[string, struct{ A int }, struct{}]{Name: "a.b", Summary: "x",
			Handle: func(context.Context, string, struct{ A int }) (struct{}, error) { return struct{}{}, nil }})
	})
	reg := newRegistry()
	apikit.Register(reg, admission, greet("greetings.create"))
	mustPanic(t, "twice", func() { apikit.Register(reg, admission, greet("greetings.create")) })
}

// The registered op is a copy: editing the declaration afterwards changes nothing.
func TestRegisterFreezesTheDeclaration(t *testing.T) {
	d := greet("greetings.create")
	o := one(t, d)
	d.Summary, d.Errors[0] = "changed", undeclare
	if m := o.Meta(); m.Summary != "Greet someone." || m.Errors[0] != notAllow {
		t.Errorf("the edit reached the registered op: %+v", m)
	}
}

// Admission comes before the input is read; authorization sees the decoded
// input; every call is a private step in the host's namespace.
func TestInvokeOrder(t *testing.T) {
	o := one(t, greet("greetings.create"))
	ctx := context.Background()
	if _, err := o.Invoke(ctx, runtime(), request{}, []byte(`{"name":`)); code(err) != "no_entry" {
		t.Errorf("unadmitted with a bad body: got %v, want no_entry", err)
	}
	if _, err := o.Invoke(ctx, runtime(), request{user: "bob"}, []byte(`{"name":""}`)); code(err) != "invalid_request" {
		t.Errorf("admitted, bad input: got %v, want invalid_request", err)
	}
	_, err := o.Invoke(ctx, runtime(), request{user: "bob"}, []byte(`{"name":"Zed"}`))
	if code(err) != "not_allowed" {
		t.Fatalf("not authorized: got %v", err)
	}
	if !strings.HasPrefix(err.Error(), "test_host.op.greetings.create(user=bob): ") {
		t.Errorf("the op step is missing from the log form: %q", err.Error())
	}
	out, err := o.Invoke(ctx, runtime(), request{user: "ann"}, []byte(`{"name":"Zed"}`))
	if err != nil || out.(greetOut).Text != "ann greets Zed" {
		t.Errorf("got %v, %v", out, err)
	}
}

func TestStrict(t *testing.T) {
	fails := func(err error) *apikit.Def[string, struct{}, greetOut] {
		return &apikit.Def[string, struct{}, greetOut]{Name: "strict.op", Summary: "x",
			Handle: func(context.Context, string, struct{}) (greetOut, error) { return greetOut{Tags: []string{}}, err }}
	}
	ann := request{user: "ann"}
	mustPanic(t, "undeclared", func() { _, _ = one(t, fails(undeclare)).Invoke(context.Background(), runtime(), ann, nil) })
	if _, err := one(t, fails(noEntry)).Invoke(context.Background(), runtime(), ann, nil); code(err) != "no_entry" {
		t.Errorf("a code the host implies: got %v", err)
	}
	nilSlice := &apikit.Def[string, struct{}, greetOut]{Name: "strict.nil", Summary: "x",
		Handle: func(context.Context, string, struct{}) (greetOut, error) { return greetOut{}, nil }}
	mustPanic(t, "tags", func() { _, _ = one(t, nilSlice).Invoke(context.Background(), runtime(), ann, nil) })
}

// Running out of the op's own time is the host's to name; the caller going away
// is not.
func TestTimeout(t *testing.T) {
	slow := one(t, &apikit.Def[string, struct{}, struct{}]{Name: "slow.wait", Summary: "x", Timeout: 10 * time.Millisecond,
		Errors: []apperr.AnyCode{tooSlow},
		Handle: func(ctx context.Context, _ string, _ struct{}) (struct{}, error) {
			<-ctx.Done()
			return struct{}{}, ctx.Err()
		}})
	if _, err := slow.Invoke(context.Background(), runtime(), request{user: "ann"}, nil); code(err) != "too_slow" {
		t.Errorf("own deadline: got %v, want too_slow", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := slow.Invoke(ctx, runtime(), request{user: "ann"}, nil); !errors.Is(err, context.Canceled) || code(err) == "too_slow" {
		t.Errorf("caller gone: got %v, want the cancellation", err)
	}
}

// The HTTP adapter, with every hook.

var codes = httpkit.Codes{
	Namespaces: []apperr.Namespace{hostNS},
	Status: map[apperr.Code]int{
		noEntry: http.StatusUnauthorized, notAllow: http.StatusForbidden,
		tooSlow: http.StatusServiceUnavailable, undeclare: http.StatusConflict,
	},
}

func adapter() apikit.HTTP[request] {
	return apikit.HTTP[request]{
		Codes: codes, Title: "test", Version: "1", Extensions: map[string]any{"x-test": 1},
		Request: func(_ http.ResponseWriter, r *http.Request) (request, error) {
			return request{user: r.Header.Get("User") + r.PathValue("team")}, nil
		},
		Base: func(m apikit.Meta) *apikit.Base {
			if m.Name != "teams.greet" {
				return nil
			}
			return &apikit.Base{Path: "/t/{team}", Variables: map[string]*huma.ServerVariable{
				"team": {Default: "red", Description: "The team."},
			}}
		},
		Guard: func(apikit.Meta) func(http.ResponseWriter, *http.Request) error {
			return func(_ http.ResponseWriter, r *http.Request) error {
				if r.Header.Get("Blocked") != "" {
					return apperr.New(apperr.InvalidRequest, "Blocked.")
				}
				return nil
			}
		},
		After: func(w http.ResponseWriter, _ any) { w.Header().Set("After", "yes") },
		Headers: func(apikit.Meta) []*huma.Param {
			return []*huma.Param{{Name: "User", In: "header", Schema: &huma.Schema{Type: "string"}}}
		},
		Errors: func(apikit.Meta) []apperr.Code { return []apperr.Code{noEntry} },
	}
}

// registry serves greetings.create at /api, and teams.greet below /api/t/{team};
// the team joins the user's name, so the handler shows it was read.
func registry() *apikit.Registry[request] {
	reg := newRegistry()
	apikit.Register(reg, admission, greet("greetings.create"))
	apikit.Register(reg, admission, greet("teams.greet"))
	return reg
}

func server(t *testing.T) http.Handler {
	t.Helper()
	reg := registry()
	return httpkit.Wrap(adapter().Handler(runtime(), reg), httpkit.Config{Codes: codes})
}

func do(h http.Handler, method, path, user, body string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("User", user)
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestHTTP(t *testing.T) {
	h := server(t)
	for _, tc := range []struct {
		name, method, path, user, body string
		headers                        []string
		status                         int
		contains                       string
	}{
		{"served", http.MethodPost, "/api/greetings.create", "ann", `{"name":"Zed"}`, nil, 200, `"ann greets Zed"`},
		{"not admitted", http.MethodPost, "/api/greetings.create", "", `{"name":"Zed"}`, nil, 401, `"no_entry"`},
		{"not authorized", http.MethodPost, "/api/greetings.create", "bob", `{"name":"Zed"}`, nil, 403, `"not_allowed"`},
		{"guarded", http.MethodPost, "/api/greetings.create", "ann", `{"name":"Zed"}`, []string{"Blocked", "1"}, 400, `"Blocked."`},
		{"not a POST", http.MethodGet, "/api/greetings.create", "ann", ``, nil, 400, `Every operation is a POST`},
		{"no such op", http.MethodPost, "/api/greetings.nope", "ann", `{}`, nil, 404, `"not_found"`},
		{"below a base", http.MethodPost, "/api/t/blue/teams.greet", "ann-", `{"name":"Zed"}`, nil, 200, `"ann-blue greets Zed"`},
		{"not at its base", http.MethodPost, "/api/teams.greet", "ann", `{"name":"Zed"}`, nil, 404, `"not_found"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := do(h, tc.method, tc.path, tc.user, tc.body, tc.headers...)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
				t.Errorf("got %d %s, want %d containing %s", w.Code, w.Body, tc.status, tc.contains)
			}
			if (w.Code == 200) != (w.Header().Get("After") == "yes") {
				t.Errorf("After ran: %q, on status %d", w.Header().Get("After"), w.Code)
			}
		})
	}
}

// The document: the op's input and output, the host's parameters and
// extensions, and one response per status, each code's body typed. An op's path
// is its name below its server: /api, or its base with its variables.
func TestSpec(t *testing.T) {
	raw, err := adapter().Spec(runtime(), registry())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Extensions int                    `yaml:"x-test"`
		Servers    []struct{ URL string } `yaml:"servers"`
		Paths      map[string]map[string]struct {
			Servers []struct {
				URL       string                       `yaml:"url"`
				Variables map[string]map[string]string `yaml:"variables"`
			} `yaml:"servers"`
			Parameters []struct{ Name string } `yaml:"parameters"`
			Responses  map[string]struct {
				Description string `yaml:"description"`
			} `yaml:"responses"`
		} `yaml:"paths"`
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Servers) != 1 || doc.Servers[0].URL != "/api" {
		t.Errorf("servers = %+v, want /api", doc.Servers)
	}
	post := doc.Paths["/greetings.create"]["post"]
	if len(post.Servers) != 0 {
		t.Errorf("an op at /api has servers of its own: %+v", post.Servers)
	}
	team := doc.Paths["/teams.greet"]["post"]
	if len(team.Servers) != 1 || team.Servers[0].URL != "/api/t/{team}" || team.Servers[0].Variables["team"]["default"] != "red" {
		t.Errorf("a based op's server = %+v, want /api/t/{team} with its variable", team.Servers)
	}
	got := map[string]string{}
	for status, r := range post.Responses {
		got[status] = r.Description
	}
	want := map[string]string{"200": "OK", "400": "invalid_request", "401": "no_entry", "403": "not_allowed", "500": "internal_server_error"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("responses = %v, want %v", got, want)
	}
	if doc.Extensions != 1 || len(post.Parameters) != 1 || post.Parameters[0].Name != "User" {
		t.Errorf("the host's extensions or parameters are missing: %s", raw)
	}
	for _, s := range []string{"GreetIn", "GreetOut", "ErrorBodyNoEntry", "ErrorBodyNotAllowed", "ErrorBodyInvalidRequest"} {
		if doc.Components.Schemas[s] == nil {
			t.Errorf("schema %s is missing", s)
		}
	}
}
