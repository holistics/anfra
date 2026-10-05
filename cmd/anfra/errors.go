package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/holistics/anfra/shared/apperr"
)

// remoteError is a warm server's error, as its /call body carried it.
type remoteError struct{ resp apperr.Response }

func (e *remoteError) Error() string { return e.resp.Code + ": " + e.resp.Message }

// printError shows a failed command: what it was doing and why it failed, then
// the error's details — an invalid query's diagnostics, the args that were
// wrong — as YAML, then its cause when it has one. Encapsulation is disabled
// in this binary (see main), so the message is the real one.
func printError(w io.Writer, err error) {
	var resp apperr.Response
	var cause error
	if re, ok := errors.AsType[*remoteError](err); ok {
		resp = re.resp
	} else {
		e := apperr.From(err)
		resp = e.Response("")
		cause = errors.Unwrap(e)
	}
	parts := make([]string, 0, len(resp.Context)+1)
	for _, c := range resp.Context {
		parts = append(parts, c.Text)
	}
	parts = append(parts, resp.Message)
	fmt.Fprintln(w, "Error:", strings.Join(parts, ": "))
	if resp.Details != nil {
		// Through JSON, so the details read as their wire form does.
		if b, jerr := json.Marshal(resp.Details); jerr == nil {
			_ = renderTo(b, "application/json", w)
		}
	}
	if cause != nil && !strings.Contains(resp.Message, cause.Error()) {
		fmt.Fprintln(w, "Cause:", cause)
	}
}
