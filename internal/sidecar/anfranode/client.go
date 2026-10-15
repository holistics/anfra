package anfranode

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/jsonkit"
)

// Client talks to an anfra-node over JSON-RPC. It is address-based and
// owns no process, so it works against a host-spawned sidecar (Unix socket) or
// an external one reachable over TCP (docker-compose / k8s).
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClientUnix dials a host-spawned sidecar over its Unix socket.
func NewClientUnix(socketPath string) *Client {
	return &Client{
		baseURL: "http://unix",
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// NewClientHTTP connects to an anfra-node reachable over TCP, e.g. a
// docker-compose / k8s service. No process is owned.
func NewClientHTTP(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{}}
}

// RepoRef names a request's repo to anfra-node: its compile-cache identity, and
// exactly one of where its files are, a directory (RepoPath) or a revision a host
// keeps (Revision, "<repo>@<sha>"). RefTo builds it from a repo; every
// repo-scoped call checks it before it leaves the process, so a caller that
// sends neither or both fails here, with a stack that names it, rather than with
// an error off the wire.
//
// anfra-node holds no repo identity of its own (one process can serve many
// repos), so RepoID is required. It is the repo's stable identity, never the
// revision: anfra-node's compile cache is an incremental seed keyed by it, made
// exactly the requested files on every request, so a stable key keeps commits
// incremental and the content keeps it correct.
type RepoRef struct {
	RepoID   string `json:"repoId"`
	RepoPath string `json:"repoPath,omitempty"`
	Revision string `json:"revision,omitempty"`
}

// RefTo is r as anfra-node names it.
func RefTo(r repo.Repo) RepoRef {
	ref := RepoRef{RepoID: r.ID()}
	if local, ok := r.Local(); ok {
		ref.RepoPath = local.Dir
	}
	if rev, ok := r.Revision(); ok {
		ref.Revision = rev
	}
	return ref
}

func (r RepoRef) check() error {
	switch {
	case r.RepoID == "":
		return errors.New("anfra-node: RepoID is required on repo-scoped requests; " +
			"anfra-node keeps no repo identity of its own, so the caller names the repo " +
			"(tenant-qualified where the deployment is shared)")
	case (r.RepoPath == "") == (r.Revision == ""):
		return errors.New("anfra-node: a repo-scoped request names exactly one of RepoPath and Revision; build it with RefTo")
	}
	return nil
}

// WaitReady polls /health until the sidecar answers or the deadline passes.
func (c *Client) WaitReady(ctx context.Context) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
		resp, err := c.http.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("anfra-node not ready within deadline")
		}
		// Give up immediately if the caller is done; otherwise an unreachable
		// address blocks for the whole deadline.
		select {
		case <-ctx.Done():
			return fmt.Errorf("anfra-node not ready: %w", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	Result jsontext.Value `json:"result"`
	Error  *struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    jsontext.Value `json:"data"`
	} `json:"error"`
}

// RPCInvalidParams is the JSON-RPC code anfra-node answers a request it
// refuses as invalid input with (its InvalidInputError).
const RPCInvalidParams = -32602

// RPCError is an error anfra-node answered a call with. Data is the error's
// JSON-RPC data, when it carries one: for invalid input, the {path} of what is
// wrong, e.g. a Query Input entry.
type RPCError struct {
	Method  string
	Code    int
	Message string
	Data    jsontext.Value
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc %s error %d: %s", e.Method, e.Code, e.Message)
}

// Path is where invalid input is wrong, from the error's data, e.g. a Query
// Input entry's "filters[2].operator"; "" for the input as a whole. ok is false
// when the error does not say: not invalid input, or no path in its data.
func (e *RPCError) Path() (path string, ok bool) {
	var d struct {
		Path *string `json:"path"`
	}
	if e.Code != RPCInvalidParams || jsonkit.Unmarshal(e.Data, &d) != nil || d.Path == nil {
		return "", false
	}
	return *d.Path, true
}

// Call invokes a JSON-RPC method and unmarshals the result into out (if non-nil).
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	body, err := jsonkit.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/rpc", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return sidecar.Unreachable(ctx, Name, fmt.Errorf("rpc %s: %w", method, err))
	}
	defer resp.Body.Close()

	var rpcResp rpcResponse
	if err := jsonkit.UnmarshalRead(resp.Body, &rpcResp); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if rpcResp.Error != nil {
		// TODO: classify anfra-node's errors. They reach a host unclassified, so as
		// internal_server_error: an unknown dataset or data source, or a model that
		// does not compile, is the caller's, not an outage. anfra-node should
		// return a stable error code per kind (in rpcResp.Error.Code or its data),
		// mapped here to engine codes — not.found-like and invalid-input-like ones,
		// with the AML location where there is one — and added to errcode.
		return &RPCError{Method: method, Code: rpcResp.Error.Code, Message: rpcResp.Error.Message, Data: rpcResp.Error.Data}
	}
	if out != nil && rpcResp.Result != nil {
		return jsonkit.Unmarshal(rpcResp.Result, out)
	}
	return nil
}

// Ping is the liveness check.
func (c *Client) Ping(ctx context.Context) (map[string]any, error) {
	var res map[string]any
	err := c.Call(ctx, "ping", nil, &res)
	return res, err
}

// CompileDataSource is the name->dialect entry the sidecar needs to compile SQL.
// Connection/credentials are NOT part of this — they stay host-side.
type CompileDataSource struct {
	Name   string `json:"name"`
	DBType string `json:"dbtype"`
}

// CompileDataSources is the repo's data sources as the sidecar takes them: a
// name and a dialect each, without their connections.
func CompileDataSources(dialects []datasource.Dialect) map[string]CompileDataSource {
	out := make(map[string]CompileDataSource, len(dialects))
	for _, d := range dialects {
		out[d.Name] = CompileDataSource{Name: d.Name, DBType: d.DBType}
	}
	return out
}

// CompileToSQLRequest / Result mirror the sidecar's aql.compile_to_sql method.
type CompileToSQLRequest struct {
	RepoRef
	DatasetFqn  string                       `json:"datasetFqn"`
	AQL         string                       `json:"aql"`
	DataSources map[string]CompileDataSource `json:"dataSources"`
	// Input is the query's Query Input, applied by rewriting the AQL before it
	// compiles. Nil: none.
	Input *QueryTransforms `json:"input,omitempty"`
	// Pagination asks for one page of rows, compiled into LIMIT/OFFSET. Nil: every
	// row. Refused for a pivot query.
	Pagination *Pagination     `json:"pagination,omitempty"`
	Options    *CompileOptions `json:"options,omitempty"`
}

// QueryTransforms is a query's Query Input: the structured transforms it
// carries on one run (a Data App's control filters, cross-filter conditions,
// sorts and date drills), applied by anfra-node rewriting its AQL before it
// compiles. The core API passes it through in these names, amql's and the anfra
// SDK's: anfra-node checks the values (an operator, a grain).
type QueryTransforms struct {
	Filters    []QueryFilter    `json:"filters,omitempty" doc:"conditions on fields, ANDed with the query's own filters"`
	Conditions []QueryCondition `json:"conditions,omitempty" doc:"AQL conditions ANDed with the query's own filters"`
	Sorts      []QuerySort      `json:"sorts,omitempty" doc:"sorts by result column, replacing the query's own"`
	DateDrills []QueryDateDrill `json:"dateDrills,omitempty" doc:"date fields redrawn at another grain, their columns' names kept"`
}

type QueryFilter struct {
	Field       string `json:"field" doc:"model.field for a dataset field, or the name of a dataset metric"`
	Operator    string `json:"operator" doc:"the operator, such as is, contains, between, last"`
	Values      []any  `json:"values" doc:"the operator's values, by the field's type: a number as a string in JSON's number syntax, such as \"9007199254740993\" (a JSON number is refused, since it may have lost digits before it was sent); true or false; a date or text as a string. Empty for an operator that takes none"`
	Modifier    string `json:"modifier,omitempty" doc:"the date unit of a relative operator, such as day"`
	Aggregation string `json:"aggregation,omitempty" doc:"the condition applies to this aggregate of field, such as sum"`
}

type QueryCondition struct {
	Expr string `json:"expr" doc:"an AQL condition"`
}

type QuerySort struct {
	Field     string `json:"field" doc:"a result column's name"`
	Direction string `json:"direction" enum:"asc,desc" doc:"the direction"`
}

type QueryDateDrill struct {
	Field string `json:"field" doc:"model.field: a date field"`
	Grain string `json:"grain" doc:"the grain, such as month"`
}

// Pagination is one 1-based page of rows.
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// CompileOptions are the compile options anfra sets. TimezoneRegion is an IANA
// zone, for relative dates and date truncation.
type CompileOptions struct {
	TimezoneRegion string `json:"timezoneRegion,omitempty"`
}

type CompileToSQLResult struct {
	SQL string `json:"sql"`
	// AQL is the query that compiled: the AQL with its Query Input applied.
	AQL        string            `json:"aql"`
	DataSource CompileDataSource `json:"dataSource"` // the data source the SQL targets (for execution routing)
	// Columns describes each output column of an explore query; absent for other
	// query shapes.
	Columns []ExploreColumn `json:"columns,omitempty"`
}

// ExploreColumn is one column of a query's answer: the key its values come back
// under, and the dataset field it draws, in the names AQL uses in the dataset.
// anfra-node describes an explore query's; the core API passes them through in
// these names, amql's and the anfra SDK's.
type ExploreColumn struct {
	Name        string `json:"name" doc:"the column's key, as in fields"`
	FieldName   string `json:"fieldName" doc:"the field it draws; for an adhoc column, its name"`
	ModelID     string `json:"modelId,omitempty" doc:"the model of fieldName, as the dataset names it; absent for a dataset metric or an adhoc column"`
	Label       string `json:"label"`
	Adhoc       bool   `json:"adhoc" doc:"a query-local expression, not a field the dataset defines"`
	IsMeasure   bool   `json:"isMeasure"`
	Aggregation string `json:"aggregation,omitempty" doc:"an aggregated field's aggregation, such as sum or count distinct"`
	// Type is the AMQL type the type checker gives the column's expression
	// (number, text, date, ...), not the type its field declares: a count of a
	// text field is a number. Absent from an anfra-node before 0.4.0.
	Type string `json:"type,omitempty"`
}

// CompileToSQL compiles an AQL query against a dataset into dialect SQL.
func (c *Client) CompileToSQL(ctx context.Context, req CompileToSQLRequest) (CompileToSQLResult, error) {
	var res CompileToSQLResult
	if err := req.RepoRef.check(); err != nil {
		return res, err
	}
	err := c.Call(ctx, "aql.compile_to_sql", req, &res)
	return res, err
}

// ValidateAMLRequest mirrors the sidecar's aml.validate params. Paths are
// file/dir/glob selectors relative to the repo; empty validates the whole repo.
// No data sources are involved — this type-checks AML.
type ValidateAMLRequest struct {
	RepoRef
	Paths []string `json:"paths,omitempty"`
}

// CompileError is an AML file that failed to compile (interpret). Row/Col are
// 1-based when a location is known.
type CompileError struct {
	FilePath string `json:"filePath,omitempty"`
	Message  string `json:"message"`
	Row      *int   `json:"row,omitempty"`
	Col      *int   `json:"col,omitempty"`
}

// ReportTraceLoc / ReportTrace locate a validator finding within the AML source.
type ReportTraceLoc struct {
	FilePath string `json:"filePath"`
	Line     *int   `json:"line,omitempty"`
	Column   *int   `json:"column,omitempty"`
}

type ReportTrace struct {
	Kind string          `json:"kind"`
	Name string          `json:"name"`
	Loc  *ReportTraceLoc `json:"loc,omitempty"`
}

// ValidationReport is one finding from the validator suite. Severity is
// "error", "warning", or "info".
type ValidationReport struct {
	Validator string        `json:"validator"`
	FilePath  string        `json:"filePath"`
	Type      string        `json:"type"`
	Fqn       string        `json:"fqn"`
	Severity  string        `json:"severity"`
	Message   string        `json:"message"`
	Code      any           `json:"code,omitzero"`
	Trace     []ReportTrace `json:"trace,omitempty"`
}

// ValidateAMLResult mirrors the sidecar's aml.validate result: files that failed
// to compile, plus the validator suite's findings.
type ValidateAMLResult struct {
	CompileErrors []CompileError     `json:"compileErrors"`
	Reports       []ValidationReport `json:"reports"`
}

// ValidateAML validates an AML repo (compile + validator suite) for the selected
// paths and returns the findings.
func (c *Client) ValidateAML(ctx context.Context, req ValidateAMLRequest) (ValidateAMLResult, error) {
	var res ValidateAMLResult
	if err := req.RepoRef.check(); err != nil {
		return res, err
	}
	err := c.Call(ctx, "aml.validate", req, &res)
	return res, err
}

// AQLDiagnostic is one AQL type-check finding. Severity is "error" or "warning";
// Line/Column are 1-based positions when known.
type AQLDiagnostic struct {
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Line     *int   `json:"line,omitempty"`
	Column   *int   `json:"column,omitempty"`
}

type ValidateAQLResult struct {
	Diagnostics []AQLDiagnostic `json:"diagnostics"`
}

// ValidateAQL type-checks a single AQL query against a dataset (same inputs as
// CompileToSQL) and returns its diagnostics instead of throwing on the first error.
func (c *Client) ValidateAQL(ctx context.Context, req CompileToSQLRequest) (ValidateAQLResult, error) {
	var res ValidateAQLResult
	if err := req.RepoRef.check(); err != nil {
		return res, err
	}
	err := c.Call(ctx, "aql.validate", req, &res)
	return res, err
}

// CatalogIngestRequest mirrors anfra-node's catalog.ingest params: the
// catalog's context, and optionally the one context source to ingest.
// anfra-node owns source parsing and ingestion internals.
type CatalogIngestRequest struct {
	Context CatalogContext `json:"context"`
	Source  string         `json:"source,omitempty"`
}

// CatalogContext is the repo a catalog call is about, as anfra-node needs it:
// the repo, named as every repo-scoped call names it (a directory, or a revision
// a host keeps), and the canal-query to reach its context sources through.
// anfra-node reads the context sources from the repo itself
// (.anfra/context_sources.yml), as it reads its AML, so a managed repo, which has
// no directory, needs no path either. catalog.ingest, catalog.search and
// catalog.lineage all take it, so search and lineage can ingest on their own and
// need no catalog.ingest first.
type CatalogContext struct {
	RepoRef
	CanalQueryBaseURL string `json:"canalQueryBaseUrl"`
}

// CatalogContextOf is r's catalog context, its context sources reached through
// the canal-query at canalQueryBaseURL.
func CatalogContextOf(r repo.Repo, canalQueryBaseURL string) CatalogContext {
	return CatalogContext{RepoRef: RefTo(r), CanalQueryBaseURL: canalQueryBaseURL}
}

// IngestCatalog builds the local search catalog through anfra-node. The
// anfra-node method is a void RPC: success means the catalog was written, while
// failures are returned through the JSON-RPC error channel.
func (c *Client) IngestCatalog(ctx context.Context, req CatalogIngestRequest) error {
	if err := req.Context.check(); err != nil {
		return err
	}
	return c.Call(ctx, "catalog.ingest", req, nil)
}

// CatalogSearchRequest mirrors anfra-node's catalog.search params. The Go host
// supplies the catalog's context; anfra-node owns search parsing, planning, and
// result shaping.
type CatalogSearchRequest struct {
	Context CatalogContext `json:"context"`
	Query   string         `json:"query"`
}

type CatalogSearchResult map[string]any

// SearchCatalog searches the active local catalog through anfra-node.
func (c *Client) SearchCatalog(ctx context.Context, req CatalogSearchRequest) (CatalogSearchResult, error) {
	var res CatalogSearchResult
	if err := req.Context.check(); err != nil {
		return res, err
	}
	err := c.Call(ctx, "catalog.search", req, &res)
	return res, err
}
