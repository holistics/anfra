package apikit

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/shared/apptracing"
	"github.com/holistics/anfra/shared/httpkit"
	"github.com/holistics/anfra/shared/requestid"
	"github.com/holistics/anfra/shared/telemetry"
)

// MCP serves a registry's MCP ops as tools over MCP's streamable HTTP: one named
// tool per op that declares MCP — its input schema the one HTTP validates, its
// result the op's output, its failure the same error body HTTP renders — and
// two tools of discovery, list_operations and describe_operation, the same
// levels HTTP serves at /ops.
//
// Each session gets the tools its caller may call, through Request and Visible.
// Stateless hosts rebuild these from every HTTP request, so credentials and
// visibility are never captured across requests. Named tools keep a client's
// per-tool approval and the read-only hints.
type MCP[Call any] struct {
	// Name and Version name the server to clients.
	Name, Version string
	// Stateless resolves the caller and tools for every HTTP request. No MCP
	// session ID conveys authority. The default preserves stateful local serving.
	Stateless bool
	// Codes are what the server serves: a failed tool's body is its code's.
	Codes httpkit.Codes
	// Request reads the caller's credential from each request in stateless mode,
	// or the session-opening request otherwise. Required.
	Request func(r *http.Request) (Call, error)
	// Visible reports which ops the caller may call (HTTP.Visible). Nil: every op.
	Visible func(ctx context.Context, r Call, ops []Meta) ([]bool, error)
	// Errors are the codes the host adds to an op (HTTP.Errors), for
	// describe_operation.
	Errors func(m Meta) []apperr.Code
	// TrustMeta continues the trace a request's _meta carries (SEP-414:
	// traceparent, tracestate), for a server whose callers are trusted, as
	// httpkit.Config.TrustTraceparent says of the request's headers. Otherwise
	// _meta's trace is ignored.
	TrustMeta bool
	// TraceSettings are the settings whose baggage allowlist
	// (telemetry.BaggageIncludeVar) decides which members of a request's
	// _meta.baggage get in, from any caller, as httpkit.Config.TraceSettings
	// does of the request's header.
	TraceSettings telemetry.Settings
	// MetaCaller names the caller a request's _meta gives under the caller's
	// own keys, such as a coding agent's session: recorded on every span and
	// log record of the request, as a host names its caller
	// (telemetry.WithAttributes), so over its baggage, and put in the call's
	// baggage as far as TraceSettings let it in, for the sidecars the op calls
	// (telemetry.WithCallerBaggage). Nil: none.
	MetaCaller func(meta map[string]any) map[string]string
}

// Handler is the MCP endpoint, to mount at /mcp behind the same middleware and
// guards as the HTTP API. Each request is routed (httpkit.Routed) by the
// pattern it was mounted at, /mcp unless a mux says otherwise, so its span is
// named "POST /mcp", and each MCP request in it gets a span of its own
// (traced).
func (m MCP[Call]) Handler(rt *Runtime, reg OpSet[Call]) http.Handler {
	h := m.handler(rt, reg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpkit.Routed(r.Context(), "", mountedAt(r))
		h.ServeHTTP(w, r)
	})
}

// mountedAt is the pattern r was routed by, without its method or host: /mcp
// when no mux routed it.
func mountedAt(r *http.Request) string {
	p := r.Pattern
	if _, path, ok := strings.Cut(p, " "); ok {
		p = path
	}
	if i := strings.IndexByte(p, '/'); i > 0 {
		p = p[i:] // a host's pattern
	}
	if p == "" {
		return "/mcp"
	}
	return p
}

func (m MCP[Call]) handler(rt *Runtime, reg OpSet[Call]) http.Handler {
	if m.Request == nil {
		panic("apikit: MCP.Request is required")
	}
	if m.Stateless {
		// Huma's schema registry is mutable during generation. Populate it
		// before concurrent requests, including discovery of output/events.
		for _, o := range reg.Ops() {
			if !o.Meta().MCP {
				continue
			}
			o.InSchema(rt)
			rt.Schema(o.OutType())
			for _, event := range o.EventTypes() {
				rt.Schema(event)
			}
		}
		type mcpServerContextKey struct{}
		h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return r.Context().Value(mcpServerContextKey{}).(*mcp.Server)
		}, &mcp.StreamableHTTPOptions{Stateless: true})
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req, err := m.Request(r)
			if err != nil {
				httpkit.WriteError(w, r, err)
				return
			}
			s, err := m.server(r.Context(), rt, reg, req)
			if err != nil {
				httpkit.WriteError(w, r, err)
				return
			}
			ctx := context.WithValue(r.Context(), mcpServerContextKey{}, s)
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		req, err := m.Request(r)
		if err != nil {
			return nil // the SDK answers 400: the session never opens
		}
		s, err := m.server(r.Context(), rt, reg, req)
		if err != nil {
			return nil
		}
		return s
	}, nil)
}

// server is the MCP server for one session: the ops its caller may call, and
// discovery over them.
func (m MCP[Call]) server(ctx context.Context, rt *Runtime, reg OpSet[Call], req Call) (*mcp.Server, error) {
	d := Discovery[Call]{Visible: m.Visible, Errors: m.Errors, Codes: m.Codes}
	served := func(meta Meta) bool { return meta.MCP }
	ops, err := d.visible(ctx, reg, req, served)
	if err != nil {
		return nil, err
	}
	s := mcp.NewServer(&mcp.Implementation{Name: m.Name, Version: m.Version}, nil)
	s.AddReceivingMiddleware(m.traced)
	for _, o := range ops {
		meta := o.Meta()
		input, err := rt.Inline(o.InSchema(rt))
		if err != nil {
			return nil, fmt.Errorf("op %q: %w", meta.Name, err)
		}
		if input["type"] == nil {
			input["type"] = "object" // MCP requires an object schema, even for no input
		}
		// The description is the op's doc when it has one, as a CLI's --help
		// shows it: a doc stands alone, opening with what the summary says, so
		// adding the summary would load that sentence twice in every session.
		description := meta.Summary
		if meta.Doc != "" {
			description = meta.Doc
		}
		readOnly, idempotent := meta.ReadOnly, meta.Idempotent
		var destructive *bool // meaningless, so unset, for a read-only op
		if !readOnly {
			destructive = &meta.Destructive
		}
		s.AddTool(&mcp.Tool{
			Name: meta.Name, Description: description, InputSchema: input,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: idempotent, DestructiveHint: destructive},
		}, func(ctx context.Context, call *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			out, err := o.Invoke(ctx, rt, req, call.Params.Arguments)
			if err != nil {
				// A failure goes in the result, so the model sees it and can correct
				// its next call: the body HTTP would answer, byte for byte.
				return m.result(apperr.Envelope{Error: m.Codes.Render(err, requestid.From(ctx))}, true)
			}
			return m.result(out, false)
		})
	}
	m.discoveryTools(s, rt, reg, req, d, served)
	return s, nil
}

// discoveryTools are list_operations and describe_operation: the index or a
// group, and an op's usage, as HTTP's /ops serves them.
func (m MCP[Call]) discoveryTools(s *mcp.Server, rt *Runtime, reg OpSet[Call], req Call, d Discovery[Call], served func(Meta) bool) {
	object := func(props map[string]any, required ...string) map[string]any {
		schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	s.AddTool(&mcp.Tool{
		Name: "list_operations",
		Description: "List the operations you may call: without a group, the groups, each in one line; with a group, " +
			"its operations, each in one line. Then describe_operation for how to call one.",
		InputSchema: object(map[string]any{"group": map[string]any{"type": "string", "description": "a group's name, from the list"}}),
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(ctx context.Context, call *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var in struct {
			Group string `json:"group"`
		}
		if err := args(call, &in); err != nil {
			return m.failed(ctx, err)
		}
		var out any
		var err error
		if in.Group == "" {
			out, err = d.Index(ctx, reg, req, served)
		} else {
			out, err = d.Group(ctx, reg, req, served, in.Group)
		}
		if err != nil {
			return m.failed(ctx, err)
		}
		return m.result(out, false)
	})
	s.AddTool(&mcp.Tool{
		Name:        "describe_operation",
		Description: "How to call an operation: what it is for, its input and output schemas, and the errors it can return.",
		InputSchema: object(map[string]any{"name": map[string]any{"type": "string", "description": "the operation's name"}}, "name"),
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(ctx context.Context, call *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var in struct {
			Name string `json:"name"`
		}
		if err := args(call, &in); err != nil {
			return m.failed(ctx, err)
		}
		out, err := d.Usage(ctx, rt, reg, req, served, in.Name)
		if err != nil {
			return m.failed(ctx, err)
		}
		return m.result(out, false)
	})
}

func args(call *mcp.CallToolRequest, into any) error {
	if len(call.Params.Arguments) == 0 {
		return nil
	}
	if err := jsonkit.Unmarshal(call.Params.Arguments, into); err != nil {
		return apperr.Encapsulate(err, apperr.InvalidRequest, "The arguments are not valid.")
	}
	return nil
}

func (m MCP[Call]) failed(ctx context.Context, err error) (*mcp.CallToolResult, error) {
	return m.result(apperr.Envelope{Error: m.Codes.Render(err, requestid.From(ctx))}, true)
}

// result is v as a tool's result: its JSON as text, for any client, and as
// structured content, for one that reads it.
func (m MCP[Call]) result(v any, isError bool) (*mcp.CallToolResult, error) {
	b, err := jsonkit.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(b)}},
		StructuredContent: jsontext.Value(b),
		IsError:           isError,
	}, nil
}

// traced runs each MCP request in a span of its own, of kind server, as
// OpenTelemetry's MCP conventions name it: "tools/call <tool>" for a tool call,
// the method alone otherwise, such as "tools/list". The request's _meta is
// read first: the caller's trace, which the span goes under when TrustMeta,
// its baggage, and the caller MetaCaller finds there, which the span and
// every span under it, the op's included, record.
//
// A failure sets error.type and an error status: the JSON-RPC error's code and
// message, or tool_error for a tool whose result says it failed, as the
// conventions say. An op's own span, under it, keeps apptracing's rule.
func (m MCP[Call]) traced(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		meta := requestMeta(req)
		ctx = m.callersTrace(ctx, meta)
		if m.MetaCaller != nil && len(meta) > 0 {
			fields := m.MetaCaller(meta)
			ctx = telemetry.WithCallerBaggage(telemetry.WithAttributes(ctx, fields), fields, m.TraceSettings)
		}
		name := method
		kv := []telemetry.KeyValue{telemetry.MCPMethod.Is(method)}
		if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil {
			name += " " + call.Params.Name
			kv = append(kv, telemetry.ToolName.Is(call.Params.Name), telemetry.GenAIOperation.Is("execute_tool"))
		}
		var version string
		if ss, ok := req.GetSession().(*mcp.ServerSession); ok && ss != nil {
			if id := ss.ID(); id != "" {
				kv = append(kv, telemetry.MCPSessionID.Is(id))
			}
			if p := ss.InitializeParams(); p != nil {
				version = p.ProtocolVersion
			}
		}
		if init, ok := req.(*mcp.InitializeRequest); ok && init.Params != nil {
			version = init.Params.ProtocolVersion
		}
		if version != "" {
			kv = append(kv, telemetry.MCPProtocolVersion.Is(version))
		}
		ctx, span := apptracing.StartServer(ctx, name, kv...)
		defer span.End()
		res, err := next(ctx, method, req)
		if err != nil {
			mcpFailed(ctx, span, err)
		} else if r, ok := res.(*mcp.CallToolResult); ok && r != nil && r.IsError {
			apptracing.SetAttributes(ctx, telemetry.ErrorType.Is("tool_error"))
			span.SetStatus(otelcodes.Error, "tool_error")
		}
		return res, err
	}
}

// mcpFailed marks an MCP request's span as failing with err: a JSON-RPC
// error by its code and message, any other by its apperr code.
func mcpFailed(ctx context.Context, span trace.Span, err error) {
	if wire, ok := errors.AsType[*jsonrpc.Error](err); ok {
		apptracing.SetAttributes(ctx, telemetry.ErrorType.Is(strconv.FormatInt(wire.Code, 10)))
		span.SetStatus(otelcodes.Error, wire.Message)
		return
	}
	code := apperr.From(err).Code
	apptracing.SetAttributes(ctx, telemetry.ErrorType.Is(code.String()))
	span.SetStatus(otelcodes.Error, code.Public().String())
}

// requestMeta is req's _meta, nil for a request without params.
func requestMeta(req mcp.Request) map[string]any {
	p := req.GetParams()
	if p == nil || reflect.ValueOf(p).IsNil() {
		return nil
	}
	return p.GetMeta()
}

// callersTrace is ctx with what a request's _meta carries, when it carries
// it: the request's own, which wins over the HTTP request's, since one HTTP
// request can carry another call's. Its baggage, the members TraceSettings allow, from
// any caller; its trace only when TrustMeta.
func (m MCP[Call]) callersTrace(ctx context.Context, meta map[string]any) context.Context {
	if len(meta) == 0 {
		return ctx
	}
	if m.TrustMeta {
		ctx = otel.GetTextMapPropagator().Extract(ctx, metaCarrier(meta))
	} else {
		ctx = propagation.Baggage{}.Extract(ctx, metaCarrier(meta))
	}
	return telemetry.WithAllowedBaggage(ctx, m.TraceSettings)
}

// metaCarrier reads a trace context from a request's _meta: its string
// members, by their lowercase W3C names.
type metaCarrier mcp.Meta

func (c metaCarrier) Get(key string) string {
	v, _ := c[key].(string)
	return v
}

func (metaCarrier) Set(string, string) {}

func (c metaCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
