package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/command/catalog"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// fakeLineageNode is an anfra-node that answers catalog.lineage with answer,
// and keeps the params it was called with.
func fakeLineageNode(t *testing.T, answer string) (*anfranode.Client, *anfranode.CatalogLineageRequest) {
	t.Helper()
	var got anfranode.CatalogLineageRequest
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string                          `json:"method"`
			Params anfranode.CatalogLineageRequest `json:"params"`
		}
		if err := jsonkit.UnmarshalRead(r.Body, &req); err != nil || req.Method != "catalog.lineage" {
			t.Errorf("rpc %q, %v", req.Method, err)
		}
		got = req.Params
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + answer + `}`))
	}))
	t.Cleanup(node.Close)
	return anfranode.NewClientHTTP(node.URL), &got
}

func TestLineageDeclaresAnfraNodeAndCanalQuery(t *testing.T) {
	cmd, ok := Find("lineage")
	if !ok {
		t.Fatal("lineage command was not registered")
	}
	if needs := cmd.Needs([]byte(`{"targets":[{"parent":"orders","child":"amount"}]}`)); !needs.Node || !needs.CanalQuery {
		t.Fatalf("lineage sidecars = %+v, want anfra-node and canal-query", needs)
	}
}

func lineageContext(node *anfranode.Client) command.CommandContext {
	return command.CommandContext{
		Clients:        command.Clients{Node: node, CanalQuery: canalquery.NewClient("http://127.0.0.1:9000/", false)},
		Repo:           repo.Local{Dir: "/repo", ID: "repo-1", ConfigDir: "/repo/.anfra"}.Repo(),
		AllowLocalRepo: true,
		DataPerms:      dataperm.Unrestricted(),
	}
}

// Each field goes to anfra-node by its parent and child, trimmed, and comes
// back by them, in the order asked, a field that failed among them.
func TestLineageTracesEachField(t *testing.T) {
	node, got := fakeLineageNode(t, `{"results":[`+
		`{"target":{"parent":"ecommerce","child":"revenue"},"query":"needed_by:(fqn:ecommerce.revenue)","diagram":{"version":1,"query":"needed_by:(fqn:ecommerce.revenue)","total":0,"matched":[],"schema":{"edges":{},"types":{}},"layers":[],"cards":[]}},`+
		`{"target":{"parent":"orders","child":"amount"},"query":"needed_by:(fqn:orders.amount)","error":"boom"}]}`)
	cc := lineageContext(node)

	out, err := Invoke(context.Background(), cc, "lineage", []byte(`{"targets":[{"parent":" ecommerce ","child":"revenue "},{"parent":"orders","child":"amount"}],"limit":5,"offset":10}`))
	if err != nil {
		t.Fatal(err)
	}
	want := anfranode.CatalogLineageRequest{
		Context: anfranode.CatalogContext{
			RepoPath: "/repo", SourcesPath: filepath.Join("/repo/.anfra", "context_sources.yml"), CanalQueryBaseURL: "http://127.0.0.1:9000",
		},
		Targets: []anfranode.LineageTarget{{Parent: "ecommerce", Child: "revenue"}, {Parent: "orders", Child: "amount"}},
		Limit:   5, Offset: 10,
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("sent %+v, want %+v", *got, want)
	}
	res := out.(catalog.LineageResult)
	if len(res.Results) != 2 || res.Results[0].Target != want.Targets[0] || res.Results[0].Diagram == nil ||
		res.Results[1].Target != want.Targets[1] || res.Results[1].Error != "boom" || res.Results[1].Diagram != nil {
		t.Errorf("answer = %+v", res)
	}
}

// A field without a parent or a child is refused, by the arg, before anfra-node
// is called.
func TestLineageRefusesWhatIsNotAField(t *testing.T) {
	node, got := fakeLineageNode(t, `{"results":[]}`)
	cc := lineageContext(node)
	for _, input := range []string{`{}`, `{"targets":[]}`, `{"targets":[{"parent":" ","child":"amount"}]}`} {
		_, err := Invoke(context.Background(), cc, "lineage", []byte(input))
		if !errors.Is(err, apperr.ValidationFailed) {
			t.Errorf("%s: got %v, want validation_failed", input, err)
		}
	}
	// The schema refuses a field that is not an object of both, and a limit of
	// no entities.
	for _, input := range []string{
		`{"targets":["orders.amount"]}`, `{"targets":[{"child":"amount"}]}`, `{"targets":[{"parent":"orders","child":""}]}`,
		`{"targets":[{"parent":"orders","child":"amount"}],"limit":0}`,
	} {
		if _, err := Invoke(context.Background(), cc, "lineage", []byte(input)); err == nil {
			t.Errorf("%s was not refused", input)
		}
	}
	if got.Targets != nil {
		t.Errorf("anfra-node was called with %+v", *got)
	}
}
