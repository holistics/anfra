// Package ingest is the catalog ingestion use-case layer: it builds the request
// anfra-node needs to ingest context sources into the local search catalog.
package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

// Run asks anfra-node to build the local catalog. Go owns orchestration
// (sidecars, repo paths, canal endpoint); the Node sidecar owns source parsing
// and catalog writes.
func Run(ctx context.Context, node *sidecar.AnfraNodeClient, canal *sidecar.CanalQueryClient, r repo.Repo, source string) (string, error) {
	if node == nil {
		return "", fmt.Errorf("ingest requires the anfra-node sidecar")
	}
	if canal == nil {
		return "", fmt.Errorf("ingest requires the canal-query sidecar")
	}
	if err := os.MkdirAll(r.CatalogDir(), 0o755); err != nil {
		return "", fmt.Errorf("create catalog dir: %w", err)
	}

	req := sidecar.CatalogIngestRequest{
		RepoPath:          r.Dir,
		SourcesPath:       filepath.Join(r.ConfigDir, "context_sources.yml"), // anfra-node owns ingest source config parsing; Go only resolves the repo path.
		CatalogPath:       r.CatalogPath(),
		CanalQueryBaseURL: canal.BaseURL(),
		Source:            source,
	}
	if err := node.IngestCatalog(ctx, req); err != nil {
		return "", err
	}
	return "ingest complete", nil
}
