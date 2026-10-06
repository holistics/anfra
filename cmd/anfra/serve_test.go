package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/repo"
)

var serverAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 7878}

// handler is the server's handler for a repo, with no sidecars connected.
func handler(t *testing.T) (http.Handler, repo.Repo) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	r := repo.Resolve(t.TempDir())
	cc := app.CommandContext{Repo: r, DataPerms: dataperm.Unrestricted(),
		Server: &app.ServerInfo{URL: "http://127.0.0.1:7878", InstanceID: "i-1", Version: "dev"}}
	return serveHandler(slog.New(slog.DiscardHandler), r, cc, serverAddr, true), r
}

func do(h http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "127.0.0.1:7878"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i] == "Host" {
			req.Host = headers[i+1]
			continue
		}
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestServeRoutes(t *testing.T) {
	h, r := handler(t)
	for _, tc := range []struct {
		name, method, path, body string
		headers                  []string
		status                   int
		contains                 string
	}{
		{"an op", http.MethodPost, "/api/core.version", `{}`, nil, 200, `"version"`},
		{"an op's verdict, in its answer", http.MethodPost, "/api/core.status", `{}`, nil, 200, `"state":"not_running"`},
		{"an invalid input", http.MethodPost, "/api/core.query.compile", `{}`, nil, 400, `"invalid_request"`},
		{"no such op", http.MethodPost, "/api/core.nope", `{}`, nil, 404, `"not_found"`},
		{"not a POST", http.MethodGet, "/api/core.version", ``, nil, 400, `Every operation is a POST`},
		{"no /call", http.MethodPost, "/call", `{"command":"version"}`, nil, 404, ``},
		{"the contract", http.MethodGet, "/api/openapi.json", ``, nil, 200, `"/core.query"`},
		{"the index", http.MethodGet, "/api/ops", ``, nil, 200, `{"groups":[{"name":"core","summary":"anfra core's commands`},
		{"a group", http.MethodGet, "/api/ops?group=core", ``, nil, 200, `"name":"core.query.compile"`},
		{"an op's usage", http.MethodGet, "/api/ops/core.query", ``, nil, 200, `"input":{`},
		{"MCP", http.MethodPost, "/mcp",
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
			[]string{"Accept", "application/json, text/event-stream"}, 200, `"serverInfo":{"name":"anfra"`},
		{"health says who it is", http.MethodGet, "/health", ``, nil, 200, `"repo_id":"` + r.ID + `","instance_id":"i-1"`},
		// The guards.
		{"another host: DNS rebinding", http.MethodPost, "/api/core.version", `{}`, []string{"Host", "evil.example:7878"}, 400, `Unknown host`},
		{"localhost is this host", http.MethodPost, "/api/core.version", `{}`, []string{"Host", "localhost:7878"}, 200, `"version"`},
		{"a cross-site page", http.MethodPost, "/api/core.version", `{}`, []string{"Sec-Fetch-Site", "cross-site"}, 400, `Cross-origin`},
		{"a form's body", http.MethodPost, "/api/core.version", `{}`, []string{"Content-Type", "text/plain"}, 400, `application/json`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := do(h, tc.method, tc.path, tc.body, tc.headers...)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
				t.Errorf("got %d %s, want %d containing %s", w.Code, w.Body, tc.status, tc.contains)
			}
		})
	}
}

// status reports where the server is, when it runs in one.
func TestStatusReportsTheServer(t *testing.T) {
	h, _ := handler(t)
	var got app.StatusResult
	if err := json.NewDecoder(do(h, http.MethodPost, "/api/core.status", `{}`).Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Server == nil || got.Server.URL != "http://127.0.0.1:7878" || got.Server.InstanceID != "i-1" {
		t.Errorf("status = %+v, want the server's URL and instance", got)
	}
}

// The CLI uses a server only when the runtime file names one that answers as
// that server: after a crash, the port may be another's.
func TestFindServer(t *testing.T) {
	h, r := handler(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx := context.Background()

	if _, ok := findServer(ctx, r); ok {
		t.Fatal("found a server with no runtime file")
	}
	write := func(instance string) {
		t.Helper()
		if err := writeRuntime(r, runtimeFile{URL: srv.URL, RepoID: r.ID, RepoDir: r.Dir, InstanceID: instance}); err != nil {
			t.Fatal(err)
		}
	}
	write("i-1")
	if f, ok := findServer(ctx, r); !ok || f.URL != srv.URL {
		t.Errorf("did not find the server its runtime file names: %+v, %v", f, ok)
	}
	write("i-old") // a stale file: a new server holds the port
	if _, ok := findServer(ctx, r); ok {
		t.Error("used a server whose instance is not the one recorded")
	}
	srv.Close()
	write("i-1") // a dead server
	if _, ok := findServer(ctx, r); ok {
		t.Error("used a server that does not answer")
	}

	removeRuntime(r, "i-other")
	if _, ok := readRuntime(runtimePath(r)); !ok {
		t.Error("removed another instance's runtime file")
	}
	removeRuntime(r, "i-1")
	if _, ok := readRuntime(runtimePath(r)); ok {
		t.Error("did not remove its own runtime file")
	}
}

// The default address moves aside for another repo's server; an explicit one
// does not, and says whose server holds it.
func TestListen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	other := repo.Resolve(t.TempDir())
	if err := writeRuntime(other, runtimeFile{URL: "http://" + taken.Addr().String(), RepoID: other.ID, RepoDir: other.Dir}); err != nil {
		t.Fatal(err)
	}
	me := repo.Resolve(t.TempDir())

	_, err = listen(me, taken.Addr().String())
	if err == nil || !strings.Contains(err.Error(), other.Dir) {
		t.Errorf("an explicit taken address: got %v, want an error naming %s", err, other.Dir)
	}
	ln, err := listen(me, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
}

func TestStopWhenIdle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	h := stopWhenIdle(ctx, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }),
		40*time.Millisecond, func() { close(stopped) })
	do(h, http.MethodGet, "/", "")
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("an idle server did not stop")
	}
}
