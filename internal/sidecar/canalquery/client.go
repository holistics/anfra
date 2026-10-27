package canalquery

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
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
	// tenant is whom the queries run for (ForTenant): canal-query's tenant_id.
	tenant string
}

// NewClient builds a client for the canal-query at baseURL. With
// enablePooling, canal-query reuses DB connections across requests from its
// process-global pool — which only pays off when canal-query is long-lived
// (under `anfra serve`); one-shot callers pass false. Sizes mirror the monolith.
// A connection idle for an hour is evicted, not after canal-query's default of
// 3 hours: a pool whose data source changed, or went unused, then stops
// holding the warehouse's connections sooner.
func NewClient(baseURL string, enablePooling bool) *Client {
	pool := map[string]any{"enabled": false}
	if enablePooling {
		pool = map[string]any{"enabled": true, "max_total": 10, "max_idle": 5, "min_evictable_idle_s": 60 * 60}
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

// Statement is a query for canal-query to run: SQL in the dialect of DBType, on
// the data source DBConfig connects to, with DBSetting's options (both passed to
// canal as they are; no DBSetting is canal's defaults), answering at most
// TruncateRows rows (negative: all of them), and failing once its result holds
// more than MaxResultBytes (zero: no maximum) with TypeResultTooLarge.
// canal-query counts those bytes as each batch's Arrow data, summed as it
// streams them, not as the JSON it writes; it holds a batch, never the result.
type Statement struct {
	SQL            string
	DBType         string
	DBConfig       map[string]any
	DBSetting      map[string]any
	TruncateRows   int
	MaxResultBytes int64
	// Unpooled runs st on a connection of its own, closed after it, whatever
	// the client's pooling.
	Unpooled bool
}

// TypeResultTooLarge is the Error.Type of a result past its MaxResultBytes.
const TypeResultTooLarge = "ResultTooLarge"

// Progress is how a query's run is going, as canal-query reports it ahead of
// its result. Kind says which of its other fields it has:
//
//   - "step": Step, the step the run has started: connecting_to_dw,
//     executing_query, fetching_query_result, or a child of one, named after it
//     ("executing_query.poll_query");
//   - "dw_query_id": DwQueryID, the query's id on the warehouse;
//   - "dw_bytes_processed": DwBytesProcessed, what the warehouse scanned, once
//     it has run (BigQuery and Athena);
//   - "retry": Attempt, the attempt about to start, after a failed one.
//
// AtMs is when, in milliseconds since canal-query started answering.
type Progress struct {
	Kind             string `json:"kind"`
	AtMs             int64  `json:"at_ms"`
	Step             string `json:"step,omitempty"`
	DwQueryID        string `json:"dw_query_id,omitempty"`
	DwBytesProcessed *int64 `json:"dw_bytes_processed,omitempty"`
	Attempt          int    `json:"attempt,omitempty"`
}

// Header is what a result's rows hold, known before any row: each field's name,
// its type as the database names it (int8, numeric, text, ...), and the Arrow
// type canal-query read it as, by which its values are written. ArrowTypes is
// empty when canal-query sent none.
type Header struct {
	Fields     []string
	Types      []string
	ArrowTypes []ArrowType
}

// ArrowType is an Arrow type as canal-query describes it: Arrow's Type of
// format/Schema.fbs as JSON, "type" the type's name (int, decimal, timestamp,
// ...), its other keys that type's fields in snake_case (bit_width, scale,
// unit, ...), "children" a nested type's, and "label" the type as arrow-go
// spells it. anfra reads only its name, and passes the rest on as canal wrote
// it, so a type or a key canal-query adds reaches callers unchanged.
type ArrowType jsontext.Value

// Name is the type's name, its "type"; "" when it has none.
func (t ArrowType) Name() string {
	var v struct {
		Type string `json:"type"`
	}
	_ = jsonkit.Unmarshal(t, &v)
	return v.Type
}

// MarshalJSONTo writes the type as canal wrote it.
func (t ArrowType) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteValue(jsontext.Value(t))
}

// UnmarshalJSONFrom reads one type as it is.
func (t *ArrowType) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	v, err := dec.ReadValue()
	if err != nil {
		return err
	}
	if v.Kind() != '{' {
		return fmt.Errorf("an Arrow type is a JSON object, not %s", v.Kind())
	}
	*t = ArrowType(v.Clone())
	return nil
}

// Schema publishes a type as an object with a name and a label, and the
// type's own fields.
func (ArrowType) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        huma.TypeObject,
		Description: "the Arrow type the values were read as: Arrow's Type of format/Schema.fbs as JSON, with its fields in snake_case",
		Properties: map[string]*huma.Schema{
			"type":  {Type: huma.TypeString, Description: "the type's name, as Schema.fbs's Type union names it, in snake_case: int, floating_point, decimal, utf8, bool, date, timestamp, list, struct, ..."},
			"label": {Type: huma.TypeString, Description: "the type as arrow-go spells it, such as decimal(38, 2): for people, not to decide on"},
		},
		Required:             []string{"type", "label"},
		AdditionalProperties: true,
	}
}

// Handler receives a result as canal-query streams it: its Progress as it goes,
// its Header, then each Row. An error from any stops the stream and is answered
// as it is. A row is the handler's to keep: it is not reused. Progress may be
// nil.
type Handler struct {
	Progress func(Progress) error
	Header   func(Header) error
	Row      func(Row) error
}

// Result is an executed query's header and rows.
type Result struct {
	Header
	Rows []Row
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

// ForTenant is the client running queries for tenant: canal-query keys its
// warehouse connection pools by it, so that tenants never share a connection,
// and records it on its spans. A client without one runs no query: a host that
// serves several tenants must not have them share pools by omission.
func (c *Client) ForTenant(tenant string) *Client {
	t := *c
	t.tenant = tenant
	return &t
}

// queryRequest mirrors canal-query's POST /query params. We use skip_cache +
// stream_result + a header with Arrow types: a header object, then every row as
// a JSON array, one per line, then a trailer object — no cache/lake involved.
type queryRequest struct {
	SQL                   string                `json:"sql"`
	Dbtype                string                `json:"dbtype"`
	Dbconfig              map[string]any        `json:"dbconfig"`
	Dbsetting             map[string]any        `json:"dbsetting"`
	PoolOptions           map[string]any        `json:"pool_options"`
	TenantID              string                `json:"tenant_id"`
	Job                   queryJob              `json:"job"`
	SkipCache             bool                  `json:"skip_cache"`
	StreamResult          bool                  `json:"stream_result"`
	StreamResultOptions   streamResultOptions   `json:"stream_result_options"`
	JSONOutputDataOptions jsonOutputDataOptions `json:"json_output_data_options"`
	TruncateRows          int                   `json:"truncate_rows"` // cap returned rows; negative = no truncation
	MaxResultBytes        int64                 `json:"max_result_bytes,omitempty"`
}

// streamResultOptions are how canal-query frames a streamed result's rows.
type streamResultOptions struct {
	Header           bool `json:"header"`             // a header line before the rows
	HeaderArrowTypes bool `json:"header_arrow_types"` // with the fields' Arrow types
	Progress         bool `json:"progress"`           // the run's progress, a line an event, ahead of the header
}

// jsonOutputDataOptions are how canal-query writes rows as JSON.
type jsonOutputDataOptions struct {
	// IntegersAsStrings writes integers as strings, as decimals already are: a
	// client that reads JSON numbers as float64 (JavaScript) would round one
	// past 2^53.
	IntegersAsStrings bool `json:"integers_as_strings"`
}

// streamObject is a line of the stream that is not a row: its progress (before
// its header), its header, its trailer (last), or, on a response that is not a
// stream, an error object.
type streamObject struct {
	HolisticsProgress bool           `json:"__holistics_progress__"`
	HolisticsHeader   bool           `json:"__holistics_header__"`
	Fields            []string       `json:"fields"`
	Types             []string       `json:"types"`
	ArrowTypes        []ArrowType    `json:"arrow_types"`
	HolisticsTrailer  bool           `json:"__holistics_trailer__"`
	Error             map[string]any `json:"error"`
}

// Error is canal-query's error object: its message, its type, and
// which side it says is responsible. Scope "User" covers both a data source
// canal cannot connect to (its config is the user's) and a query the database
// refused, so it does not say whether the query was at fault.
//
// Cause is the failure's whole chain, which canal-query sends when
// EXPOSE_ERROR_CAUSES is set, as anfra sets it on the one it spawns. It is for
// the log and for Error, never for Message: a caller may show Message to its
// clients (asQueryFailed does), and the cause names canal-query's internals.
type Error struct {
	Message string
	Type    string
	Scope   string
	Cause   string
}

func (e *Error) Error() string {
	text := e.Message
	if e.Cause != "" {
		text = e.Cause
	}
	return fmt.Sprintf("canal query error (%s, %s): %s", e.Scope, e.Type, text)
}

func canalError(m map[string]any) *Error {
	str := func(k string) string { s, _ := m[k].(string); return s }
	e := &Error{Message: str("message"), Type: str("type"), Scope: str("scope"), Cause: str("cause")}
	if e.Message == "" {
		e.Message = fmt.Sprint(m)
	}
	return e
}

// Execute runs st and answers its result, held whole.
func (c *Client) Execute(ctx context.Context, st Statement) (*Result, error) {
	result := &Result{}
	err := c.Stream(ctx, st, Handler{
		Header: func(h Header) error {
			result.Header = h
			return nil
		},
		Row: func(r Row) error {
			result.Rows = append(result.Rows, r)
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// poolFor is the pool_options st is sent with: the client's, except that an
// unpooled statement, or one on an in-memory DuckDB, is never pooled. Each of
// an in-memory DuckDB's connections is a database of its own, so a pooled one
// would carry one query's tables into another's, and canal-query refuses it.
func (c *Client) poolFor(st Statement) map[string]any {
	if st.Unpooled || st.DBType == "duckdb" && duckdbInMemory(st.DBConfig) {
		return map[string]any{"enabled": false}
	}
	return c.pool
}

// duckdbInMemory reports whether a DuckDB dbconfig opens an in-memory database,
// as canal-query decides it (DuckdbConnector.isInMemory): no path, ":memory:",
// or a path that is only options ("?...", ":memory:?...").
func duckdbInMemory(dbconfig map[string]any) bool {
	path, _ := dbconfig["path"].(string)
	return path == "" || path == ":memory:" || strings.HasPrefix(path, ":memory:?") || strings.HasPrefix(path, "?")
}

// Stream runs st and hands its result to h as canal-query streams it, holding
// none of it. The stream is checked as it goes: a header before any row, rows
// that are JSON arrays, and a trailer at its end. A query's error comes in the
// trailer, which may come with no header before it.
func (c *Client) Stream(ctx context.Context, st Statement, h Handler) error {
	if c.tenant == "" {
		return errors.New("canal-query: no tenant to run the query for (the invocation's TenantID)")
	}
	dbsetting := st.DBSetting
	if dbsetting == nil {
		dbsetting = map[string]any{} // canal-query requires one
	}
	body, err := jsonkit.Marshal(queryRequest{
		SQL:                   st.SQL,
		Dbtype:                st.DBType,
		Dbconfig:              st.DBConfig,
		Dbsetting:             dbsetting,
		PoolOptions:           c.poolFor(st),
		TenantID:              c.tenant,
		Job:                   queryJob{ID: -1, CreatedAt: time.Now().UTC().Format(time.RFC3339)},
		SkipCache:             true,
		StreamResult:          true,
		StreamResultOptions:   streamResultOptions{Header: true, HeaderArrowTypes: true, Progress: true},
		JSONOutputDataOptions: jsonOutputDataOptions{IntegersAsStrings: true},
		TruncateRows:          st.TruncateRows,
		MaxResultBytes:        st.MaxResultBytes,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/query", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return sidecar.Unreachable(ctx, Name, fmt.Errorf("canal query: %w", err))
	}
	defer resp.Body.Close()

	headed, ended := false, false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024) // rows can be large
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		switch {
		case len(line) == 0:
			continue
		case ended:
			return fmt.Errorf("canal-query answered after its trailer: %.100s", line)
		case line[0] == '{':
			var obj streamObject
			if err := jsonkit.Unmarshal(line, &obj); err != nil {
				return fmt.Errorf("canal query failed (status %d): %s", resp.StatusCode, line)
			}
			switch {
			case obj.HolisticsProgress: // before the header, but a late one is no reason to fail the query
				if h.Progress != nil {
					var p Progress
					if err := jsonkit.Unmarshal(line, &p); err != nil {
						return fmt.Errorf("canal-query reported unreadable progress: %w", err)
					}
					if err := h.Progress(p); err != nil {
						return err
					}
				}
			case obj.HolisticsTrailer && len(obj.Error) > 0:
				return canalError(obj.Error)
			case obj.HolisticsTrailer && !headed:
				return errors.New("canal-query answered no header before its trailer")
			case obj.HolisticsTrailer:
				ended = true
			case obj.HolisticsHeader && headed:
				return errors.New("canal-query answered a second header")
			case obj.HolisticsHeader && len(obj.ArrowTypes) > 0 && len(obj.ArrowTypes) != len(obj.Fields):
				return fmt.Errorf("canal-query answered %d Arrow types for %d fields", len(obj.ArrowTypes), len(obj.Fields))
			case obj.HolisticsHeader:
				headed = true
				if h.Header != nil {
					if err := h.Header(Header{Fields: obj.Fields, Types: obj.Types, ArrowTypes: obj.ArrowTypes}); err != nil {
						return err
					}
				}
			default:
				return fmt.Errorf("canal query failed (status %d): %s", resp.StatusCode, line)
			}
		case !headed:
			return fmt.Errorf("canal-query answered a row before its header: %.100s", line)
		case line[0] != '[' || !jsonkit.Valid(line):
			return fmt.Errorf("canal answered a row that is not a JSON array: %.100s", line)
		default:
			if err := h.Row(Row(bytes.Clone(line))); err != nil { // the scanner reuses line
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read result stream: %w", err)
	}
	if !ended {
		return errors.New("canal-query's answer ended before its trailer")
	}
	return nil
}
