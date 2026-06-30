package sidecar

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CanalQueryClient talks to a canal-query server over HTTP. Address-based and
// owns no process, so it works against a host-spawned canal sidecar or an
// external one (docker-compose / k8s).
type CanalQueryClient struct {
	baseURL string
	http    *http.Client
	pool    map[string]any // pool_options sent with each query
}

// NewCanalQueryClient builds a client for the canal-query at baseURL. With
// enablePooling, canal-query reuses DB connections across requests from its
// process-global pool — which only pays off when canal-query is long-lived
// (under `anfra serve`); one-shot callers pass false. Sizes mirror the monolith.
func NewCanalQueryClient(baseURL string, enablePooling bool) *CanalQueryClient {
	pool := map[string]any{"enabled": false}
	if enablePooling {
		pool = map[string]any{"enabled": true, "max_total": 10, "max_idle": 5}
	}
	return &CanalQueryClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{},
		pool:    pool,
	}
}

// WaitReady polls /health until canal-query answers or the deadline passes.
func (c *CanalQueryClient) WaitReady(ctx context.Context) error {
	deadline := time.Now().Add(15 * time.Second)
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
			return fmt.Errorf("canal-query not ready within deadline")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// QueryResult is the column header + rows of an executed query.
type QueryResult struct {
	Fields []string
	Rows   [][]any
}

type queryJob struct {
	ID        int    `json:"id"`
	CreatedAt string `json:"created_at"`
}

// queryRequest mirrors canal-query's POST /query params. We use skip_cache +
// stream_result: all rows stream back as newline-delimited JSON arrays, ending
// with a trailer object — no cache/lake involved.
type queryRequest struct {
	SQL          string         `json:"sql"`
	Dbtype       string         `json:"dbtype"`
	Dbconfig     map[string]any `json:"dbconfig"`
	Dbsetting    map[string]any `json:"dbsetting"`
	PoolOptions  map[string]any `json:"pool_options"`
	TenantID     int            `json:"tenant_id"`
	Job          queryJob       `json:"job"`
	SkipCache    bool           `json:"skip_cache"`
	StreamResult bool           `json:"stream_result"`
}

type streamTrailer struct {
	HolisticsTrailer bool `json:"__holistics_trailer__"`
	Metadata         *struct {
		Fields      []string `json:"fields"`
		RecordCount int      `json:"record_count"`
	} `json:"metadata"`
	HTTPCode int            `json:"http_code"`
	Error    map[string]any `json:"error"`
}

// Execute runs SQL against a data source (dbtype + dbconfig) and returns the
// rows. dbconfig is passed straight through to canal as the connection config.
func (c *CanalQueryClient) Execute(ctx context.Context, dbtype string, dbconfig map[string]any, sql string) (*QueryResult, error) {
	body, err := json.Marshal(queryRequest{
		SQL:          sql,
		Dbtype:       dbtype,
		Dbconfig:     dbconfig,
		Dbsetting:    map[string]any{},
		PoolOptions:  c.pool,
		TenantID:     1,
		Job:          queryJob{ID: -1, CreatedAt: time.Now().UTC().Format(time.RFC3339)},
		SkipCache:    true,
		StreamResult: true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/query", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("canal query: %w", err)
	}
	defer resp.Body.Close()

	result := &QueryResult{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024) // rows can be large
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if line[0] == '{' {
			// Trailer (stream end) or, on a non-streamed error response, the error object.
			var tr streamTrailer
			if err := json.Unmarshal(line, &tr); err == nil && tr.HolisticsTrailer {
				if len(tr.Error) > 0 {
					return nil, fmt.Errorf("canal query error: %v", tr.Error)
				}
				if tr.Metadata != nil {
					result.Fields = tr.Metadata.Fields
				}
				continue
			}
			return nil, fmt.Errorf("canal query failed (status %d): %s", resp.StatusCode, line)
		}
		var row []any
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("parse result row: %w", err)
		}
		result.Rows = append(result.Rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read result stream: %w", err)
	}
	return result, nil
}
