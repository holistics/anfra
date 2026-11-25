package apikit_test

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/httpkit"
)

// The trace an MCP client passes: its tool call's span.
const (
	callerTrace       = "920f174f97fbdcc18b7dd6f7d10bd48b"
	callerSpan        = "79cbcb6bcdf1bccf"
	callerTraceparent = "00-" + callerTrace + "-" + callerSpan + "-01"
)

// traceparentHeader sends the caller's trace on each HTTP request, as an HTTP
// MCP client that traces itself does.
type traceparentHeader struct{}

func (traceparentHeader) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User", "ann")
	r.Header.Set("traceparent", callerTraceparent)
	return http.DefaultTransport.RoundTrip(r)
}

// An MCP tool call goes in the trace its caller passes, in the HTTP request's
// traceparent header or in the call's _meta (SEP-414), and is named by the
// session its _meta.baggage gives. An untrusted caller's _meta starts no trace
// of its caller's, but its session, an allowed member, still names the call.
// The host's MetaCaller, reading the agent's own key, wins over the baggage.
func TestMCPToolCallFollowsTheCallersTrace(t *testing.T) {
	meta := mcp.Meta{"traceparent": callerTraceparent, "baggage": "gen_ai.conversation.id=conv-1"}
	for _, tc := range []struct {
		name          string
		transport     http.RoundTripper
		meta          mcp.Meta
		trusted       bool
		inTrace       bool
		session       string
		underCallSpan bool // the MCP request's span directly under the caller's span
	}{
		{"traceparent header", traceparentHeader{}, nil, true, true, "", false},
		{"_meta", userHeader("ann"), meta, true, true, "conv-1", true},
		{"_meta, untrusted", userHeader("ann"), meta, false, false, "conv-1", false},
		{"_meta, the agent's own key", userHeader("ann"), mcp.Meta{"baggage": "gen_ai.conversation.id=conv-1", "sessionId": "codex-1"}, false, false, "codex-1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from := record(t)
			prevProp := otel.GetTextMapPropagator()
			otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
			t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

			m := apikit.MCP[request]{
				Name: "test", Version: "1", Codes: codes, TrustMeta: tc.trusted,
				MetaCaller: func(meta map[string]any) map[string]string {
					id, _ := meta["sessionId"].(string)
					return map[string]string{"gen_ai.conversation.id": id}
				},
				Request: func(r *http.Request) (request, error) { return request{user: r.Header.Get("User")}, nil },
			}
			srv := httptest.NewServer(httpkit.Wrap(m.Handler(runtime(), discoverable()), httpkit.Config{TrustTraceparent: tc.trusted}))
			defer srv.Close()
			ctx := context.Background()
			session, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(ctx,
				&mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &http.Client{Transport: tc.transport}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			params := &mcp.CallToolParams{Name: "greetings.create", Arguments: map[string]any{"name": "Zed"}}
			params.Meta = tc.meta
			if _, err := session.CallTool(ctx, params); err != nil {
				t.Fatal(err)
			}
			session.Close()
			srv.Close() // every span of the call has ended

			var op, call sdktrace.ReadOnlySpan
			for _, s := range spanRecorder().Ended()[from:] {
				switch s.Name() {
				case "test_host.op.greetings.create":
					op = s
				case "tools/call greetings.create":
					call = s
				}
			}
			if op == nil || call == nil {
				t.Fatalf("op's span %v, the MCP request's %v: want both", op != nil, call != nil)
			}
			if op.Parent().SpanID() != call.SpanContext().SpanID() {
				t.Errorf("op's parent %s, want the MCP request's span %s", op.Parent().SpanID(), call.SpanContext().SpanID())
			}
			if got := op.SpanContext().TraceID().String() == callerTrace; got != tc.inTrace {
				t.Errorf("op in the caller's trace: %v, want %v", got, tc.inTrace)
			}
			if got := call.Parent().SpanID().String() == callerSpan; got != tc.underCallSpan {
				t.Errorf("MCP request directly under the caller's span: %v (parent %s), want %v", got, call.Parent().SpanID(), tc.underCallSpan)
			}
			var named string
			for _, a := range op.Attributes() {
				if a.Key == "gen_ai.conversation.id" {
					named = a.Value.String()
				}
			}
			if named != tc.session {
				t.Errorf("op's gen_ai.conversation.id %q, want %q", named, tc.session)
			}
		})
	}
}

// Each MCP request gets a span of its own, as OpenTelemetry's MCP conventions
// name it, under the HTTP request's, which is routed as POST /mcp: a tool
// call's names its tool and is an execute_tool operation; a tool whose result
// says it failed is a tool_error; a JSON-RPC error is its code.
func TestEachMCPRequestHasItsOwnSpan(t *testing.T) {
	from := record(t)
	m := apikit.MCP[request]{
		Name: "test", Version: "1", Codes: codes,
		Request: func(r *http.Request) (request, error) { return request{user: r.Header.Get("User")}, nil },
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", m.Handler(runtime(), discoverable()))
	srv := httptest.NewServer(httpkit.Wrap(mux, httpkit.Config{}))
	defer srv.Close()
	ctx := context.Background()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(ctx,
		&mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: userHeader("ann")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Zed", ""} { // "" fails: the tool's result says so
		if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "greetings.create", Arguments: map[string]any{"name": name}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "nope"}); err == nil {
		t.Fatal("an unknown tool was called")
	}
	session.Close()
	srv.Close() // every span has ended

	attrs := func(s sdktrace.ReadOnlySpan) map[string]string {
		out := map[string]string{}
		for _, a := range s.Attributes() {
			out[string(a.Key)] = a.Value.String()
		}
		return out
	}
	byName := map[string][]sdktrace.ReadOnlySpan{}
	for _, s := range spanRecorder().Ended()[from:] {
		byName[s.Name()] = append(byName[s.Name()], s)
	}
	if http := byName["POST /mcp"]; len(http) == 0 || attrs(http[0])["http.route"] != "/mcp" {
		t.Errorf("no POST /mcp span routed /mcp; spans: %v", keys(byName))
	}
	if list := byName["tools/list"]; len(list) != 1 || attrs(list[0])["mcp.method.name"] != "tools/list" || attrs(list[0])["gen_ai.operation.name"] != "" {
		t.Errorf("tools/list spans %d, want one, with its method and no operation", len(list))
	}
	calls := byName["tools/call greetings.create"]
	if len(calls) != 2 {
		t.Fatalf("tools/call greetings.create spans %d, want 2; spans: %v", len(calls), keys(byName))
	}
	for i, c := range calls {
		a := attrs(c)
		if c.SpanKind() != trace.SpanKindServer || a["mcp.method.name"] != "tools/call" || a["gen_ai.tool.name"] != "greetings.create" ||
			a["gen_ai.operation.name"] != "execute_tool" || a["mcp.session.id"] == "" || a["mcp.protocol.version"] == "" {
			t.Errorf("call %d: kind %v, attributes %v", i, c.SpanKind(), a)
		}
		failed := i == 1
		if got := c.Status().Code == otelcodes.Error && a["error.type"] == "tool_error"; got != failed {
			t.Errorf("call %d: status %v, error.type %q; failed %v", i, c.Status(), a["error.type"], failed)
		}
	}
	if nope := byName["tools/call nope"]; len(nope) != 1 || nope[0].Status().Code != otelcodes.Error || attrs(nope[0])["error.type"] == "" {
		t.Errorf("an unknown tool's spans %v, want one, failed, with its JSON-RPC code", len(nope))
	} else if _, err := strconv.Atoi(attrs(nope[0])["error.type"]); err != nil {
		t.Errorf("an unknown tool's error.type %q, want a JSON-RPC code", attrs(nope[0])["error.type"])
	}
}

func keys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }

// Every tool takes the host's call args beside its op's input: listed in its
// schema, not required, taken out before the op reads its input, and recorded
// on the op's span. A value the host doesn't take fails the call, before the op
// runs.
func TestMCPToolsTakeTheHostsCallArgs(t *testing.T) {
	from := record(t)
	m := apikit.MCP[request]{
		Name: "test", Version: "1", Codes: codes,
		CallArgs: []apikit.CallArg{{Name: "task", Description: "the task", Fields: func(v string) (map[string]string, error) {
			if v != "t1" {
				return nil, errors.New("not a task")
			}
			return map[string]string{"anfra.task.id": v}, nil
		}}},
		Request: func(r *http.Request) (request, error) { return request{user: r.Header.Get("User")}, nil },
	}
	srv := httptest.NewServer(httpkit.Wrap(m.Handler(runtime(), discoverable()), httpkit.Config{}))
	defer srv.Close()
	ctx := context.Background()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(ctx,
		&mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &http.Client{Transport: userHeader("ann")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name != "greetings.create" {
			continue
		}
		schema, _ := tool.InputSchema.(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		if _, ok := props["task"]; !ok {
			t.Errorf("greetings.create's schema has no task: %v", keys(props))
		}
		if required, _ := schema["required"].([]any); slices.Contains(required, any("task")) {
			t.Error("task is required")
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "greetings.create", Arguments: map[string]any{"name": "Zed", "task": "t1"}})
	if err != nil || res.IsError {
		t.Fatalf("a call with a task failed: %v %+v", err, res)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "greetings.create", Arguments: map[string]any{"name": "Zed", "task": "bogus"}})
	if err != nil || !res.IsError {
		t.Errorf("a call with a task the host doesn't take: %v, error result %v, want one", err, res != nil && res.IsError)
	}
	session.Close()
	srv.Close() // every span of the calls has ended

	var ops []sdktrace.ReadOnlySpan
	for _, s := range spanRecorder().Ended()[from:] {
		if s.Name() == "test_host.op.greetings.create" {
			ops = append(ops, s)
		}
	}
	if len(ops) != 1 {
		t.Fatalf("got %d op spans, want 1: the refused call runs no op", len(ops))
	}
	var task string
	for _, a := range ops[0].Attributes() {
		if a.Key == "anfra.task.id" {
			task = a.Value.AsString()
		}
	}
	if task != "t1" {
		t.Errorf("op's anfra.task.id %q, want t1", task)
	}
}
