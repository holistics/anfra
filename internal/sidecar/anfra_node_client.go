package sidecar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// AnfraNodeClient talks to an anfra-node over JSON-RPC. It is address-based and
// owns no process, so it works against a host-spawned sidecar (Unix socket) or
// an external one reachable over TCP (docker-compose / k8s).
type AnfraNodeClient struct {
	baseURL string
	http    *http.Client
}

// NewAnfraNodeClientUnix dials a host-spawned sidecar over its Unix socket.
func NewAnfraNodeClientUnix(socketPath string) *AnfraNodeClient {
	return &AnfraNodeClient{
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

// NewAnfraNodeClientHTTP connects to an anfra-node reachable over TCP, e.g. a
// docker-compose / k8s service. No process is owned.
func NewAnfraNodeClientHTTP(baseURL string) *AnfraNodeClient {
	return &AnfraNodeClient{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{}}
}

// checkRepoID guards the repo-scoped RPCs. anfra-node holds no repo identity of
// its own — one process can serve many repos — so it requires RepoID on every
// such request and rejects it as INVALID_PARAMS otherwise. Checking here too
// means a caller that forgets fails in its own process, with a stack that names
// it, rather than reading an error off the wire.
func checkRepoID(repoID string) error {
	if repoID == "" {
		return errors.New("anfra-node: RepoID is required on repo-scoped requests; " +
			"anfra-node keeps no repo identity of its own, so the caller names the repo " +
			"(tenant-qualified where the deployment is shared)")
	}
	return nil
}

// WaitReady polls /health until the sidecar answers or the deadline passes.
func (c *AnfraNodeClient) WaitReady(ctx context.Context) error {
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
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
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
	Data    json.RawMessage
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
	if e.Code != RPCInvalidParams || json.Unmarshal(e.Data, &d) != nil || d.Path == nil {
		return "", false
	}
	return *d.Path, true
}

// Call invokes a JSON-RPC method and unmarshals the result into out (if non-nil).
func (c *AnfraNodeClient) Call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
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
		return unreachable(ctx, "anfra-node", fmt.Errorf("rpc %s: %w", method, err))
	}
	defer resp.Body.Close()

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
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
		return json.Unmarshal(rpcResp.Result, out)
	}
	return nil
}

// Ping is the liveness check.
func (c *AnfraNodeClient) Ping(ctx context.Context) (map[string]any, error) {
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

// CompileToSQLRequest / Result mirror the sidecar's aql.compile_to_sql method.
type CompileToSQLRequest struct {
	RepoPath string `json:"repoPath"`
	// RepoID is the sidecar's compile-cache identity for this repo. One sidecar
	// may serve many repos (and, when shared, many tenants), so it travels per
	// request rather than per process. Omitted, the sidecar derives one from
	// RepoPath.
	RepoID      string                       `json:"repoId,omitempty"`
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
// compiles. The core API passes it through in these names, amql's and the Anfra
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
	Values      []any  `json:"values" doc:"the operator's values: strings, numbers or booleans; empty for an operator that takes none"`
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
// these names, amql's and the Anfra SDK's.
type ExploreColumn struct {
	Name        string `json:"name" doc:"the column's key, as in fields"`
	FieldName   string `json:"fieldName" doc:"the field it draws; for an adhoc column, its name"`
	ModelID     string `json:"modelId,omitempty" doc:"the model of fieldName, as the dataset names it; absent for a dataset metric or an adhoc column"`
	Label       string `json:"label"`
	Adhoc       bool   `json:"adhoc" doc:"a query-local expression, not a field the dataset defines"`
	IsMeasure   bool   `json:"isMeasure"`
	Aggregation string `json:"aggregation,omitempty" doc:"an aggregated field's aggregation, such as sum or count distinct"`
}

// CompileToSQL compiles an AQL query against a dataset into dialect SQL.
func (c *AnfraNodeClient) CompileToSQL(ctx context.Context, req CompileToSQLRequest) (CompileToSQLResult, error) {
	var res CompileToSQLResult
	if err := checkRepoID(req.RepoID); err != nil {
		return res, err
	}
	err := c.Call(ctx, "aql.compile_to_sql", req, &res)
	return res, err
}

// ValidateAMLRequest mirrors the sidecar's aml.validate params. Paths are
// file/dir/glob selectors relative to the repo; empty validates the whole repo.
// No data sources are involved — this type-checks AML.
type ValidateAMLRequest struct {
	RepoPath string `json:"repoPath"`
	// RepoID is the compile-cache identity; see CompileToSQLRequest.RepoID.
	RepoID string   `json:"repoId,omitempty"`
	Paths  []string `json:"paths,omitempty"`
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
	Code      any           `json:"code,omitempty"`
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
func (c *AnfraNodeClient) ValidateAML(ctx context.Context, req ValidateAMLRequest) (ValidateAMLResult, error) {
	var res ValidateAMLResult
	if err := checkRepoID(req.RepoID); err != nil {
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
func (c *AnfraNodeClient) ValidateAQL(ctx context.Context, req CompileToSQLRequest) (ValidateAQLResult, error) {
	var res ValidateAQLResult
	if err := checkRepoID(req.RepoID); err != nil {
		return res, err
	}
	err := c.Call(ctx, "aql.validate", req, &res)
	return res, err
}

// CatalogIngestRequest mirrors anfra-node's catalog.ingest params. The Go host
// starts sidecars and passes runtime paths/URLs; anfra-node owns source parsing
// and ingestion internals.
type CatalogIngestRequest struct {
	RepoPath          string `json:"repoPath"`
	SourcesPath       string `json:"sourcesPath"`
	CatalogPath       string `json:"catalogPath"`
	CanalQueryBaseURL string `json:"canalQueryBaseUrl"`
	Source            string `json:"source,omitempty"`
}

// IngestCatalog builds the local search catalog through anfra-node. The
// anfra-node method is a void RPC: success means the catalog was written, while
// failures are returned through the JSON-RPC error channel.
func (c *AnfraNodeClient) IngestCatalog(ctx context.Context, req CatalogIngestRequest) error {
	return c.Call(ctx, "catalog.ingest", req, nil)
}

// CatalogSearchRequest mirrors anfra-node's catalog.search params. The Go host
// supplies the local catalog path and canal-query URL; anfra-node owns search
// parsing, planning, and result shaping.
type CatalogSearchRequest struct {
	CatalogPath       string `json:"catalogPath"`
	CanalQueryBaseURL string `json:"canalQueryBaseUrl"`
	Query             string `json:"query"`
}

type CatalogSearchResult map[string]any

// SearchCatalog searches the active local catalog through anfra-node.
func (c *AnfraNodeClient) SearchCatalog(ctx context.Context, req CatalogSearchRequest) (CatalogSearchResult, error) {
	var res CatalogSearchResult
	err := c.Call(ctx, "catalog.search", req, &res)
	return res, err
}
