package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

func TestRunPassesQueryToRPC(t *testing.T) {
	var captured sidecar.CatalogSearchRequest
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var req struct {
			Method string                       `json:"method"`
			Params sidecar.CatalogSearchRequest `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode rpc request: %v", err)
		}
		if req.Method != "catalog.search" {
			t.Fatalf("method = %q, want catalog.search", req.Method)
		}
		captured = req.Params
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"results": []any{
					map[string]any{"entity_id": "aml.model:orders"},
				},
				"meta": map[string]any{"total": 1},
			},
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
		"type:aml.model orders",
	)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if captured.Query != "type:aml.model orders" {
		t.Fatalf("query = %q, want type:aml.model orders", captured.Query)
	}
	if captured.CatalogPath != r.CatalogPath() {
		t.Fatalf("catalogPath = %q, want %q", captured.CatalogPath, r.CatalogPath())
	}
	if captured.CanalQueryBaseURL != "http://127.0.0.1:9000" {
		t.Fatalf("canalQueryBaseUrl = %q", captured.CanalQueryBaseURL)
	}
	results, _ := res["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1", len(results))
	}
}

func TestRunRequiresQuery(t *testing.T) {
	_, err := Run(context.Background(),
		sidecar.NewAnfraNodeClientHTTP("http://127.0.0.1:9001"),
		sidecar.NewCanalQueryClient("http://127.0.0.1:9000/", false),
		repo.Repo{},
		" ",
	)
	if err == nil || !strings.Contains(err.Error(), "search query is required") {
		t.Fatalf("error = %v, want clear missing query error", err)
	}
}
