package canalquery

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/jsonkit"
)

// Client talks to a canal-query server over HTTP. Address-based and
// owns no process, so it works against a host-spawned canal sidecar or an
// external one (docker-compose / k8s).
type Client struct {
	baseURL string
	http    *http.Client
	pool    map[string]any // pool_options sent with each query
}

// NewClient builds a client for the canal-query at baseURL. With
// enablePooling, canal-query reuses DB connections across requests from its
// process-global pool — which only pays off when canal-query is long-lived
// (under `anfra serve`); one-shot callers pass false. Sizes mirror the monolith.
func NewClient(baseURL string, enablePooling bool) *Client {
	pool := map[string]any{"enabled": false}
	if enablePooling {
		pool = map[string]any{"enabled": true, "max_total": 10, "max_idle": 5}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{},
		pool:    pool,
	}
}

// BaseURL exposes the canal-query HTTP endpoint for runtimes that own their own
// query client, such as anfra-node catalog ingestion.
func (c *Client) BaseURL() string { return c.baseURL }

// Health does a single /health check (unlike WaitReady, which polls).
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("canal-query health status %d", resp.StatusCode)
	}
	return nil
}

// WaitReady polls /health until canal-query answers or the deadline passes.
func (c *Client) WaitReady(ctx context.Context) error {
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
		// Give up immediately if the caller is done; otherwise an unreachable
		// address blocks for the whole deadline.
		select {
		case <-ctx.Done():
			return fmt.Errorf("canal-query not ready: %w", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Result is the column header + rows of an executed query.
type Result struct {
	Fields []string
	Rows   []Row
}

// Row is one row of a result as canal wrote it: a JSON array, kept as its bytes.
// anfra reads no cell, so it never decodes one: a number keeps every digit
// (decoded, an integer past 2^53 would round), and a row costs one copy.
type Row jsontext.Value

// MarshalJSONTo writes the row as canal wrote it.
func (r Row) MarshalJSONTo(enc *jsontext.Encoder) error { return enc.WriteValue(jsontext.Value(r)) }

// UnmarshalJSONFrom reads one row as it is.
func (r *Row) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	v, err := dec.ReadValue()
	if err != nil {
		return err
	}
	if v.Kind() != '[' {
		return fmt.Errorf("a row is a JSON array, not %s", v.Kind())
	}
	*r = Row(v.Clone())
	return nil
}

// Schema publishes a row as what it is, an array of values of any type.
func (Row) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: huma.TypeArray, Items: &huma.Schema{}}
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
	TruncateRows int            `json:"truncate_rows"` // cap returned rows; negative = no truncation
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

// Error is canal-query's error object: its message, its type, and
// which side it says is responsible. Scope "User" covers both a data source
// canal cannot connect to (its config is the user's) and a query the database
// refused, so it does not say whether the query was at fault.
type Error struct {
	Message string
	Type    string
	Scope   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("canal query error (%s, %s): %s", e.Scope, e.Type, e.Message)
}

func canalError(m map[string]any) *Error {
	str := func(k string) string { s, _ := m[k].(string); return s }
	e := &Error{Message: str("message"), Type: str("type"), Scope: str("scope")}
	if e.Message == "" {
		e.Message = fmt.Sprint(m)
	}
	return e
}

// Execute runs SQL against a data source (dbtype + dbconfig) and returns the
// rows. dbconfig is passed straight through to canal as the connection config.
// truncateRows caps how many rows canal returns (negative = no truncation).
func (c *Client) Execute(ctx context.Context, dbtype string, dbconfig map[string]any, sql string, truncateRows int) (*Result, error) {
	body, err := jsonkit.Marshal(queryRequest{
		SQL:          sql,
		Dbtype:       dbtype,
		Dbconfig:     dbconfig,
		Dbsetting:    map[string]any{},
		PoolOptions:  c.pool,
		TenantID:     1,
		Job:          queryJob{ID: -1, CreatedAt: time.Now().UTC().Format(time.RFC3339)},
		SkipCache:    true,
		StreamResult: true,
		TruncateRows: truncateRows,
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
		return nil, sidecar.Unreachable(ctx, Name, fmt.Errorf("canal query: %w", err))
	}
	defer resp.Body.Close()

	result := &Result{}
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
			if err := jsonkit.Unmarshal(line, &tr); err == nil && tr.HolisticsTrailer {
				if len(tr.Error) > 0 {
					return nil, canalError(tr.Error)
				}
				if tr.Metadata != nil {
					result.Fields = tr.Metadata.Fields
				}
				continue
			}
			return nil, fmt.Errorf("canal query failed (status %d): %s", resp.StatusCode, line)
		}
		if line[0] != '[' || !jsonkit.Valid(line) {
			return nil, fmt.Errorf("canal answered a row that is not a JSON array: %.100s", line)
		}
		result.Rows = append(result.Rows, Row(bytes.Clone(line))) // the scanner reuses line
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read result stream: %w", err)
	}
	return result, nil
}
