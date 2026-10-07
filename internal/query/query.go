// Package query is the query use-case layer: it checks a (dataset, AQL)
// request, turns it into SQL and runs it, by loading the repo's data sources
// and driving the sidecars via their clients. What an invalid query is, and the
// code it fails with, are its own. It depends on the clients (not the
// process managers), so it works equally against host-spawned or external
// sidecars. The CLI command and the future HTTP/MCP handler are thin shells
// over these functions.
package query

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
)

// NoLimit is the row limit meaning "no truncation" (canal's truncate_rows uses a
// negative value for this).
const NoLimit = -1

// limitDirectiveRe matches an anfra-level `limit:` directive trailing an AQL
// query — the closing brace of the query block, whitespace, then `limit: <n>` to
// end of line (e.g. "...} limit: 100"). The brace is kept; only the directive is
// stripped.
var limitDirectiveRe = regexp.MustCompile(`\}\s+limit:\s*([^\n]*)`)

// ExtractLimit pulls an anfra-level `limit:` directive out of an AQL query and
// returns the query with the directive removed plus the row limit. The AQL engine
// does not support `limit`, so anfra strips it here and applies it at execution
// time via canal's truncate_rows. Returns NoLimit when no directive is present.
// TODO: remove this when AQL supports limit
func ExtractLimit(aql string) (string, int, error) {
	m := limitDirectiveRe.FindStringSubmatch(aql)
	if m == nil {
		return aql, NoLimit, nil
	}
	raw := strings.TrimSpace(m[1])
	n, err := strconv.Atoi(raw)
	if err != nil {
		return aql, NoLimit, fmt.Errorf("invalid limit %q: expected a non-negative integer", raw)
	}
	if n < 0 {
		return aql, NoLimit, fmt.Errorf("invalid limit %d: must be >= 0", n)
	}
	cleaned := limitDirectiveRe.ReplaceAllString(aql, "}")
	return cleaned, n, nil
}

// CompileRequest loads the repo's data sources and builds the sidecar compile
// request for a (dataset, aql). Shared by SQL generation and AQL validation
// (both feed the same {repoPath, datasetFqn, aql, dataSources} to the sidecar).
func CompileRequest(r repo.Repo, dataset, aql string) (anfranode.CompileToSQLRequest, error) {
	sources, err := datasource.Load(r.ConfigDir)
	if err != nil {
		return anfranode.CompileToSQLRequest{}, fmt.Errorf("load data sources: %w", err)
	}
	return anfranode.CompileToSQLRequest{
		RepoPath:    r.Dir,
		RepoID:      r.ID,
		DatasetFqn:  dataset,
		AQL:         aql,
		DataSources: anfranode.CompileDataSources(sources),
	}, nil
}

// Run is what shapes one run of a query beyond its AQL: its Query Input,
// applied to the AQL before it compiles; a page of rows (nil: every row); and
// the time zone relative dates and date truncation use ("": anfra-node's
// default).
type Run struct {
	Input      *anfranode.QueryTransforms
	Pagination *anfranode.Pagination
	Timezone   string
}

// Compile compiles an AQL query against a dataset into SQL plus the data source
// it targets (dialect + execution routing), without executing. Shared by
// --generate and the run path so both fail identically on a bad query: with
// QueryInvalid and its diagnostics when the query does not compile. A failure
// that is not the query's (an unknown dataset, a missing data source, the run's
// shaping refused) is not diagnostic-shaped, and is returned as it is. The
// result's AQL is the query with run's Query Input applied.
func Compile(ctx context.Context, node *anfranode.Client, repo repo.Repo, dataset, aql string, run Run) (anfranode.CompileToSQLResult, error) {
	req, err := CompileRequest(repo, dataset, aql)
	if err != nil {
		return anfranode.CompileToSQLResult{}, err
	}
	req.Input, req.Pagination = run.Input, run.Pagination
	if run.Timezone != "" {
		req.Options = &anfranode.CompileOptions{TimezoneRegion: run.Timezone}
	}
	res, err := node.CompileToSQL(ctx, req)
	if err == nil {
		return res, nil
	}
	err = fmt.Errorf("compile AQL for dataset %q: %w", dataset, err)
	// anfra-node refusing what shapes the run (a Query Input entry, a page) is
	// not the query's fault: the caller says which arg is wrong.
	if e, ok := errors.AsType[*anfranode.RPCError](err); ok {
		if _, ok := e.Path(); ok {
			return anfranode.CompileToSQLResult{}, err
		}
	}
	if diags, verr := Check(ctx, node, repo, dataset, aql); verr == nil && !diags.Valid {
		return anfranode.CompileToSQLResult{}, apperr.EncapsulateWith(err, QueryInvalid, "", diags)
	}
	return anfranode.CompileToSQLResult{}, err
}

// RunResult is the compiled SQL plus the executed result.
type RunResult struct {
	SQL     string
	Fields  []string
	Records [][]any
}

// Execute runs already-compiled SQL via canal-query against the data source the
// dataset targets, returning the SQL and the result rows. Split from Compile so
// callers can distinguish a compile failure (a query problem) from an execution
// failure (a data-source/DB problem). truncateRows caps the rows canal returns
// (NoLimit for all rows); it's how anfra applies the AQL `limit:` directive.
func Execute(ctx context.Context, canal *canalquery.Client, repo repo.Repo, compiled anfranode.CompileToSQLResult, truncateRows int) (*RunResult, error) {
	sources, err := datasource.Load(repo.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("load data sources: %w", err)
	}
	ds, ok := sources[compiled.DataSource.Name]
	if !ok {
		return nil, fmt.Errorf("data source %q is not defined in data_sources.yml", compiled.DataSource.Name)
	}
	return run(ctx, canal, ds, compiled.DataSource.DBType, compiled.SQL, truncateRows)
}

// DataSource is the repo's data source by name, from data_sources.yml. ok is
// false when there is none by that name.
func DataSource(r repo.Repo, name string) (ds datasource.DataSource, ok bool, err error) {
	sources, err := datasource.Load(r.ConfigDir)
	if err != nil {
		return datasource.DataSource{}, false, fmt.Errorf("load data sources: %w", err)
	}
	ds, ok = sources[name]
	return ds, ok, nil
}

// ExecuteSQL runs sql on ds as it is: the caller wrote it in the data source's
// dialect, with its own LIMIT. Nothing is compiled into it — no restriction
// applies.
func ExecuteSQL(ctx context.Context, canal *canalquery.Client, ds datasource.DataSource, sql string) (*RunResult, error) {
	return run(ctx, canal, ds, ds.DBType, sql, NoLimit)
}

func run(ctx context.Context, canal *canalquery.Client, ds datasource.DataSource, dbType, sql string, truncateRows int) (*RunResult, error) {
	if ds.Connection == nil {
		return nil, fmt.Errorf("data source %q has no `connection` in data_sources.yml (required to run queries)", ds.Name)
	}
	result, err := canal.Execute(ctx, dbType, ds.Connection, sql, truncateRows)
	if err != nil {
		return nil, fmt.Errorf("execute query on data source %q: %w", ds.Name, err)
	}
	return &RunResult{SQL: sql, Fields: result.Fields, Records: result.Rows}, nil
}
