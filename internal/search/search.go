// Package search is the catalog search use-case layer: it builds the request
// anfra-node needs to search the local catalog.
package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// Run asks anfra-node to search the local catalog. Go owns orchestration
// (sidecars, repo paths, canal endpoint); anfra-node owns search execution.
func Run(ctx context.Context, node *anfranode.Client, canal *canalquery.Client, r repo.Local, query string) (anfranode.CatalogSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if node == nil {
		return nil, fmt.Errorf("search requires the anfra-node sidecar")
	}
	if canal == nil {
		return nil, fmt.Errorf("search requires the canal-query sidecar")
	}

	return node.SearchCatalog(ctx, anfranode.CatalogSearchRequest{
		Context: anfranode.CatalogContext{
			RepoPath:          r.Dir,
			SourcesPath:       r.ContextSourcesPath(),
			CanalQueryBaseURL: canal.BaseURL(),
		},
		Query: query,
	})
}
