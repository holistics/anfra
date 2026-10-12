package canalquery

import (
	"context"
	"errors"
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
	return NewClient(srv.URL, false), &sent
}

var statement = Statement{SQL: "select 1", DBType: "postgres", TruncateRows: -1}

// anfra asks for a stream with a header first, with the fields' Arrow types,
// and integers as strings, as decimals are, so a JavaScript client keeps every
// digit.
func TestRequest(t *testing.T) {
	canal, sent := streaming(t, header, trailer)
	if _, err := canal.Execute(context.Background(), statement); err != nil {
		t.Fatal(err)
	}
	stream, _ := (*sent)["stream_result_options"].(map[string]any)
	output, _ := (*sent)["json_output_data_options"].(map[string]any)
	if (*sent)["stream_result"] != true || stream["header"] != true || stream["header_arrow_types"] != true ||
		output["integers_as_strings"] != true {
		t.Errorf("request = %v", *sent)
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

	canal := NewClient(url, false)
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
