package httpkit_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/shared/httpkit"
)

// A server's own catalog, as a server defines one.
var (
	testNS    = apperr.DefineNamespace("test_httpkit")
	busy      = apperr.DefinePublicCodeWith[wait](testNS, "busy", apperr.Server, "Busy.")
	unserved  = apperr.DefinePublicCode(apperr.DefineNamespace("test_httpkit_lib"), "lib_failed", apperr.User, "The library failed.")
	testCodes = httpkit.Codes{Namespaces: []apperr.Namespace{testNS}, Status: map[apperr.Code]int{busy.Code(): http.StatusServiceUnavailable}}
)

// wait is busy's details: it knows how long to wait.
type wait struct {
	Seconds int `json:"seconds"`
}

func (w wait) RetryAfterSeconds() int { return w.Seconds }

// spans captures the spans the tests end; installed once, globally, as otelhttp
// uses the global provider.
var spans = func() *tracetest.SpanRecorder {
	r := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(r)))
	return r
}()

// serve runs one request through Wrap(h) and returns the response and its log
// line.
func serve(t *testing.T, h http.HandlerFunc) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var logs bytes.Buffer
	rec := httptest.NewRecorder()
	httpkit.Wrap(h, httpkit.Config{Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Codes: testCodes}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/rpc/things.get", nil))
	var line map[string]any
	if err := jsonkit.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("log line: %v: %s", err, logs.String())
	}
	return rec, line
}

func errorBody(t *testing.T, rec *httptest.ResponseRecorder) apperr.Response {
	t.Helper()
	var env apperr.Envelope
	if err := jsonkit.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body: %v: %s", err, rec.Body.String())
	}
	return env.Error
}

// A routed request is named on its log line and its span, and carries its
// request id in the header, the log line and the span.
func TestRouted(t *testing.T) {
	spans.Reset()
	rec, line := serve(t, func(w http.ResponseWriter, r *http.Request) {
		httpkit.Routed(r.Context(), "things.get", "/rpc/things.get")
		httpkit.WriteJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	})
	id := rec.Header().Get("Request-Id")
	if rec.Code != 200 || id == "" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("got %d, headers %v", rec.Code, rec.Header())
	}
	if line["op"] != "things.get" || line["request_id"] != id || line["status"] != float64(200) || line["level"] != "INFO" {
		t.Errorf("log line = %v", line)
	}
	ended := spans.Ended()
	if len(ended) != 1 || ended[0].Name() != "POST /rpc/things.get" {
		t.Fatalf("spans = %v", ended)
	}
	var hasID bool
	for _, kv := range ended[0].Attributes() {
		hasID = hasID || (kv.Key == "request.id" && kv.Value.AsString() == id)
	}
	if !hasID {
		t.Errorf("the root span does not carry request.id %s", id)
	}
}

// An error is rendered with its served code's status, Retry-After from its
// details, and logged at the level its scope sets.
func TestWriteError(t *testing.T) {
	rec, line := serve(t, func(w http.ResponseWriter, r *http.Request) {
		httpkit.WriteError(w, r, apperr.NewWith(busy, "", wait{Seconds: 7}))
	})
	if body := errorBody(t, rec); rec.Code != 503 || body.Code != "busy" || body.RequestID != rec.Header().Get("Request-Id") {
		t.Errorf("got %d %+v", rec.Code, body)
	}
	if rec.Header().Get("Retry-After") != "7" {
		t.Errorf("Retry-After = %q", rec.Header().Get("Retry-After"))
	}
	if line["code"] != "busy" || line["scope"] != "server" || line["level"] != "ERROR" {
		t.Errorf("log line = %v", line)
	}
}

// A panic is internal_server_error to the client, and the stack goes to the log
// line only.
func TestRecover(t *testing.T) {
	rec, line := serve(t, func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	body := errorBody(t, rec)
	if rec.Code != 500 || body.Code != "internal_server_error" || bytes.Contains(rec.Body.Bytes(), []byte("kaboom")) {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
	if msg, _ := line["error"].(string); line["level"] != "ERROR" || !bytes.Contains([]byte(msg), []byte("kaboom")) {
		t.Errorf("log line = %v", line)
	}
}

// A client that went away gets nothing, and an info line saying so.
func TestCanceled(t *testing.T) {
	rec, line := serve(t, func(_ http.ResponseWriter, r *http.Request) { httpkit.Canceled(r.Context()) })
	if rec.Body.Len() != 0 {
		t.Errorf("a gone client was written %q", rec.Body.String())
	}
	if line["canceled"] != true || line["level"] != "INFO" {
		t.Errorf("log line = %v", line)
	}
}

// Every generic code has a status of its own; a server's table holds only its
// own codes.
func TestEveryGenericCodeHasAStatus(t *testing.T) {
	for _, c := range apperr.Codes(apperr.Generic) {
		if s := (httpkit.Codes{}).StatusOf(c); s == http.StatusInternalServerError && c != apperr.InternalServerError {
			t.Errorf("%s has no status", c.Qualified())
		}
	}
}

// A code outside the server's namespaces fails closed: the client gets
// internal_server_error, never the library's code or message.
func TestUnservedCodeFailsClosed(t *testing.T) {
	got := testCodes.Render(unserved, "req_1")
	if got.Code != "internal_server_error" || got.Status != 500 || got.Message == "The library failed." {
		t.Errorf("client receives %+v", got)
	}
	if testCodes.Served(busy.Code()) != busy.Code() || testCodes.StatusOf(busy.Code()) != 503 {
		t.Error("the server's own code is not served with its status")
	}
}
