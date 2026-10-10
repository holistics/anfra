package apikit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/httpkit"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/holistics/anfra/shared/apikit"
)

// discoverable is a registry where bob may not see teams.greet, and greetings.hidden
// is served over neither HTTP nor MCP.
func discoverable() *apikit.Registry[request] {
	reg := newRegistry()
	g := greet("greetings.create")
	g.MCP = true
	apikit.Register(reg, admission, g)
	apikit.Register(reg, admission, greet("teams.greet"))
	hidden := greet("greetings.hidden")
	hidden.HTTP = false
	apikit.Register(reg, admission, hidden)
	return reg
}

func notForBob(_ context.Context, r request, ops []apikit.Meta) ([]bool, error) {
	seen := make([]bool, len(ops))
	for i, m := range ops {
		seen[i] = !strings.HasPrefix(r.user, "bob") || m.Name != "teams.greet" // below a base, the user carries the team
	}
	return seen, nil
}

func discoveryServer(t *testing.T) http.Handler {
	t.Helper()
	a := adapter()
	a.Visible = notForBob
	return httpkit.Wrap(a.Handler(runtime(), discoverable()), httpkit.Config{Codes: codes})
}

func getJSON(t *testing.T, h http.Handler, path, user string, into any) int {
	t.Helper()
	w := do(h, http.MethodGet, path, user, "")
	if w.Code == http.StatusOK {
		if err := jsonkit.Unmarshal(w.Body.Bytes(), into); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	return w.Code
}

// The three levels over HTTP, each showing what the caller may call, below the
// server the ops are served from.
func TestDiscoveryOverHTTP(t *testing.T) {
	h := discoveryServer(t)

	var idx apikit.Index
	if getJSON(t, h, "/api/ops", "ann", &idx); !slices.Equal(idx.Groups, []apikit.GroupEntry{{Name: "greetings", Summary: "Greet people."}}) {
		t.Errorf("index at /api = %+v, want greetings alone (teams is below /api/t/{team})", idx)
	}
	var group apikit.GroupListing
	getJSON(t, h, "/api/ops?group=greetings", "ann", &group)
	if len(group.Ops) != 1 || group.Ops[0].Name != "greetings.create" {
		t.Errorf("group = %+v, want greetings.create alone (hidden is not served over HTTP)", group)
	}
	if code := getJSON(t, h, "/api/ops?group=nope", "ann", &group); code != 404 {
		t.Errorf("an unknown group: %d", code)
	}

	var usage apikit.Usage
	getJSON(t, h, "/api/ops/greetings.create", "ann", &usage)
	props, _ := usage.Input["properties"].(map[string]any)
	if usage.Summary != "Greet someone." || props["name"] == nil || usage.Output["properties"] == nil {
		t.Errorf("usage = %+v: want the summary and the inlined schemas", usage)
	}
	var codes []string
	for _, e := range usage.Errors {
		codes = append(codes, e.Code)
	}
	for _, want := range []string{"invalid_request", "internal_server_error", "no_entry", "not_allowed"} {
		if !slices.Contains(codes, want) {
			t.Errorf("usage errors %v lack %s", codes, want)
		}
	}

	// Below a base: its ops, as its caller may see them.
	getJSON(t, h, "/api/t/red/ops", "ann", &idx)
	if len(idx.Groups) != 1 || idx.Groups[0].Name != "teams" {
		t.Errorf("index below the base = %+v, want teams", idx)
	}
	if code := getJSON(t, h, "/api/t/red/ops", "bob", &idx); code != 200 || len(idx.Groups) != 0 {
		t.Errorf("bob's index below the base = %d %+v, want empty", code, idx)
	}
	if code := getJSON(t, h, "/api/t/red/ops/teams.greet", "bob", &usage); code != 404 {
		t.Errorf("an op bob may not see: %d, want 404, as for one that does not exist", code)
	}
}

// An op is registered only into a declared group.
func TestGroupsAreDeclared(t *testing.T) {
	mustPanic(t, "not declared", func() { apikit.Register(newRegistry(), admission, greet("undeclared.op")) })
	mustPanic(t, "declared twice", func() { newRegistry().Group("greetings", "Again.") })
}

// Over MCP: one tool per MCP op, with discovery; a call answers the op's
// output, a failure its error body.
func TestMCP(t *testing.T) {
	m := apikit.MCP[request]{
		Name: "test", Version: "1", Codes: codes, Visible: notForBob,
		Request: func(r *http.Request) (request, error) { return request{user: r.Header.Get("User")}, nil },
	}
	srv := httptest.NewServer(m.Handler(runtime(), discoverable()))
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
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if want := []string{"describe_operation", "greetings.create", "list_operations"}; !slices.Equal(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "greetings.create", Arguments: map[string]any{"name": "Zed"}})
	if err != nil || res.IsError || !strings.Contains(text(res), "ann greets Zed") {
		t.Errorf("a call: %v %+v", err, res)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "greetings.create", Arguments: map[string]any{"name": ""}})
	if err != nil || !res.IsError || !strings.Contains(text(res), `"code":"invalid_request"`) {
		t.Errorf("an invalid call: %v %+v", err, res)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "describe_operation", Arguments: map[string]any{"name": "greetings.create"}})
	if err != nil || res.IsError || !strings.Contains(text(res), `"summary":"Greet someone."`) {
		t.Errorf("describe_operation: %v %+v", err, res)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "list_operations", Arguments: map[string]any{}})
	if err != nil || res.IsError || !strings.Contains(text(res), `"name":"greetings"`) {
		t.Errorf("list_operations: %v %+v", err, res)
	}
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

type userHeader string

func (u userHeader) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User", string(u))
	return http.DefaultTransport.RoundTrip(r)
}

// An op that changes things says to MCP whether it may destroy: one declared
// NonDestructive only adds; one not declared so may.
func TestMCPDestructiveHint(t *testing.T) {
	reg := newRegistry()
	add := func(name string, nonDestructive bool) {
		g := greet(name)
		g.MCP, g.NonDestructive = true, nonDestructive
		apikit.Register(reg, admission, g)
	}
	add("greetings.create", true)
	add("greetings.replace", false)
	m := apikit.MCP[request]{Name: "test", Version: "1", Codes: codes,
		Request: func(r *http.Request) (request, error) { return request{user: r.Header.Get("User")}, nil }}
	srv := httptest.NewServer(m.Handler(runtime(), reg))
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
		d := tool.Annotations.DestructiveHint
		switch tool.Name {
		case "greetings.create":
			if d == nil || *d {
				t.Errorf("a non-destructive op: destructiveHint %v, want false", d)
			}
		case "greetings.replace":
			if d == nil || !*d {
				t.Errorf("an op that may destroy: destructiveHint %v, want true", d)
			}
		}
	}
}
