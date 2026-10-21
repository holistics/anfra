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

// DefaultMaxResultBytes is the most a query's result may hold, in bytes, when
// the host sets no maximum (Limits.MaxResultBytes).
const DefaultMaxResultBytes = 256 << 20

// DefaultMaxPageBytes is the most a page of a query's result may hold, in its
// rows' bytes, when the host sets no maximum (Limits.MaxPageBytes).
const DefaultMaxPageBytes = 16 << 20

// Limits bound what a query may cost the host. The zero value is the defaults.
type Limits struct {
	// MaxResultRows is the most rows a query's result may hold: a query that
	// produces more fails with ResultTooLarge rather than being cut short. Zero:
	// DefaultMaxResultRows.
	MaxResultRows int
	// MaxResultBytes is the most a query's result may hold, in bytes, as
	// canal-query holds it (Arrow data in memory, about what its JSON takes): a
	// query that produces more fails with ResultTooLarge. Zero:
	// DefaultMaxResultBytes.
	MaxResultBytes int64
	// MaxPageBytes is the most a page answered whole may hold, in its rows'
	// bytes: a page over it fails with PageTooLarge rather than being cut short,
	// so that page numbers stay exact. Zero: DefaultMaxPageBytes.
	MaxPageBytes int
}

// ResultRows is MaxResultRows, or its default.
func (l Limits) ResultRows() int {
	if l.MaxResultRows > 0 {
		return l.MaxResultRows
	}
	return DefaultMaxResultRows
}

// ResultBytes is MaxResultBytes, or its default.
func (l Limits) ResultBytes() int64 {
	if l.MaxResultBytes > 0 {
		return l.MaxResultBytes
	}
	return DefaultMaxResultBytes
}

// ResultBounds bound how much of a query's result is fetched: up to Truncate
// rows, where the result is cut as the query asked (its limit: directive, or a
// page); and no more than MaxRows rows or MaxBytes bytes, past which the query
// fails with ResultTooLarge — the host's maximums (Limits), bounding what it
// holds.
type ResultBounds struct {
	Truncate int   // NoLimit: every row the query produces
	MaxRows  int   // zero: no maximum
	MaxBytes int64 // zero: no maximum
}

// Bounded are the bounds of the host's maximums (limits), the result truncated
// at truncate rows.
func Bounded(truncate int, limits Limits) ResultBounds {
	return ResultBounds{Truncate: truncate, MaxRows: limits.ResultRows(), MaxBytes: limits.ResultBytes()}
}

// Unbounded is every row, however many: for a result streamed to a file, which
// holds none of it.
var Unbounded = ResultBounds{Truncate: NoLimit}

// PageBytes is MaxPageBytes, or its default.
func (l Limits) PageBytes() int {
	if l.MaxPageBytes > 0 {
		return l.MaxPageBytes
	}
	return DefaultMaxPageBytes
}

// ResultTooLarge is a query whose result has more rows than the host allows: it
// fails rather than being cut short, since a chart drawn from part of a result
// says something the whole would not. The maximum is in its details.
var ResultTooLarge = apperr.DefinePublicCodeWith[ResultLimit](errcode.NS, "result_too_large", apperr.User,
	"The query's result is larger than this server allows.")

// ResultLimit is the maximum a result went past: its rows, or its bytes.
type ResultLimit struct {
	MaxRows  int   `json:"max_rows,omitempty"`
	MaxBytes int64 `json:"max_bytes,omitempty"`
}

// PageTooLarge is a page of a query's result larger, in bytes, than the host
// answers at once: it fails rather than being cut short, so that page numbers
// stay exact. The maximum is in its details.
var PageTooLarge = apperr.DefinePublicCodeWith[PageLimit](errcode.NS, "page_too_large", apperr.User,
	"The page is larger than this server answers at once.")

// PageLimit is the maximum a page went past.
type PageLimit struct {
	MaxBytes int `json:"max_bytes"`
}

// NewPageTooLarge is the error of a page past maxBytes.
func NewPageTooLarge(maxBytes int) error {
	return apperr.NewWith(PageTooLarge,
		fmt.Sprintf("The page is larger than %d MiB, more than this server answers at once. Ask for fewer rows a page, or stream the result.", maxBytes>>20),
		PageLimit{MaxBytes: maxBytes})
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
// QueryInvalid and its diagnostics when the query does not compile. Any other
// refusal (a dataset that cannot be used, the run's shaping) is returned as it
// is, for the command to place on its arg (anfranode.Violation). The result's
// AQL is the query with run's Query Input applied.
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
	if d, ok := anfranode.DetailsOf[anfranode.QueryDiagnostics](err, anfranode.QueryInvalid); ok {
		return anfranode.CompileToSQLResult{}, apperr.EncapsulateWith(err, QueryInvalid, "", QueryValidation{Valid: false, Diagnostics: d.Diagnostics})
	}
	return anfranode.CompileToSQLResult{}, err
}

// Stream runs already-compiled SQL via canal-query against the data source the
// dataset targets, handing its rows within bounds to h as they arrive, its
// header first, holding none of it. Split from Compile so callers can distinguish a
// compile failure (a query problem) from an execution failure (a data-source/DB
// problem).
func Stream(ctx context.Context, canal *canalquery.Client, sources datasource.DataSources, compiled anfranode.CompileToSQLResult, bounds ResultBounds, h canalquery.Handler) error {
	ds, err := dataSourceOf(ctx, sources, compiled)
	if err != nil {
		return err
	}
	return stream(ctx, canal, ds, canalquery.Statement{SQL: compiled.SQL, DBType: compiled.DataSource.DBType}, bounds, h)
}

// dataSourceOf is the data source a compiled query runs on, connection included:
// the one its dataset targets, and only that one is asked for, in the governed
// mode: what runs on it is the compiler's.
func dataSourceOf(ctx context.Context, sources datasource.DataSources, compiled anfranode.CompileToSQLResult) (datasource.DataSource, error) {
	ds, err := connect(ctx, sources, compiled.DataSource.Name, datasource.DataPermModeGoverned)
	if errors.Is(err, datasource.ErrUnknown) {
		return datasource.DataSource{}, fmt.Errorf("data source %q is not defined among the repo's data sources", compiled.DataSource.Name)
	}
	return ds, err
}

// stream fetches st's rows within bounds: canal is asked for no more than
// bounds.Truncate rows, or one row past bounds.MaxRows, and that row fails the
// query before h sees it; and to fail past bounds.MaxBytes itself.
func stream(ctx context.Context, canal *canalquery.Client, ds datasource.DataSource, st canalquery.Statement, bounds ResultBounds, h canalquery.Handler) error {
	st, err := on(ds, st)
	if err != nil {
		return err
	}
	st.MaxResultBytes = bounds.MaxBytes
	maxRows := bounds.MaxRows
	var capped bool
	st.TruncateRows, capped = rowCap(bounds.Truncate, maxRows)
	if capped {
		row, n := h.Row, 0
		h.Row = func(r canalquery.Row) error {
			if n++; n > maxRows {
				return apperr.NewWith(ResultTooLarge,
					fmt.Sprintf("The query's result has more than %d rows, more than this server allows. Narrow the query, or add a limit.", maxRows),
					ResultLimit{MaxRows: maxRows})
			}
			return row(r)
		}
	}
	if err := canal.Stream(ctx, st, h); err != nil {
		if _, formal := err.(*apperr.Error); formal {
			return err // h's own, such as the row maximum's: wrapping it would hide its code
		}
		if e, ok := errors.AsType[*canalquery.Error](err); ok && e.Type == canalquery.TypeResultTooLarge {
			return apperr.EncapsulateWith(err, ResultTooLarge,
				fmt.Sprintf("The query's result is larger than %d MiB, more than this server allows. Narrow the query, or add a limit.", bounds.MaxBytes>>20),
				ResultLimit{MaxBytes: bounds.MaxBytes})
		}
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
