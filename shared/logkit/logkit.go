// Package logkit is what a log record names: its event, defined once, and the
// trace it was written in.
//
// A record's msg is a sentence for people; its event is the name programs match
// on (alert rules, dashboards, searches), a soft contract kept compatible as
// best anfra can (docs/designs/logs.md). An event is defined at package level
// with DefineEvent, never invented at a call site, so Events lists every event
// a binary can write:
//
//	var eventFailed = logkit.DefineEvent("routine.failed", "A routine's run returned an error.",
//		"routine", "outcome", "error")
//
//	logger.ErrorContext(ctx, "Routine failed", logkit.EventKey, eventFailed, "routine", name, ...)
//
// TraceHandler adds the trace and span a record was written in, so one
// request's records join across services.
package logkit

import (
	"fmt"
	"log/slog"
	"regexp"
	"slices"
)

// EventKey is the attribute that carries a record's event.
const EventKey = "event"

// Event is something that happened, as programs reading the log know it. As an
// attribute's value, it is its name.
type Event struct{ e *event }

type event struct {
	name, doc string
	attrs     []string
}

var (
	// eventName is <area>.<what happened>, each part snake_case, as many
	// parts as the area needs.
	eventName = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	attrName  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	// defined is every event, in definition order.
	defined []Event
	names   = map[string]bool{}
)

// DefineEvent defines an event: its name, what it means, and the attributes
// its records may carry. Call it from a package-level var, once per name; a
// name already defined or not <area>.<what happened> in snake_case, no doc, or
// an attribute not snake_case or listed twice panics at startup.
func DefineEvent(name, doc string, attrs ...string) Event {
	if !eventName.MatchString(name) {
		panic(fmt.Sprintf("logkit: event %q is not <area>.<what happened> in snake_case", name))
	}
	if names[name] {
		panic("logkit: duplicate event " + name)
	}
	if doc == "" {
		panic("logkit: event " + name + " has no doc")
	}
	for i, a := range attrs {
		if !attrName.MatchString(a) || a == EventKey {
			panic(fmt.Sprintf("logkit: event %s: attribute %q is not a snake_case name of its own", name, a))
		}
		if slices.Contains(attrs[:i], a) {
			panic(fmt.Sprintf("logkit: event %s lists attribute %q twice", name, a))
		}
	}
	names[name] = true
	e := Event{&event{name: name, doc: doc, attrs: slices.Clone(attrs)}}
	defined = append(defined, e)
	return e
}

// Name is the event's name, as records carry it.
func (e Event) Name() string { return e.e.name }

// Doc says what the event means.
func (e Event) Doc() string { return e.e.doc }

// Attrs are the attributes its records may carry, besides those every record
// has.
func (e Event) Attrs() []string { return slices.Clone(e.e.attrs) }

// String is the event's name.
func (e Event) String() string { return e.e.name }

// LogValue makes the event's name its value in a record.
func (e Event) LogValue() slog.Value { return slog.StringValue(e.e.name) }

// Events is every event defined in this binary, in definition order.
func Events() []Event { return slices.Clone(defined) }
