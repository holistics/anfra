// Package ingest is the catalog ingestion use-case layer: it builds the request
// anfra-node needs to ingest context sources into the local search catalog.
package ingest

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// Run asks anfra-node to build the repo's catalog. Go owns orchestration
// (sidecars, which repo, canal endpoint); the Node sidecar owns source parsing
// and catalog writes.
func Run(ctx context.Context, node *anfranode.Client, canal *canalquery.Client, r repo.Repo, source string) (string, error) {
	if node == nil {
		return "", fmt.Errorf("ingest requires the anfra-node sidecar")
	}
	if canal == nil {
		return "", fmt.Errorf("ingest requires the canal-query sidecar")
	}

	req := anfranode.CatalogIngestRequest{
		Context: anfranode.CatalogContextOf(r, canal.BaseURL()),
		Source:  source,
	}
	if err := node.IngestCatalog(ctx, req); err != nil {
		return "", err
	}
	return "ingest complete", nil
}
