package canalquery

import (
	"context"
	"errors"
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

// streaming answers each query with lines, as canal-query streams a result.
func streaming(t *testing.T, lines ...string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, false)
}

// Rows reach the answer as canal wrote them: an integer past 2^53 keeps every
// digit, where a float64 would round it; a cell's object is untouched; invalid
// UTF-8 becomes U+FFFD, as anywhere else.
func TestRowsAreKeptAsCanalWroteThem(t *testing.T) {
	canal := streaming(t,
		`[9007199254740993,"12345.67",{"b":1,"a":2},null]`,
		"[1,\"bad \xff byte\",{},true]",
		`{"__holistics_trailer__":true,"metadata":{"fields":["id","amount","attrs","flag"],"record_count":2},"http_code":200}`,
	)
	res, err := canal.Execute(context.Background(), "postgres", nil, "select 1", -1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Fields, ",") != "id,amount,attrs,flag" {
		t.Errorf("fields = %v", res.Fields)
	}
	b, err := jsonkit.Marshal(res.Rows)
	want := `[[9007199254740993,"12345.67",{"b":1,"a":2},null],[1,"bad ` + "\ufffd" + ` byte",{},true]]`
	if err != nil || string(b) != want {
		t.Errorf("rows = %s %v, want %s", b, err, want)
	}
	var back []Row
	if err := jsonkit.Unmarshal(b, &back); err != nil || len(back) != 2 {
		t.Errorf("rows do not read back: %v %v", back, err)
	}
}

// A row that is not a JSON array is canal's fault, refused rather than passed on.
func TestMalformedRowIsRefused(t *testing.T) {
	for _, line := range []string{`[1,2`, `"x"`, `7`} {
		if _, err := streaming(t, line).Execute(context.Background(), "postgres", nil, "select 1", -1); err == nil {
			t.Errorf("row %s was accepted", line)
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
	if _, err := canal.Execute(context.Background(), "postgres", nil, "select 1", 0); !errors.Is(err, errcode.SidecarUnavailable) {
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
