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
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
)

// NoLimit is the row limit meaning "no truncation" (canal's truncate_rows uses a
// negative value for this).
const NoLimit = -1

// DefaultMaxResultRows is the most rows a query's result may hold when the host
// sets no maximum (Limits.MaxResultRows).
const DefaultMaxResultRows = 1_000_000

// Limits bound what a query may cost the host. The zero value is the defaults.
type Limits struct {
	// MaxResultRows is the most rows a query's result may hold: a query that
	// produces more fails with ResultTooLarge rather than being cut short. Zero:
	// DefaultMaxResultRows.
	MaxResultRows int
}

func (l Limits) maxResultRows() int {
	if l.MaxResultRows > 0 {
		return l.MaxResultRows
	}
	return DefaultMaxResultRows
}

// ResultTooLarge is a query whose result has more rows than the host allows: it
// fails rather than being cut short, since a chart drawn from part of a result
// says something the whole would not. The maximum is in its details.
var ResultTooLarge = apperr.DefinePublicCodeWith[ResultLimit](errcode.NS, "result_too_large", apperr.User,
	"The query's result is larger than this server allows.")

// ResultLimit is the maximum a result went past.
type ResultLimit struct {
	MaxRows int `json:"max_rows"`
}

// rowCap is the truncate_rows to ask canal for: the query's own limit when it is
// within maxRows, otherwise one row past maxRows, so that a result over the
// maximum shows itself by that extra row instead of being cut (capped). A
// maxRows of zero or less is no maximum.
func rowCap(limit, maxRows int) (truncate int, capped bool) {
	if maxRows <= 0 || (limit != NoLimit && limit <= maxRows) {
		return limit, false
	}
	return maxRows + 1, true
}

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

// Source is what a query compiles against and runs on: the repo's AML, and its
// data sources, whose dialects compiling asks for and one of whose connections a
// run asks for.
type Source struct {
	Repo        repo.Repo
	DataSources datasource.DataSources
}

// CompileRequest builds the sidecar compile request for a (dataset, aql) against
// src, with its data sources' dialects and no connection. Shared by SQL
// generation and AQL validation (both feed the same {repo, datasetFqn, aql,
// dataSources} to the sidecar).
func CompileRequest(ctx context.Context, src Source, dataset, aql string) (anfranode.CompileToSQLRequest, error) {
	dialects, err := src.DataSources.Dialects(ctx)
	if err != nil {
		return anfranode.CompileToSQLRequest{}, fmt.Errorf("data source dialects: %w", err)
	}
	return anfranode.CompileToSQLRequest{
		RepoRef:     anfranode.RefTo(src.Repo),
		DatasetFqn:  dataset,
		AQL:         aql,
		DataSources: anfranode.CompileDataSources(dialects),
		// The AQL v2 compiler is off: anfra-node compiles and type-checks with
		// the legacy one.
		Options: &anfranode.CompileOptions{UseV2Compiler: false},
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
func Compile(ctx context.Context, node *anfranode.Client, src Source, dataset, aql string, run Run) (anfranode.CompileToSQLResult, error) {
	req, err := CompileRequest(ctx, src, dataset, aql)
	if err != nil {
		return anfranode.CompileToSQLResult{}, err
	}
	req.Input, req.Pagination = run.Input, run.Pagination
	if run.Timezone != "" {
		req.Options.TimezoneRegion = run.Timezone
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
	if diags, verr := Check(ctx, node, src, dataset, aql); verr == nil && !diags.Valid {
		return anfranode.CompileToSQLResult{}, apperr.EncapsulateWith(err, QueryInvalid, "", diags)
	}
	return anfranode.CompileToSQLResult{}, err
}

// RunResult is the compiled SQL plus the executed result: its header (fields
// and their types) and its rows.
type RunResult struct {
	SQL string
	canalquery.Header
	Records []canalquery.Row
}

// Execute runs already-compiled SQL via canal-query against the data source the
// dataset targets, returning the SQL and the result rows. Split from Compile so
// callers can distinguish a compile failure (a query problem) from an execution
// failure (a data-source/DB problem). truncateRows caps the rows canal returns
// (NoLimit for all rows); it's how anfra applies the AQL `limit:` directive.
// limits bound the result: past them, the query fails with ResultTooLarge.
func Execute(ctx context.Context, canal *canalquery.Client, sources datasource.DataSources, compiled anfranode.CompileToSQLResult, truncateRows int, limits Limits) (*RunResult, error) {
	ds, err := dataSourceOf(ctx, sources, compiled)
	if err != nil {
		return nil, err
	}
	return run(ctx, canal, ds, canalquery.Statement{SQL: compiled.SQL, DBType: compiled.DataSource.DBType, TruncateRows: truncateRows}, limits)
}

// Stream is Execute that hands the result to h as it arrives, its header
// first, holding none of it.
func Stream(ctx context.Context, canal *canalquery.Client, sources datasource.DataSources, compiled anfranode.CompileToSQLResult, truncateRows int, h canalquery.Handler) error {
	ds, err := dataSourceOf(ctx, sources, compiled)
	if err != nil {
		return err
	}
	return stream(ctx, canal, ds, canalquery.Statement{SQL: compiled.SQL, DBType: compiled.DataSource.DBType, TruncateRows: truncateRows}, h)
}

// dataSourceOf is the data source a compiled query runs on, connection included:
// the one its dataset targets, and only that one is asked for.
func dataSourceOf(ctx context.Context, sources datasource.DataSources, compiled anfranode.CompileToSQLResult) (datasource.DataSource, error) {
	ds, err := sources.Connection(ctx, compiled.DataSource.Name)
	if errors.Is(err, datasource.ErrUnknown) {
		return datasource.DataSource{}, fmt.Errorf("data source %q is not defined among the repo's data sources", compiled.DataSource.Name)
	}
	return ds, err
}

// ExecuteSQL runs sql on ds as it is: the caller wrote it in the data source's
// dialect, with its own LIMIT. Nothing is compiled into it — no restriction
// applies. limits are as for Execute.
func ExecuteSQL(ctx context.Context, canal *canalquery.Client, ds datasource.DataSource, sql string, limits Limits) (*RunResult, error) {
	return run(ctx, canal, ds, canalquery.Statement{SQL: sql, DBType: ds.DBType, TruncateRows: NoLimit}, limits)
}

// StreamSQL is ExecuteSQL that hands the result to h as it arrives, its header
// first, holding none of it.
func StreamSQL(ctx context.Context, canal *canalquery.Client, ds datasource.DataSource, sql string, h canalquery.Handler) error {
	return stream(ctx, canal, ds, canalquery.Statement{SQL: sql, DBType: ds.DBType, TruncateRows: NoLimit}, h)
}

// run holds the whole result, so it is where limits bound it: canal is asked for
// no more than one row past the maximum, and that row fails the query.
func run(ctx context.Context, canal *canalquery.Client, ds datasource.DataSource, st canalquery.Statement, limits Limits) (*RunResult, error) {
	st, err := on(ds, st)
	if err != nil {
		return nil, err
	}
	maxRows := limits.maxResultRows()
	var capped bool
	st.TruncateRows, capped = rowCap(st.TruncateRows, maxRows)
	result, err := canal.Execute(ctx, st)
	if err != nil {
		return nil, fmt.Errorf("execute query on data source %q: %w", ds.Name, err)
	}
	if capped && len(result.Rows) > maxRows {
		return nil, apperr.NewWith(ResultTooLarge,
			fmt.Sprintf("The query's result has more than %d rows, more than this server allows. Narrow the query, or add a limit.", maxRows),
			ResultLimit{MaxRows: maxRows})
	}
	return &RunResult{SQL: st.SQL, Header: result.Header, Records: result.Rows}, nil
}

func stream(ctx context.Context, canal *canalquery.Client, ds datasource.DataSource, st canalquery.Statement, h canalquery.Handler) error {
	st, err := on(ds, st)
	if err != nil {
		return err
	}
	if err := canal.Stream(ctx, st, h); err != nil {
		return fmt.Errorf("execute query on data source %q: %w", ds.Name, err)
	}
	return nil
}

// on is st run on ds: with its connection, which data_sources.yml must give, and
// its settings, if any.
func on(ds datasource.DataSource, st canalquery.Statement) (canalquery.Statement, error) {
	if ds.Connection == nil {
		return st, fmt.Errorf("data source %q has no `connection` in data_sources.yml (required to run queries)", ds.Name)
	}
	st.DBConfig = ds.Connection
	st.DBSetting = ds.Settings
	return st, nil
}
