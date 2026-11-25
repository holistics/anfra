package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/command/sl"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestSLAddReportsManagedRepoWriteFailure(t *testing.T) {
	var node fakeNode
	want := anfranode.SLResult{ModelName: "public_orders", Path: "models/public_orders.model.aml", Content: "Model public_orders {}", Warnings: []string{}}
	srv := node.serve(t, want)
	cc := command.CommandContext{
		Clients: command.Clients{Node: anfranode.NewClientHTTP(srv.URL)},
		Repo:    managedRepo(t), DataPerms: dataperm.Ungoverned(),
	}

	_, err := Invoke(t.Context(), cc, "sl.add", []byte(`{"table":"public.orders","data_source":"warehouse"}`))
	if !errors.Is(err, repo.WriteFailed) || !strings.Contains(apperr.From(err).Response("").Message, want.Path) {
		t.Fatalf("write failure = %v", err)
	}
	if len(node.calls) != 1 || node.calls[0] != "sl.add" {
		t.Fatalf("calls = %v", node.calls)
	}

	params := node.params[0]
	ctx := params["context"].(map[string]any)
	if params["table"] != "public.orders" || params["dataSource"] != "warehouse" || ctx["revision"] == nil || ctx["metadata"] != nil {
		t.Fatalf("params = %+v", params)
	}
}

func TestSLWritesGeneratedContentToTheLocalRepo(t *testing.T) {
	local := repo.Resolve(t.TempDir())
	file := anfranode.SLResult{ModelName: "orders", Path: "models/orders.model.aml", Content: "Model orders {}\n"}
	path := filepath.Join(local.Dir, file.Path)
	run := func(name, args string) {
		t.Helper()
		var node fakeNode
		srv := node.serve(t, file)
		cc := command.CommandContext{
			Clients: command.Clients{Node: anfranode.NewClientHTTP(srv.URL)},
			Repo:    local.Repo(), AllowLocalRepo: true, DataPerms: dataperm.Ungoverned(),
		}
		answer, err := Invoke(t.Context(), cc, name, []byte(args))
		if err != nil {
			t.Fatal(err)
		}
		got := answer.(sl.Result)
		content, err := os.ReadFile(path)
		if err != nil || string(content) != file.Content || got.Content != file.Content || got.Path != file.Path {
			t.Fatalf("%s: result %+v, file %q, read error %v", name, got, content, err)
		}
	}
	run("sl.add", `{"table":"public.orders","data_source":"warehouse"}`)

	changed := true
	file.Changed = &changed
	file.Content = "// updated\n" + file.Content
	run("sl.update", `{"model_name":"orders"}`)

	past := time.Unix(1000, 0)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	changed = false
	run("sl.update", `{"model_name":"orders"}`)
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(past) {
		t.Fatalf("unchanged model attributes: %v, %v", info, err)
	}
}

func TestSLUpdateRequiresChanged(t *testing.T) {
	local := repo.Resolve(t.TempDir())
	var node fakeNode
	srv := node.serve(t, anfranode.SLResult{Path: "orders.aml", Content: "Model orders {}"})
	cc := command.CommandContext{
		Clients: command.Clients{Node: anfranode.NewClientHTTP(srv.URL)},
		Repo:    local.Repo(), AllowLocalRepo: true, DataPerms: dataperm.Ungoverned(),
	}
	if _, err := Invoke(t.Context(), cc, "sl.update", []byte(`{"model_name":"orders"}`)); err == nil {
		t.Fatal("accepted an update without changed")
	}
	if entries, err := os.ReadDir(local.Dir); err != nil || len(entries) != 0 {
		t.Fatalf("repo after invalid updates = %v, %v", entries, err)
	}
}

func TestSLAddMapsMissingEntityToItsArgument(t *testing.T) {
	for param, field := range map[string]string{"dataSource": "data_source", "table": "table"} {
		t.Run(field, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = jsonkit.MarshalWrite(w, map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{
					"code": -32602, "message": "Metadata is missing. Run ingest first.", "data": map[string]any{"code": "invalid_input", "path": param},
				}})
			}))
			defer srv.Close()

			cc := command.CommandContext{Clients: command.Clients{Node: anfranode.NewClientHTTP(srv.URL)}, Repo: managedRepo(t), DataPerms: dataperm.Ungoverned()}

			_, err := Invoke(context.Background(), cc, "sl.add", []byte(`{"table":"public.orders","data_source":"warehouse"}`))

			details, ok := apperr.DetailsOf(err, apperr.ValidationFailed)
			if !ok || len(details.Violations) != 1 || details.Violations[0].Field != field {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSLUpdateUsesManagedSourceAndMapsItsArgument(t *testing.T) {
	var node fakeNode
	changed := false
	want := anfranode.SLResult{ModelName: "sales.orders", Path: "models/orders.model.aml", Content: "Model orders {}", Changed: &changed}
	srv := node.serve(t, want)
	cc := command.CommandContext{Clients: command.Clients{Node: anfranode.NewClientHTTP(srv.URL)}, Repo: managedRepo(t), DataPerms: dataperm.Ungoverned()}

	result, err := Invoke(context.Background(), cc, "sl.update", []byte(`{"model_name":"sales::orders"}`))
	if err != nil {
		t.Fatal(err)
	}

	file, ok := result.(sl.Result)
	if !ok || file.Changed == nil || *file.Changed || file.Content != want.Content {
		t.Fatalf("file = %+v", result)
	}
	if len(node.calls) != 1 || node.calls[0] != "sl.update" || node.params[0]["modelName"] != "sales::orders" {
		t.Fatalf("calls = %v, params = %+v", node.calls, node.params)
	}
	if node.params[0]["context"].(map[string]any)["revision"] == nil {
		t.Fatal("missing managed revision")
	}

	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = jsonkit.MarshalWrite(w, map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{
			"code": -32602, "message": "The model is missing.", "data": map[string]any{"code": "invalid_input", "path": "modelName"},
		}})
	}))
	defer failed.Close()

	cc.Clients.Node = anfranode.NewClientHTTP(failed.URL)
	_, err = Invoke(context.Background(), cc, "sl.update", []byte(`{"model_name":"sales.orders"}`))

	details, ok := apperr.DetailsOf(err, apperr.ValidationFailed)
	if !ok || len(details.Violations) != 1 || details.Violations[0].Field != "model_name" {
		t.Fatalf("error = %v", err)
	}
}

func TestIngestContinuesWhenPostgresCannotBeRead(t *testing.T) {
	var node fakeNode
	srv := node.serve(t, nil)
	cc := command.CommandContext{
		Clients: command.Clients{Node: anfranode.NewClientHTTP(srv.URL), CanalQuery: canalquery.NewClient("http://unused", false)},
		Repo:    managedRepo(t), DataPerms: dataperm.Governed(nil),
		DataSources: datasource.Set{"warehouse": {Name: "warehouse", DBType: "postgresql"}},
	}

	result, err := Invoke(context.Background(), cc, "ingest", []byte(`{"source":"aml"}`))
	if err != nil || result != "ingest complete" {
		t.Fatalf("ingest = %v, %v", result, err)
	}
	if len(node.calls) != 1 || node.calls[0] != "catalog.ingest" || node.params[0]["source"] != "aml" {
		t.Fatalf("calls = %v, params = %+v", node.calls, node.params)
	}
	if node.params[0]["context"].(map[string]any)["metadata"] != nil {
		t.Fatal("failed metadata reached Node")
	}
}
