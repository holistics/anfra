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

// Request is a search: the query, and the page of what it matches. A zero
// Limit leaves the page size to anfra-node.
type Request struct {
	Query  string
	Limit  int
	Offset int
}

// Run asks anfra-node to search the repo's catalog. Go owns orchestration
// (sidecars, which repo, canal endpoint); anfra-node owns search execution.
func Run(ctx context.Context, node *anfranode.Client, canal *canalquery.Client, r repo.Repo, req Request) (anfranode.CatalogSearchResult, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if node == nil {
		return nil, fmt.Errorf("search requires the anfra-node sidecar")
	}
	if canal == nil {
		return nil, fmt.Errorf("search requires the canal-query sidecar")
	}

	return node.SearchCatalog(ctx, anfranode.CatalogSearchRequest{
		Context: anfranode.CatalogContextOf(r, canal.BaseURL()),
		Query:   req.Query,
		Limit:   req.Limit,
		Offset:  req.Offset,
	})
}
