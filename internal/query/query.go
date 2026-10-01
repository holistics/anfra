// Package query is the query use-case layer: it turns a (dataset, AQL) request
// into SQL (and later, results) by loading the repo's data sources and
// driving the sidecars via their clients. It depends on the clients (not the
// process managers), so it works equally against host-spawned or external
// sidecars. The CLI command and the future HTTP/MCP handler are thin shells
// over these functions.
package query

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
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
		RepoID:      r.ID,
		DatasetFqn:  dataset,
		AQL:         aql,
		DataSources: compileDataSources(sources),
	}, nil
}

// ParseInput normalizes a `query` Query Input arg into raw JSON for the sidecar.
// It arrives as a JSON string from the CLI (--input) or as an object over /call;
// absent or empty means no Query Input. anfra doesn't interpret the contents —
// the sidecar does — it only checks that it's a JSON object.
func ParseInput(v any) (json.RawMessage, error) {
	var raw []byte
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, nil
		}
		raw = []byte(t)
	case map[string]any:
		b, err := json.Marshal(t)
		if err != nil {
			return nil, fmt.Errorf("invalid input: %w", err)
		}
		raw = b
	default:
		return nil, fmt.Errorf("invalid input: expected a JSON object")
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("invalid input: expected a JSON object")
	}
	return json.RawMessage(bytes.TrimSpace(raw)), nil
}

// Run is how one query run is shaped beyond its AQL: its Query Input (see
// ParseInput) and its Execution Options. Pagination nil means all rows;
// Timezone empty means the sidecar's default.
type Run struct {
	Input      json.RawMessage
	Pagination *sidecar.Pagination
	Timezone   string
}

// ParsePagination builds the run's page from the `page` and `page-size` args
// (absent: nil, all rows). A page size alone means the first page; a page
// without a size is an error, since there's no sensible default size.
func ParsePagination(page, pageSize int) (*sidecar.Pagination, error) {
	switch {
	case page == 0 && pageSize == 0:
		return nil, nil
	case pageSize == 0:
		return nil, fmt.Errorf("--page needs --page-size")
	case page == 0:
		page = 1
	}
	if page < 1 || pageSize < 1 {
		return nil, fmt.Errorf("invalid paging: --page and --page-size must be >= 1")
	}
	return &sidecar.Pagination{Page: page, PageSize: pageSize}, nil
}

// Compile compiles an AQL query against a dataset into SQL plus the data source
// it targets (dialect + execution routing), without executing. Shared by
// --generate and the run path so both fail identically on a bad query. The
// result's AQL is the Executed AQL (the query with run.Input applied).
func Compile(ctx context.Context, node *sidecar.AnfraNodeClient, repo repo.Repo, dataset, aql string, run Run) (sidecar.CompileToSQLResult, error) {
	req, err := CompileRequest(repo, dataset, aql)
	if err != nil {
		return sidecar.CompileToSQLResult{}, err
	}
	req.Input = run.Input
	req.Pagination = run.Pagination
	if run.Timezone != "" {
		req.Options = &sidecar.CompileOptions{TimezoneRegion: run.Timezone}
	}
	res, err := node.CompileToSQL(ctx, req)
	if err != nil {
		return sidecar.CompileToSQLResult{}, fmt.Errorf("compile AQL for dataset %q: %w", dataset, err)
	}
	return res, nil
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
func Execute(ctx context.Context, canal *sidecar.CanalQueryClient, repo repo.Repo, compiled sidecar.CompileToSQLResult, truncateRows int) (*RunResult, error) {
	sources, err := datasource.Load(repo.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("load data sources: %w", err)
	}
	ds, ok := sources[compiled.DataSource.Name]
	if !ok {
		return nil, fmt.Errorf("data source %q is not defined in data_sources.yml", compiled.DataSource.Name)
	}
	if ds.Connection == nil {
		return nil, fmt.Errorf("data source %q has no `connection` in data_sources.yml (required to run queries)", ds.Name)
	}
	result, err := canal.Execute(ctx, compiled.DataSource.DBType, ds.Connection, compiled.SQL, truncateRows)
	if err != nil {
		return nil, fmt.Errorf("execute query on data source %q: %w", ds.Name, err)
	}
	return &RunResult{SQL: compiled.SQL, Fields: result.Fields, Records: result.Rows}, nil
}
