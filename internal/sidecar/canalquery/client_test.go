package canalquery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/jsonkit"
)

// canal-query's error object is read into a typed error, whatever it says.
func TestCanalError(t *testing.T) {
	e := canalError(map[string]any{"message": "run query: boom", "scope": "User", "type": "GenericError"})
	if e.Message != "run query: boom" || e.Scope != "User" || e.Type != "GenericError" {
		t.Errorf("read %+v", e)
	}
	if e := canalError(map[string]any{"code": 7}); e.Message == "" {
		t.Error("an error object with no message reads as an empty message")
	}
}

// A cause, when canal-query sends one, is the error's text, never its message.
func TestCanalErrorCause(t *testing.T) {
	e := canalError(map[string]any{"message": "run query: internal error", "scope": "Server", "type": "GenericError", "cause": "run query: nil pointer"})
	if e.Message != "run query: internal error" {
		t.Errorf("message %q, want canal's message", e.Message)
	}
	if want := "canal query error (Server, GenericError): run query: nil pointer"; e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

const (
	header = `{"__holistics_header__":true,"fields":["id","amount","attrs","flag"],"types":["int8","numeric","jsonb","bool"],` +
		`"arrow_types":[{"type":"int","bit_width":64,"is_signed":true,"label":"int64"},` +
		`{"type":"decimal","precision":38,"scale":2,"bit_width":128,"label":"decimal(38, 2)"},` +
		`{"type":"utf8","label":"utf8"},{"type":"bool","label":"bool"}]}`
	trailer = `{"__holistics_trailer__":true,"metadata":{"record_count":2},"http_code":200}`
)

// streaming answers each query with lines, as canal-query streams a result, and
// keeps the request it was sent.
func streaming(t *testing.T, lines ...string) (*Client, *map[string]any) {
	t.Helper()
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = jsonkit.Unmarshal(body, &sent)
		_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, false).ForTenant("org_1"), &sent
}

// A pooling client pools a query, except an unpooled one, or one on an
// in-memory DuckDB, whose every connection is a database of its own.
func TestPoolOptions(t *testing.T) {
	canal := NewClient("http://canal", true)
	for _, tc := range []struct {
		dbtype   string
		dbconfig map[string]any
		pooled   bool
		unpooled bool
	}{
		{"postgres", map[string]any{"host": "h"}, true, false},
		{"postgres", map[string]any{"host": "h"}, false, true},
		{"duckdb", map[string]any{"path": "data/shop.duckdb"}, true, false},
		{"duckdb", map[string]any{"path": "data/shop.duckdb"}, false, true},
		{"duckdb", map[string]any{"path": "/abs/shop.duckdb?access_mode=read_only"}, true, false},
		{"duckdb", nil, false, false},
		{"duckdb", map[string]any{"path": ""}, false, false},
		{"duckdb", map[string]any{"path": ":memory:"}, false, false},
		{"duckdb", map[string]any{"path": ":memory:?threads=4"}, false, false},
		{"duckdb", map[string]any{"path": "?threads=4"}, false, false},
	} {
		got := canal.poolFor(Statement{DBType: tc.dbtype, DBConfig: tc.dbconfig, Unpooled: tc.unpooled})
		if got["enabled"] != tc.pooled {
			t.Errorf("%s %v, unpooled %v: pool %v, want enabled=%v", tc.dbtype, tc.dbconfig, tc.unpooled, got, tc.pooled)
		}
	}
	if got := canal.poolFor(Statement{DBType: "postgres"}); got["min_evictable_idle_s"] != 60*60 {
		t.Errorf("pool %v, want connections idle for an hour evicted", got)
	}
}

var statement = Statement{SQL: "select 1", DBType: "postgres", TruncateRows: -1}

// anfra asks for a stream with a header first, with the fields' Arrow types,
// and integers as strings, as decimals are, so a JavaScript client keeps every
// digit; for the client's tenant, as a string.
func TestRequest(t *testing.T) {
	canal, sent := streaming(t, header, trailer)
	if _, err := canal.Execute(context.Background(), statement); err != nil {
		t.Fatal(err)
	}
	stream, _ := (*sent)["stream_result_options"].(map[string]any)
	output, _ := (*sent)["json_output_data_options"].(map[string]any)
	if (*sent)["stream_result"] != true || stream["header"] != true || stream["header_arrow_types"] != true ||
		output["integers_as_strings"] != true || (*sent)["tenant_id"] != "org_1" {
		t.Errorf("request = %v", *sent)
	}
	if setting, ok := (*sent)["dbsetting"].(map[string]any); !ok || len(setting) != 0 {
		t.Errorf("dbsetting = %v, want an empty object for canal's defaults", (*sent)["dbsetting"])
	}
}

// A statement's settings reach canal-query as its dbsetting, unchanged.
func TestRequestSettings(t *testing.T) {
	canal, sent := streaming(t, header, trailer)
	st := statement
	st.DBSetting = map[string]any{"statement_timeout_ms": 60000, "require_ssl": true}
	if _, err := canal.Execute(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	setting, _ := (*sent)["dbsetting"].(map[string]any)
	if setting["statement_timeout_ms"] != float64(60000) || setting["require_ssl"] != true || len(setting) != 2 {
		t.Errorf("dbsetting = %v", (*sent)["dbsetting"])
	}
}

// A client without a tenant runs no query: a host serving several tenants must
// not have them share canal-query's connection pools by omission.
func TestNoTenantNoQuery(t *testing.T) {
	canal, sent := streaming(t, header, trailer)
	_, err := NewClient("http://127.0.0.1:1", false).Execute(context.Background(), statement)
	if err == nil || !strings.Contains(err.Error(), "no tenant") {
		t.Errorf("err = %v, want refused for no tenant", err)
	}
	if _, err := canal.ForTenant("org_2").Execute(context.Background(), statement); err != nil || (*sent)["tenant_id"] != "org_2" {
		t.Errorf("ForTenant: %v, sent %v", err, (*sent)["tenant_id"])
	}
}

// The header reaches the handler before any row, its types with it. Rows reach
// the answer as canal wrote them: an integer past 2^53 keeps every digit (were
// it a number, not the string canal is asked for); a cell's object is
// untouched; invalid UTF-8 becomes U+FFFD, as anywhere else.
func TestStream(t *testing.T) {
	canal, _ := streaming(t,
		header,
		`[9007199254740993,"12345.67",{"b":1,"a":2},null]`,
		"[1,\"bad \xff byte\",{},true]",
		trailer,
	)
	var got []string
	err := canal.Stream(context.Background(), statement, Handler{
		Header: func(h Header) error {
			got = append(got, "header "+strings.Join(h.Fields, ",")+" "+strings.Join(h.Types, ","))
			return nil
		},
		Row: func(Row) error {
			got = append(got, "row")
			return nil
		},
	})
	if want := "header id,amount,attrs,flag int8,numeric,jsonb,bool|row|row"; err != nil || strings.Join(got, "|") != want {
		t.Errorf("handled %q %v, want %q", strings.Join(got, "|"), err, want)
	}

	res, err := canal.Execute(context.Background(), statement)
	if err != nil {
		t.Fatal(err)
	}
	b, err := jsonkit.Marshal(res.Rows)
	want := `[[9007199254740993,"12345.67",{"b":1,"a":2},null],[1,"bad ` + "\ufffd" + ` byte",{},true]]`
	if err != nil || string(b) != want || strings.Join(res.Fields, ",") != "id,amount,attrs,flag" {
		t.Errorf("rows = %s %v (fields %v), want %s", b, err, res.Fields, want)
	}

	// The Arrow types pass on as canal wrote them, every key kept.
	types, err := jsonkit.Marshal(res.ArrowTypes)
	if want := header[strings.Index(header, `[{"type"`) : len(header)-1]; err != nil || string(types) != want {
		t.Errorf("Arrow types = %s %v, want %s", types, err, want)
	}
	if res.ArrowTypes[1].Name() != "decimal" {
		t.Errorf("name = %q, want decimal", res.ArrowTypes[1].Name())
	}
	var back []Row
	if err := jsonkit.Unmarshal(b, &back); err != nil || len(back) != 2 {
		t.Errorf("rows do not read back: %v %v", back, err)
	}
}

// A query's error is in the trailer, with or without a header before it.
func TestQueryError(t *testing.T) {
	failed := `{"__holistics_trailer__":true,"http_code":422,"error":{"message":"relation does not exist","scope":"User","type":"QueryError"}}`
	for name, lines := range map[string][]string{"before the header": {failed}, "after it": {header, failed}} {
		canal, _ := streaming(t, lines...)
		var e *Error
		if _, err := canal.Execute(context.Background(), statement); !errors.As(err, &e) || e.Message != "relation does not exist" {
			t.Errorf("%s: %v, want canal's error", name, err)
		}
	}
}

// A header without Arrow types is a header: its Arrow types are empty.
func TestHeaderWithoutArrowTypes(t *testing.T) {
	canal, _ := streaming(t, `{"__holistics_header__":true,"fields":["a"],"types":["int8"]}`, `["1"]`, trailer)
	res, err := canal.Execute(context.Background(), statement)
	if err != nil || len(res.ArrowTypes) != 0 || len(res.Rows) != 1 {
		t.Errorf("read %+v %v", res, err)
	}
}

// A stream that is not what was asked for is refused, never passed on: a row
// that is not a JSON array, a row or the trailer before the header, a second
// header, Arrow types that are not one per field, anything after the trailer,
// or no trailer at all.
func TestMalformedStreamIsRefused(t *testing.T) {
	for name, lines := range map[string][]string{
		"a row that is not an array":  {header, `[1,2`, trailer},
		"a string for a row":          {header, `"x"`, trailer},
		"a number for a row":          {header, `7`, trailer},
		"a row before the header":     {`[1]`, header, trailer},
		"no header":                   {trailer},
		"a second header":             {header, header, trailer},
		"an Arrow type short":         {`{"__holistics_header__":true,"fields":["a","b"],"types":["x","y"],"arrow_types":[{"type":"int"}]}`, trailer},
		"an Arrow type not an object": {`{"__holistics_header__":true,"fields":["a"],"types":["x"],"arrow_types":["int64"]}`, trailer},
		"a row after the trailer":     {header, trailer, `[1]`},
		"no trailer":                  {header, `[1]`},
	} {
		canal, _ := streaming(t, lines...)
		if _, err := canal.Execute(context.Background(), statement); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A canal-query that does not answer is an outage, classified.
func TestUnreachable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + l.Addr().String()
	l.Close() // nothing listens there now

	canal := NewClient(url, false).ForTenant("org_1")
	if _, err := canal.Execute(context.Background(), Statement{SQL: "select 1", DBType: "postgres"}); !errors.Is(err, errcode.SidecarUnavailable) {
		t.Errorf("canal-query down: %v, want sidecar_unavailable", err)
	}
}

// With Config.CanalQueryURL set the supervisor dials an existing canal-query: no
// binary is resolved, and Close leaves the process alone.
func TestExternalDoesNotSpawn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	canal := New(sidecar.Config{CanalQueryURL: srv.URL})
	if err := canal.Start(context.Background()); err != nil {
		t.Fatalf("Start against an external canal-query: %v", err)
	}
	if canal.proc != nil {
		t.Error("a process was spawned for an external canal-query")
	}
	if canal.Client() == nil {
		t.Fatal("Client() is nil")
	}
	canal.Close()
}

// Progress reaches the handler before the header, read as canal-query wrote it;
// a handler without Progress ignores it.
func TestProgress(t *testing.T) {
	step := `{"__holistics_progress__":true,"kind":"step","at_ms":3,"step":"executing_query.poll_query"}`
	id := `{"__holistics_progress__":true,"kind":"dw_query_id","at_ms":5,"dw_query_id":"job_1"}`
	scanned := `{"__holistics_progress__":true,"kind":"dw_bytes_processed","at_ms":9,"dw_bytes_processed":2048}`
	canal, _ := streaming(t, step, id, scanned, header, `["1","2","{}",true]`, trailer)
	var got []string
	err := canal.Stream(context.Background(), statement, Handler{
		Progress: func(p Progress) error {
			got = append(got, fmt.Sprintf("%s %d %s%s", p.Kind, p.AtMs, p.Step, p.DwQueryID))
			if p.DwBytesProcessed != nil {
				got = append(got, fmt.Sprint(*p.DwBytesProcessed))
			}
			return nil
		},
		Header: func(Header) error { got = append(got, "header"); return nil },
		Row:    func(Row) error { got = append(got, "row"); return nil },
	})
	if want := "step 3 executing_query.poll_query|dw_query_id 5 job_1|dw_bytes_processed 9 |2048|header|row"; err != nil || strings.Join(got, "|") != want {
		t.Errorf("handled %q, %v; want %q", strings.Join(got, "|"), err, want)
	}
	if _, err := canal.Execute(context.Background(), statement); err != nil {
		t.Errorf("without a Progress handler: %v", err)
	}
}
