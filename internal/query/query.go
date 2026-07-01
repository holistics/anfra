// Package query is the query use-case layer: it turns a (dataset, AQL) request
// into SQL (and later, results) by loading the repo's data sources and
// driving the sidecars via their clients. It depends on the clients (not the
// process managers), so it works equally against host-spawned or external
// sidecars. The CLI command and the future HTTP/MCP handler are thin shells
// over these functions.
package query

import (
	"context"
	"fmt"

	"github.com/anfra-ai/anfra/internal/datasource"
	"github.com/anfra-ai/anfra/internal/repo"
	"github.com/anfra-ai/anfra/internal/sidecar"
)

func compileDataSources(m map[string]datasource.DataSource) map[string]sidecar.CompileDataSource {
	out := make(map[string]sidecar.CompileDataSource, len(m))
	for name, ds := range m {
		out[name] = sidecar.CompileDataSource{Name: ds.Name, DBType: ds.DBType}
	}
	return out
}

// CompileRequest loads the repo's data sources and builds the sidecar compile
// request for a (dataset, aql). Shared by SQL generation and AQL validation
// (both feed the same {repoPath, datasetFqn, aql, dataSources} to the sidecar).
func CompileRequest(r repo.Repo, dataset, aql string) (sidecar.CompileToSQLRequest, error) {
	sources, err := datasource.Load(r.ConfigDir)
	if err != nil {
		return sidecar.CompileToSQLRequest{}, fmt.Errorf("load data sources: %w", err)
	}
	return sidecar.CompileToSQLRequest{
		RepoPath:    r.Dir,
		DatasetFqn:  dataset,
		AQL:         aql,
		DataSources: compileDataSources(sources),
	}, nil
}

// GenerateSQL compiles an AQL query against a dataset into SQL, using the
// dialects declared in the repo's data_sources.yml.
func GenerateSQL(ctx context.Context, node *sidecar.AnfraNodeClient, repo repo.Repo, dataset, aql string) (string, error) {
	req, err := CompileRequest(repo, dataset, aql)
	if err != nil {
		return "", err
	}
	res, err := node.CompileToSQL(ctx, req)
	if err != nil {
		return "", fmt.Errorf("compile AQL for dataset %q: %w", dataset, err)
	}
	return res.SQL, nil
}

// RunResult is the compiled SQL plus the executed result.
type RunResult struct {
	SQL     string
	Fields  []string
	Records [][]any
}

// Run compiles an AQL query to SQL and executes it via canal-query against the
// data source the dataset targets, returning the SQL and the result rows.
func Run(ctx context.Context, node *sidecar.AnfraNodeClient, canal *sidecar.CanalQueryClient, repo repo.Repo, dataset, aql string) (*RunResult, error) {
	sources, err := datasource.Load(repo.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("load data sources: %w", err)
	}
	compiled, err := node.CompileToSQL(ctx, sidecar.CompileToSQLRequest{
		RepoPath:    repo.Dir,
		DatasetFqn:  dataset,
		AQL:         aql,
		DataSources: compileDataSources(sources),
	})
	if err != nil {
		return nil, fmt.Errorf("compile AQL for dataset %q: %w", dataset, err)
	}

	ds, ok := sources[compiled.DataSource.Name]
	if !ok {
		return nil, fmt.Errorf("data source %q (used by dataset %q) is not defined in data_sources.yml", compiled.DataSource.Name, dataset)
	}
	if ds.Connection == nil {
		return nil, fmt.Errorf("data source %q has no `connection` in data_sources.yml (required to run queries)", ds.Name)
	}

	result, err := canal.Execute(ctx, compiled.DataSource.DBType, ds.Connection, compiled.SQL)
	if err != nil {
		return nil, fmt.Errorf("execute query on data source %q: %w", ds.Name, err)
	}
	return &RunResult{SQL: compiled.SQL, Fields: result.Fields, Records: result.Rows}, nil
}
