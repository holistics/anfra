package httpkit

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/holistics/anfra/shared/apperr"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/shared/requestid"
)

// assignRequestID mints the request's id, returns it in the Request-Id header —
// so a failure without a body still carries it — and puts it in the context for
// the renderer and the log line.
func assignRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := requestid.New()
		w.Header().Set("Request-Id", id)
		next.ServeHTTP(w, r.WithContext(requestid.With(r.Context(), id)))
	})
}

// traceRequests starts the request's root span, carrying the request id as
// request.id, so a request id leads to its trace in one query
// (error_handling.md §The request id). The span is named by its method until
// the request is routed to an op (setRoute).
//
// Unless trusted, the endpoint is public: a traceparent from a browser or any
// other caller starts no trace of ours, since a caller could otherwise choose
// our trace ids and sampling. It is kept as a link.
func traceRequests(next http.Handler, trusted bool) http.Handler {
	tagged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("request.id", requestid.From(r.Context())))
		next.ServeHTTP(w, r)
	})
	traced := otelhttp.NewHandler(tagged, "",
		otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return !trusted }),
		// otelhttp names the span when it starts, and again when it ends if the
		// request has a mux pattern, which an enclosing mux sets ("/api/"). Both
		// go through here, so both use the route once there is one.
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			if rt, _ := r.Context().Value(routeKey{}).(*string); rt != nil && *rt != "" {
				return http.MethodPost + " " + *rt
			}
			return r.Method
		}),
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traced.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), routeKey{}, new(string))))
	})
}

// routeKey holds the route the request was served by, once it is known.
type routeKey struct{}

// Routed records the operation a request was routed to: op names it on the log
// line, and route (POST-only, so "/api/<op>" or "/rpc/<name>") names the root span
// and labels the request's metrics. An unrouted request keeps its method alone,
// so a stream of junk paths cannot create a span name or metric series each.
func Routed(ctx context.Context, op, route string) {
	setOp(ctx, op)
	if rt, _ := ctx.Value(routeKey{}).(*string); rt != nil {
		*rt = route
	}
	span := trace.SpanFromContext(ctx)
	span.SetName(http.MethodPost + " " + route)
	span.SetAttributes(semconv.HTTPRoute(route))
	if l, ok := otelhttp.LabelerFromContext(ctx); ok {
		l.Add(semconv.HTTPRoute(route))
	}
}

// entry is what the request's log line reports beyond the request itself,
// filled in as the request is served.
type entry struct {
	op       string
	err      error
	canceled bool
}

type entryKey struct{}

func entryOf(ctx context.Context) *entry {
	e, _ := ctx.Value(entryKey{}).(*entry)
	return e
}

func setOp(ctx context.Context, name string) {
	if e := entryOf(ctx); e != nil {
		e.op = name
	}
}

func setError(ctx context.Context, err error) {
	if e := entryOf(ctx); e != nil {
		e.err = err
	}
}

// Canceled records that the client went away before a response: the log line
// says so, at info, and nothing is written to it.
func Canceled(ctx context.Context) {
	if e := entryOf(ctx); e != nil {
		e.canceled = true
	}
}

// recorder notes the status a handler wrote.
type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// logRequests writes one log line per request: the request id, the op, the
// status, the duration, and for a failure the public code, the internal code if
// any, the scope, and the whole chain — err.Error(), with sensitive parameters
// redacted. The level follows the scope: user info, client warn, server error.
// A canceled request whose client is gone logs at info.
func logRequests(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		e := &entry{}
		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), entryKey{}, e)))

		attrs := []slog.Attr{
			slog.String("request_id", requestid.From(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Duration("duration", time.Since(start)),
		}
		if e.op != "" {
			attrs = append(attrs, slog.String("op", e.op))
		}
		level := slog.LevelInfo
		switch {
		case e.canceled:
			attrs = append(attrs, slog.Bool("canceled", true))
		case e.err != nil:
			fe := apperr.From(e.err)
			pub := codesOf(r.Context()).Served(fe.Code)
			attrs = append(attrs, slog.String("code", pub.String()), slog.String("scope", string(pub.Scope())))
			if fe.Code != pub {
				attrs = append(attrs, slog.String("internal_code", fe.Code.Qualified()))
			}
			attrs = append(attrs, slog.String("error", e.err.Error()))
			switch pub.Scope() {
			case apperr.Client:
				level = slog.LevelWarn
			case apperr.Server:
				level = slog.LevelError
			}
		}
		logger.LogAttrs(r.Context(), level, "request", attrs...)
	})
}

// recoverPanics turns a panic into internal_server_error. The panic and its stack
// go to the log line; the client gets only the generic message. A panic after the
// response has started cannot be rendered, and is only logged.
// http.ErrAbortHandler is the standard library's signal to abort silently, and is
// passed on.
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recorder{ResponseWriter: w}
		defer func() { //nolint:contextcheck // it renders through r, whose context it uses
			p := recover()
			if p == nil {
				return
			}
			if p == http.ErrAbortHandler { //nolint:errorlint // a sentinel compared by identity, as net/http does
				panic(p)
			}
			err := fmt.Errorf("panic: %v\n%s", p, debug.Stack())
			if rec.status != 0 {
				setError(r.Context(), err)
				return
			}
			WriteError(rec, r, err)
		}()
		next.ServeHTTP(rec, r)
	})
}
