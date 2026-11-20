package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestAddTableUsesManagedCatalogWithoutAConnection(t *testing.T) {
	var node fakeNode
	display := "orders"
	want := anfranode.CatalogEntity{EntityID: "database.table:orders", Type: "database.table", Source: "warehouse", DisplayName: &display, Properties: map[string]any{"table_name": "orders"}}
	srv := node.serve(t, want)
	cc := command.CommandContext{
		Clients: command.Clients{Node: anfranode.NewClientHTTP(srv.URL)},
		Repo:    managedRepo(t), DataPerms: dataperm.Ungoverned(),
	}
	result, err := Invoke(context.Background(), cc, "sl.add", []byte(`{"table":"public.orders","data_source":"warehouse"}`))
	if err != nil {
		t.Fatal(err)
	}
	entity, ok := result.(anfranode.CatalogEntity)
	if !ok || entity.EntityID != want.EntityID || entity.Source != want.Source {
		t.Fatalf("entity = %+v", result)
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

func TestAddTableMapsMissingEntityToItsArgument(t *testing.T) {
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
