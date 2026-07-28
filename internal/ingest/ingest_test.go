package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

func TestRunPassesSourceToRPC(t *testing.T) {
	var captured sidecar.CatalogIngestRequest
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var req struct {
			Method string                       `json:"method"`
			Params sidecar.CatalogIngestRequest `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode rpc request: %v", err)
		}
		if req.Method != "catalog.ingest" {
			t.Fatalf("method = %q, want catalog.ingest", req.Method)
		}
		captured = req.Params
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
		})
	}))
	defer node.Close()

	repoDir := t.TempDir()
	r := repo.Repo{
		Dir:       repoDir,
		DataDir:   filepath.Join(t.TempDir(), "repos", "repo-123"),
		ConfigDir: filepath.Join(repoDir, ".anfra"),
	}
	res, err := Run(context.Background(),
		sidecar.NewAnfraNodeClientHTTP(node.URL),
		sidecar.NewCanalQueryClient("http://127.0.0.1:9000/", false),
		r,
		"warehouse",
	)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if captured.Source != "warehouse" {
		t.Fatalf("source = %q, want warehouse", captured.Source)
	}
	if captured.RepoPath != r.Dir {
		t.Fatalf("repoPath = %q, want %q", captured.RepoPath, r.Dir)
	}
	if captured.SourcesPath != filepath.Join(r.ConfigDir, "context_sources.yml") {
		t.Fatalf("sourcesPath = %q", captured.SourcesPath)
	}
	if captured.CatalogPath != r.CatalogPath() {
		t.Fatalf("catalogPath = %q, want %q", captured.CatalogPath, r.CatalogPath())
	}
	if info, err := os.Stat(r.CatalogDir()); err != nil || !info.IsDir() {
		t.Fatalf("catalog dir was not created: info=%v err=%v", info, err)
	}
	if captured.CanalQueryBaseURL != "http://127.0.0.1:9000" {
		t.Fatalf("canalQueryBaseUrl = %q", captured.CanalQueryBaseURL)
	}
	if res != "ingest complete" {
		t.Fatalf("result = %q, want ingest complete", res)
	}
}

func TestRunRequiresCanalQueryClient(t *testing.T) {
	_, err := Run(context.Background(),
		sidecar.NewAnfraNodeClientHTTP("http://127.0.0.1:9001"),
		nil,
		repo.Repo{},
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "canal-query sidecar") {
		t.Fatalf("error = %v, want clear missing canal-query error", err)
	}
}
