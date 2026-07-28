// Package search is the catalog search use-case layer: it builds the request
// anfra-node needs to search the local catalog.
package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

// Run asks anfra-node to search the local catalog. Go owns orchestration
// (sidecars, catalog path, canal endpoint); anfra-node owns search execution.
func Run(ctx context.Context, node *sidecar.AnfraNodeClient, canal *sidecar.CanalQueryClient, r repo.Repo, query string) (sidecar.CatalogSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if node == nil {
		return nil, fmt.Errorf("search requires the anfra-node sidecar")
	}
	if canal == nil {
		return nil, fmt.Errorf("search requires the canal-query sidecar")
	}

	return node.SearchCatalog(ctx, sidecar.CatalogSearchRequest{
		CatalogPath:       r.CatalogPath(),
		CanalQueryBaseURL: canal.BaseURL(),
		Query:             query,
	})
}
