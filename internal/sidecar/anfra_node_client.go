package sidecar

import (
	"bytes"
	"context"
	"encoding/json"
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
		time.Sleep(20 * time.Millisecond)
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
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
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
		return fmt.Errorf("rpc %s: %w", method, err)
	}
	defer resp.Body.Close()

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("rpc %s error %d: %s", method, rpcResp.Error.Code, rpcResp.Error.Message)
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
	RepoPath    string                       `json:"repoPath"`
	DatasetFqn  string                       `json:"datasetFqn"`
	AQL         string                       `json:"aql"`
	DataSources map[string]CompileDataSource `json:"dataSources"`
}

type CompileToSQLResult struct {
	SQL        string            `json:"sql"`
	DataSource CompileDataSource `json:"dataSource"` // the data source the SQL targets (for execution routing)
}

// CompileToSQL compiles an AQL query against a dataset into dialect SQL.
func (c *AnfraNodeClient) CompileToSQL(ctx context.Context, req CompileToSQLRequest) (CompileToSQLResult, error) {
	var res CompileToSQLResult
	err := c.Call(ctx, "aql.compile_to_sql", req, &res)
	return res, err
}
