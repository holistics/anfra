package httpkit

import (
	"context"
	"io"
	"net/http"
	"strconv"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"

	"github.com/holistics/anfra/shared/requestid"
)

type errorHeadersKey struct{}

// WithErrorHeaders calls headers with the original error before WriteError
// commits a response within h. The callback may only set response headers; it
// must not write a status or body. Errors rendered inside a committed stream
// do not call it. A nil callback leaves h unchanged.
func WithErrorHeaders(h http.Handler, headers func(http.ResponseWriter, *http.Request, error)) http.Handler {
	if headers == nil {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), errorHeadersKey{}, headers)
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// WriteError is the one place an HTTP error is rendered: the public code's
// status, the envelope, and Retry-After when the details know it. It records the
// error for the request's log line.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if headers, ok := r.Context().Value(errorHeadersKey{}).(func(http.ResponseWriter, *http.Request, error)); ok {
		headers(w, r, err)
	}
	setError(r.Context(), err)
	resp := codesOf(r.Context()).Render(err, requestid.From(r.Context()))
	if ra, ok := resp.Details.(RetryAfter); ok && ra.RetryAfterSeconds() > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(ra.RetryAfterSeconds()))
	}
	WriteJSON(w, resp.Status, apperr.Envelope{Error: resp})
}

// RenderError is an error as WriteError would answer it, for a response that has
// already begun — a stream, whose status is sent — to carry in its body. It
// records the error for the request's log line, as WriteError does.
func RenderError(r *http.Request, err error) apperr.Response {
	setError(r.Context(), err)
	return codesOf(r.Context()).Render(err, requestid.From(r.Context()))
}

// RetryAfter is implemented by error details that know how long a client should
// wait before trying again; WriteError sends it as the Retry-After header.
type RetryAfter interface{ RetryAfterSeconds() int }

// WriteJSON writes v as a JSON response, never cached: every response is per
// caller.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // API responses are per caller, never cached
	w.WriteHeader(status)
	_ = jsonkit.MarshalWrite(w, v)
	_, _ = io.WriteString(w, "\n")
}
