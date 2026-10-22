package apikit

import (
	"context"
	"encoding/json/jsontext"
	"fmt"
	"net/http"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/holistics/anfra/shared/httpkit"
	"github.com/holistics/anfra/shared/requestid"
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
}

// Handler is the MCP endpoint, to mount at /mcp behind the same middleware and
// guards as the HTTP API.
func (m MCP[Call]) Handler(rt *Runtime, reg OpSet[Call]) http.Handler {
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
	for _, o := range ops {
		meta := o.Meta()
		input, err := rt.Inline(o.InSchema(rt))
		if err != nil {
			return nil, fmt.Errorf("op %q: %w", meta.Name, err)
		}
		if input["type"] == nil {
			input["type"] = "object" // MCP requires an object schema, even for no input
		}
		description := meta.Summary
		if meta.Doc != "" {
			description += "\n\n" + meta.Doc
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
