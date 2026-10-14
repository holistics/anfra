package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestRunPassesSourceToRPC(t *testing.T) {
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
	res, err := Run(context.Background(),
		anfranode.NewClientHTTP(node.URL),
		canalquery.NewClient("http://127.0.0.1:9000/", false),
		r,
		"warehouse",
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
	}
	if captured.Context != want {
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
	)
	if err == nil || !strings.Contains(err.Error(), "canal-query sidecar") {
		t.Fatalf("error = %v, want clear missing canal-query error", err)
	}
}
