package httpkit

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/holistics/anfra/shared/apperr"

	"github.com/holistics/anfra/shared/requestid"
)

// WriteError is the one place an HTTP error is rendered: the public code's
// status, the envelope, and Retry-After when the details know it. It records the
// error for the request's log line.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	setError(r.Context(), err)
	resp := codesOf(r.Context()).Render(err, requestid.From(r.Context()))
	if ra, ok := resp.Details.(RetryAfter); ok && ra.RetryAfterSeconds() > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(ra.RetryAfterSeconds()))
	}
	WriteJSON(w, resp.Status, apperr.Envelope{Error: resp})
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
	_ = json.NewEncoder(w).Encode(v)
}
