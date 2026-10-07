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
func Show(ctx context.Context, node *anfranode.Client, r repo.Repo, typ, fqn string) (anfranode.ShowResult, error) {
	sources, err := datasource.Load(r.ConfigDir)
	if err != nil {
		return anfranode.ShowResult{}, fmt.Errorf("load data sources: %w", err)
	}
	return node.ShowAML(ctx, anfranode.ShowRequest{
		RepoPath: r.Dir, RepoID: r.ID, DataSources: anfranode.CompileDataSources(sources), Type: typ, Fqn: fqn,
	})
}
