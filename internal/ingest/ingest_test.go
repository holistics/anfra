package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/metadata/postgres"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestRunPassesSourceAndMetadataToRPC(t *testing.T) {
	var captured anfranode.CatalogIngestRequest
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var req struct {
			Method string                         `json:"method"`
			Params anfranode.CatalogIngestRequest `json:"params"`
		}
		if err := jsonkit.UnmarshalRead(r.Body, &req); err != nil {
			t.Fatalf("decode rpc request: %v", err)
		}
		if req.Method != "catalog.ingest" {
			t.Fatalf("method = %q, want catalog.ingest", req.Method)
		}
		captured = req.Params
		wire, err := jsonkit.Marshal(req.Params)
		if err != nil || !strings.Contains(string(wire), `"metadata":{"postgres":{"sourceKey":"warehouse","snapshot":`) {
			t.Fatalf("metadata envelope = %s, %v", wire, err)
		}
		_ = jsonkit.MarshalWrite(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
		})
	}))
	defer node.Close()

	repoDir := t.TempDir()
	r := repo.Local{
		Dir:       repoDir,
		ID:        "repo-123",
		DataDir:   filepath.Join(t.TempDir(), "repos", "repo-123"),
		ConfigDir: filepath.Join(repoDir, ".anfra"),
	}.Repo()
	metadata := &anfranode.CatalogMetadata{Postgres: &anfranode.PostgresMetadata{
		SourceKey: "warehouse",
		Snapshot: postgres.SchemaSnapshot{
			Tables: []postgres.Table{{
				TableIdentity: postgres.TableIdentity{DatabaseName: "analytics", SchemaName: "public", TableName: "orders"},
				RelationName:  `"analytics"."public"."orders"`,
			}},
			Columns: []postgres.Column{}, ForeignKeys: []postgres.ForeignKey{},
		},
	}}
	res, err := Run(context.Background(),
		anfranode.NewClientHTTP(node.URL),
		canalquery.NewClient("http://127.0.0.1:9000/", false),
		r,
		"warehouse",
		metadata,
	)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if captured.Source != "warehouse" {
		t.Fatalf("source = %q, want warehouse", captured.Source)
	}
	want := anfranode.CatalogContext{
		RepoRef:           anfranode.RepoRef{RepoID: "repo-123", RepoPath: repoDir},
		CanalQueryBaseURL: "http://127.0.0.1:9000",
		Metadata:          metadata,
	}
	if !reflect.DeepEqual(captured.Context, want) {
		t.Fatalf("context = %+v, want %+v", captured.Context, want)
	}
	if res != "ingest complete" {
		t.Fatalf("result = %q, want ingest complete", res)
	}
}

func TestRunRequiresCanalQueryClient(t *testing.T) {
	_, err := Run(context.Background(),
		anfranode.NewClientHTTP("http://127.0.0.1:9001"),
		nil,
		repo.Repo{},
		"",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "canal-query sidecar") {
		t.Fatalf("error = %v, want clear missing canal-query error", err)
	}
}
