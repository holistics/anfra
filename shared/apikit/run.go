package apikit

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/holistics/anfra/shared/apperr"
)

// Run is the output of an op that answers as it works: what its handler returns
// instead of a finished value, when the work is long or its result large. It
// does its work when a transport reads it, in the form that transport takes:
//
//   - Page: the work, then one answer — what a JSON client, MCP and the CLI get.
//     It is the op's output as discovery and OpenAPI describe it.
//   - Stream: the work, emitting its events as they happen — progress, then the
//     result in batches — for a client that follows it (HTTP, Accept:
//     application/x-ndjson).
//
// The op's timeout covers the handler and the reading alike, and the op's step
// spans both: the work runs inside the op, only later than its handler.
//
// Neither is called more than once, nor both. A handler builds a Run from
// closures over its input; the handler itself checks the input and returns
// without doing the work, so that what it refuses is an ordinary error, before
// the stream starts.
//
// # A stream's events
//
// Every event is a JSON object whose "type" names it. A stream is:
//
//   - start: first, as soon as the handler has returned the Run — the op has
//     accepted the request. Its status is sent with it, so what fails after it
//     is reported in the stream (end), not as an error response;
//   - status: the run's phase, whenever the op reports one (Events.Status);
//   - the op's own events (Events.Emit), of its Event type;
//   - end: always last, once — succeeded, or failed with the error as an error
//     response's body would carry it. A stream without one was cut;
//   - ping: while nothing else is sent, so that proxies do not close it as idle.
//
// apikit owns start, status, end and ping; an op's Event type names its own
// types, none of them those (Register checks).
type Run[Page, Event any] struct {
	Page   func(ctx context.Context) (Page, error)
	Stream func(ctx context.Context, events Events[Event]) error
}

// Events is how a Run's Stream emits: its own events, and its phase.
type Events[Event any] struct {
	emit func(any) error
}

// NewEvents is Events sending to emit: for reading a Run's stream outside a
// transport, as a test does.
func NewEvents[Event any](emit func(any) error) Events[Event] { return Events[Event]{emit: emit} }

// Emit sends one of the op's own events.
func (e Events[Event]) Emit(ev Event) error { return e.emit(ev) }

// Status reports the run's phase ("compiling_query", "executing_query"): what
// a client shows while it waits.
func (e Events[Event]) Status(phase string) error {
	return e.emit(StreamStatus{Type: "status", Phase: phase})
}

// StreamStart is a stream's first event: the op has accepted the request.
type StreamStart struct {
	Type string `json:"type" enum:"start"`
}

// StreamStatus is the run's phase, as the op reports it.
type StreamStatus struct {
	Type  string `json:"type" enum:"status"`
	Phase string `json:"phase"`
}

// StreamEnd is a stream's last event: how the run ended. Error is set when it
// failed, as an error response's body would carry it.
type StreamEnd struct {
	Type   string           `json:"type" enum:"end"`
	Status string           `json:"status" enum:"succeeded,failed"`
	Error  *apperr.Response `json:"error,omitempty"`
}

// StreamPing is sent while a stream has nothing else to send, so that proxies
// do not close it as idle. It carries nothing.
type StreamPing struct {
	Type string `json:"type" enum:"ping"`
}

// reservedEvents are apikit's events, by their type: no op's event may use one.
var reservedEvents = map[string]reflect.Type{
	"start":  reflect.TypeFor[StreamStart](),
	"status": reflect.TypeFor[StreamStatus](),
	"end":    reflect.TypeFor[StreamEnd](),
	"ping":   reflect.TypeFor[StreamPing](),
}

// run is a Run of any types, as the framework handles it.
type run interface {
	pageType() reflect.Type
	eventType() reflect.Type
	page(ctx context.Context) (any, error)
	stream(ctx context.Context, emit func(any) error) error
}

var runType = reflect.TypeFor[run]()

func (r Run[Page, Event]) pageType() reflect.Type  { return reflect.TypeFor[Page]() }
func (r Run[Page, Event]) eventType() reflect.Type { return reflect.TypeFor[Event]() }

func (r Run[Page, Event]) page(ctx context.Context) (any, error) { return r.Page(ctx) }

func (r Run[Page, Event]) stream(ctx context.Context, emit func(any) error) error {
	return r.Stream(ctx, Events[Event]{emit: emit})
}

// OutputType is what an op whose handler returns out answers: a Run's page, or
// out itself.
func OutputType(out reflect.Type) reflect.Type {
	if page, _, ok := runTypes(out); ok {
		return page
	}
	return out
}

// runTypes are the page and event types of Out when it is a Run.
func runTypes(out reflect.Type) (page, event reflect.Type, ok bool) {
	if !out.Implements(runType) {
		return nil, nil, false
	}
	r := reflect.Zero(out).Interface().(run)
	return r.pageType(), r.eventType(), true
}

// eventTypesOf are the types an op's events may be: event, when it is a struct;
// each of declared, when it is an interface they all implement. Each must name
// its types (opEventNames), and no two the same.
func eventTypesOf(op string, event reflect.Type, declared []any) []reflect.Type {
	types := []reflect.Type{event}
	switch {
	case event.Kind() == reflect.Interface && len(declared) == 0:
		panic(fmt.Sprintf("op %q: its events are an interface; declare each type in Events", op))
	case event.Kind() == reflect.Interface:
		types = nil
		for _, v := range declared {
			t := reflect.TypeOf(v)
			if t == nil || !t.Implements(event) {
				panic(fmt.Sprintf("op %q: event %T does not implement %v", op, v, event))
			}
			types = append(types, t)
		}
	case len(declared) > 0:
		panic(fmt.Sprintf("op %q: declares Events, but its events are one type, %v", op, event))
	}
	seen := map[string]bool{}
	for _, t := range types {
		for _, n := range opEventNames(op, t) {
			if seen[n] {
				panic(fmt.Sprintf("op %q: two of its events are named %q", op, n))
			}
			seen[n] = true
		}
	}
	return types
}

// opEventNames are the types an op's Event names: the values of its "type" field's
// enum. A struct without one, or naming one of apikit's own, is refused.
func opEventNames(op string, t reflect.Type) []string {
	if t.Kind() == reflect.Struct {
		for f := range t.Fields() {
			if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name != "type" || f.Type.Kind() != reflect.String {
				continue
			}
			names := strings.Split(f.Tag.Get("enum"), ",")
			if slices.Contains(names, "") {
				break
			}
			for _, n := range names {
				if _, reserved := reservedEvents[n]; reserved {
					panic(fmt.Sprintf("op %q: event type %q is reserved by apikit", op, n))
				}
			}
			return names
		}
	}
	panic(fmt.Sprintf("op %q: an event is a struct with a string field \"type\", its values listed in its enum tag", op))
}
