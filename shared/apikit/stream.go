package apikit

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/holistics/anfra/shared/jsonkit"

	"github.com/holistics/anfra/shared/httpkit"
)

// NDJSON is the media type of a streamed response: one JSON event per line.
const NDJSON = "application/x-ndjson"

// pingEvery is how long a stream may go without an event before a ping: below
// the idle timeouts of common proxies and load balancers (60s for AWS's).
var pingEvery = 15 * time.Second

// writeEvery bounds the write of one event: a client that stops reading fails
// its stream rather than holding the run.
const writeEvery = 30 * time.Second

// wantsStream reports whether the request asks for a stream: Accept names NDJSON.
func wantsStream(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept") {
		for part := range strings.SplitSeq(v, ",") {
			if mt, _, err := mime.ParseMediaType(strings.TrimSpace(part)); err == nil && mt == NDJSON {
				return true
			}
		}
	}
	return false
}

// stream serves an op that returns a Run as NDJSON: its events, each flushed as
// it is emitted, then end. The status is sent with the first event, start, so an
// error before it is an ordinary error response; after it, end says it failed.
func (h HTTP[Call]) stream(w http.ResponseWriter, r *http.Request, rt *Runtime, req Call, raw []byte, o Op[Call]) {
	ew := &eventWriter{w: w, rc: http.NewResponseController(w), last: time.Now()}
	stop := ew.pinging()
	err := o.InvokeStream(r.Context(), rt, req, raw, ew.write)
	stop()
	switch {
	case errors.Is(r.Context().Err(), context.Canceled):
		// The client is gone: nobody reads the rest. The log line still says so.
		httpkit.Canceled(r.Context())
	case err == nil:
		_ = ew.write(StreamEnd{Type: "end", Status: "succeeded"})
	case !ew.begun():
		httpkit.WriteError(w, r, err)
	default:
		body := httpkit.RenderError(r, err)
		_ = ew.write(StreamEnd{Type: "end", Status: "failed", Error: &body})
	}
}

// eventWriter writes a stream's events, one per line, from the run and from its
// pings alike.
type eventWriter struct {
	mu      sync.Mutex
	w       http.ResponseWriter
	rc      *http.ResponseController
	started bool
	last    time.Time
}

func (e *eventWriter) begun() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.started
}

func (e *eventWriter) write(v any) error {
	b, err := jsonkit.Marshal(v)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.started {
		e.started = true
		h := e.w.Header()
		h.Set("Content-Type", NDJSON)
		h.Set("Cache-Control", "no-store")
		h.Set("X-Accel-Buffering", "no") // a buffering proxy would hold events back
		e.w.WriteHeader(http.StatusOK)
	}
	// Not every writer has deadlines (a test recorder has none); without one, the
	// write is bounded only by the connection.
	_ = e.rc.SetWriteDeadline(time.Now().Add(writeEvery))
	if _, err := e.w.Write(append(b, '\n')); err != nil {
		return err
	}
	e.last = time.Now()
	return e.rc.Flush()
}

// pinging sends a ping whenever the stream has begun and been idle for
// pingEvery, until stop is called; stop returns once no ping can be written.
func (e *eventWriter) pinging() (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		t := time.NewTicker(pingEvery / 3)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				e.mu.Lock()
				idle := e.started && time.Since(e.last) >= pingEvery
				e.mu.Unlock()
				if idle {
					_ = e.write(StreamPing{Type: "ping"})
				}
			}
		}
	})
	return func() {
		close(done)
		wg.Wait()
	}
}
