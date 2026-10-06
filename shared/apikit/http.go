package apikit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/holistics/anfra/shared/apperr"

	"github.com/holistics/anfra/shared/httpkit"
)

// HTTP serves a registry's HTTP ops: every op at POST /api/<name> — or below a
// base the host gives it, /api/o/{org}/<name> — its whole input as the JSON body,
// its output as the body of a 200, an error as its code's body. It translates and
// nothing more: the op layer validates, admits and runs; the host's hooks add
// what is its own.
//
// In the OpenAPI document an op's path is its name alone, /<name>, below the
// server it is served from: /api, or its base. So an op has the same path on
// every host, whatever base one serves it under, and a client takes the base
// once, when it is built.
type HTTP[R any] struct {
	// Codes are what the server serves, with their statuses: an op's error
	// responses, and every error's rendering.
	Codes httpkit.Codes
	// Title and Version are the OpenAPI document's info; Extensions its own
	// extensions (a compatibility number, say).
	Title, Version string
	Extensions     map[string]any

	// Request reads what the op is admitted from: the request's credential, and
	// the base's path values (r.PathValue). It may write to w — clearing a cookie
	// that no longer resolves, say. Required.
	Request func(w http.ResponseWriter, r *http.Request) (R, error)
	// Base is where an op is served below /api, or nil for /api itself.
	Base func(m Meta) *Base
	// Guard returns the check an op's requests pass before the body is read — a
	// compatibility header, a rate limit — or nil for none. It is called once per
	// op, when the handler is built.
	Guard func(m Meta) func(w http.ResponseWriter, r *http.Request) error
	// After sees a successful output before it is written: setting a cookie the
	// op asked for, say.
	After func(w http.ResponseWriter, out any)
	// Headers are the request headers the host reads for an op, published with
	// it.
	Headers func(m Meta) []*huma.Param
	// Errors are the codes the host's admission and Guard can answer an op with,
	// published with the codes it declares, in this order.
	Errors func(m Meta) []apperr.Code
}

// Base is a path below /api that ops are served under, with variables:
// "/o/{org}". A variable is a path wildcard to the router (r.PathValue("org"))
// and a server variable to the OpenAPI document, which requires a default.
type Base struct {
	Path      string
	Variables map[string]*huma.ServerVariable
}

// root is the server every op is below: the document's, and an op's without a
// Base.
const root = "/api"

// Handler is the router with every HTTP op registered. Anything else under /api
// is a JSON not_found, not whatever is mounted beside it: an API typo that
// answers 200 with HTML is much harder to diagnose than one that says so. It has
// no middleware: wrap it in httpkit.Wrap, which renders errors with h.Codes.
func (h HTTP[R]) Handler(rt *Runtime, reg *Registry[R]) http.Handler {
	if h.Request == nil {
		// A transport that cannot tell who is asking would admit everyone alike;
		// refusing to start is the only safe answer.
		panic("apikit: HTTP.Request is required")
	}
	_, mux := h.api(rt, reg)
	mux.HandleFunc(root+"/", func(w http.ResponseWriter, r *http.Request) {
		httpkit.WriteError(w, r, apperr.New(apperr.NotFound, "No such endpoint."))
	})
	return mux
}

// Spec is the OpenAPI document for reg's HTTP ops, in YAML, built without a
// server: what the server publishes, for a committed snapshot and generated
// clients.
func (h HTTP[R]) Spec(rt *Runtime, reg *Registry[R]) ([]byte, error) {
	api, _ := h.api(rt, reg)
	return api.OpenAPI().YAML()
}

func (h HTTP[R]) api(rt *Runtime, reg *Registry[R]) (huma.API, *http.ServeMux) {
	mux := http.NewServeMux()
	cfg := huma.DefaultConfig(h.Title, h.Version)
	cfg.OpenAPI.Components.Schemas = rt.Schemas // validated and published alike
	cfg.OpenAPIPath = root + "/openapi"         // served as /api/openapi.json and .yaml
	cfg.Servers = []*huma.Server{{URL: root}}
	cfg.DocsPath = ""
	cfg.SchemasPath = ""
	cfg.CreateHooks = nil // no $schema links in bodies: they are the ops' types, as declared
	cfg.OpenAPI.Extensions = h.Extensions
	api := humago.New(mux, cfg)

	for _, o := range reg.Ops() {
		if o.Meta().HTTP {
			h.register(api, mux, rt, o)
		}
	}
	return api, mux
}

// register serves o at POST <server>/<name>, and refuses other methods there
// with the error body rather than the standard library's plain text. The route
// is the router's; huma only documents it.
func (h HTTP[R]) register(api huma.API, mux *http.ServeMux, rt *Runtime, o Op[R]) {
	m := o.Meta()
	server := root
	var base *Base
	if h.Base != nil {
		base = h.Base(m)
	}
	if base != nil {
		server += base.Path
	}
	api.OpenAPI().AddOperation(h.operation(rt, o, base))

	route := server + "/" + m.Name
	var guard func(http.ResponseWriter, *http.Request) error
	if h.Guard != nil {
		guard = h.Guard(m)
	}
	mux.HandleFunc(http.MethodPost+" "+route, func(w http.ResponseWriter, r *http.Request) {
		h.serve(w, r, rt, route, guard, o)
	})
	mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
		httpkit.WriteError(w, r, apperr.New(apperr.InvalidRequest, "Every operation is a POST: POST "+route+"."))
	})
}

// serve runs one request through the op layer. route is its pattern, with the
// base's variables unfilled, so a span name and a metric label stay one per op.
func (h HTTP[R]) serve(w http.ResponseWriter, r *http.Request, rt *Runtime, route string, guard func(http.ResponseWriter, *http.Request) error, o Op[R]) {
	httpkit.Routed(r.Context(), o.Meta().Name, route)

	if guard != nil {
		if err := guard(w, r); err != nil {
			httpkit.WriteError(w, r, err)
			return
		}
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			httpkit.WriteError(w, r, apperr.Encapsulate(err, apperr.InvalidRequest, "The body is larger than the limit."))
			return
		}
		httpkit.WriteError(w, r, apperr.Encapsulate(err, apperr.InvalidRequest, "The body could not be read."))
		return
	}
	req, err := h.Request(w, r)
	if err != nil {
		httpkit.WriteError(w, r, err)
		return
	}

	out, err := o.Invoke(r.Context(), rt, req, raw)
	if err != nil {
		if errors.Is(r.Context().Err(), context.Canceled) {
			// The client is gone: nobody reads a response. The log line still says so.
			httpkit.Canceled(r.Context())
			return
		}
		httpkit.WriteError(w, r, err)
		return
	}
	if h.After != nil {
		h.After(w, out)
	}
	httpkit.WriteJSON(w, http.StatusOK, out)
}

// operation is o's OpenAPI operation: POST /<name> below its server, its input
// schema as the request body, its output's as the 200 response, and one error
// response per status among the codes it can return, all the same envelope.
func (h HTTP[R]) operation(rt *Runtime, o Op[R], base *Base) *huma.Operation {
	m := o.Meta()
	operation := &huma.Operation{
		OperationID: m.Name,
		Method:      http.MethodPost,
		Path:        "/" + m.Name,
		Summary:     m.Summary,
		Description: m.Doc,
		RequestBody: &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{
			"application/json": {Schema: o.InSchema(rt)},
		}},
		Responses: map[string]*huma.Response{
			"200": {Description: "OK", Content: map[string]*huma.MediaType{
				"application/json": {Schema: rt.Schema(o.OutType())},
			}},
		},
		Extensions: map[string]any{"x-read-only": m.ReadOnly, "x-idempotent": m.Idempotent},
	}
	if base != nil {
		operation.Servers = []*huma.Server{{URL: root + base.Path, Variables: base.Variables}}
	}
	if h.Headers != nil {
		operation.Parameters = h.Headers(m)
	}
	codes := []apperr.Code{apperr.InvalidRequest.Code(), apperr.InternalServerError}
	if h.Errors != nil {
		for _, c := range h.Errors(m) {
			if !slices.Contains(codes, c) {
				codes = append(codes, c)
			}
		}
	}
	for _, c := range m.Errors {
		if p := h.Codes.Served(c); !slices.Contains(codes, p) {
			codes = append(codes, p)
		}
	}

	// One response per status, its body the envelope around the union of the codes
	// the op can return with that status: a client switching on error.code gets
	// each code's details typed.
	byStatus := map[int][]apperr.Code{}
	for _, c := range codes {
		byStatus[h.Codes.StatusOf(c)] = append(byStatus[h.Codes.StatusOf(c)], c)
	}
	for status, codes := range byStatus {
		names := make([]string, len(codes))
		bodies := make([]*huma.Schema, len(codes))
		for i, c := range codes {
			names[i] = c.String()
			bodies[i] = &huma.Schema{Ref: h.errorBodyRef(rt.Schemas, c)}
		}
		body := bodies[0]
		if len(bodies) > 1 {
			body = &huma.Schema{OneOf: bodies}
		}
		operation.Responses[strconv.Itoa(status)] = &huma.Response{
			Description: strings.Join(names, ", "),
			Content: map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{
				Type:       "object",
				Properties: map[string]*huma.Schema{"error": body},
				Required:   []string{"error"},
			}}},
		}
	}
	return operation
}

// errorBodyRef is the reference to code's error body schema, which it adds to
// the registry on first use: the error envelope's body with code, scope and
// status fixed to the code's own, and details typed by the code's details type.
// A code with no details has none in its schema.
func (h HTTP[R]) errorBodyRef(reg huma.Registry, c apperr.Code) string {
	name := "ErrorBody" + pascal(c.String())
	ref := "#/components/schemas/" + name
	if _, ok := reg.Map()[name]; ok {
		return ref
	}
	props := map[string]*huma.Schema{
		"code":       {Type: "string", Const: c.String()},
		"scope":      {Type: "string", Const: string(c.Scope())},
		"status":     {Type: "integer", Const: h.Codes.StatusOf(c)},
		"context":    {Type: "array", Items: reg.Schema(reflect.TypeFor[apperr.ContextEntry](), true, "")},
		"message":    {Type: "string"},
		"request_id": {Type: "string"},
	}
	if t := c.DetailsType(); t != nil {
		props["details"] = reg.Schema(t, true, "")
	}
	reg.Map()[name] = &huma.Schema{
		Type:                 "object",
		Description:          c.Message(),
		Properties:           props,
		Required:             []string{"code", "scope", "status", "message", "request_id"},
		AdditionalProperties: false,
	}
	return ref
}

// pascal turns a snake_case code into PascalCase: validation_failed is
// ValidationFailed.
func pascal(snake string) string {
	var b strings.Builder
	for _, part := range strings.Split(snake, "_") {
		if part != "" {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}
