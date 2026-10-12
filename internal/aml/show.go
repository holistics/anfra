package aml

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
)

// Show shows an object of the repo, read from its compiled AML: the repo when
// typ and fqn are empty, or the object the catalog identifies by them (for now,
// a dataset). anfra-node refuses what it cannot find as invalid input, at the
// arg's path.
func Show(ctx context.Context, node *anfranode.Client, r repo.Repo, sources datasource.DataSources, typ, fqn string) (anfranode.ShowResult, error) {
	dialects, err := sources.Dialects(ctx)
	if err != nil {
		return anfranode.ShowResult{}, fmt.Errorf("data source dialects: %w", err)
	}
	return node.ShowAML(ctx, anfranode.ShowRequest{
		RepoRef: anfranode.RefTo(r), DataSources: anfranode.CompileDataSources(dialects), Type: typ, Fqn: fqn,
	})
}
