package telemetry

import (
	"context"
	"slices"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/shared/apperr"
)

// Filter is the span processor that applies the data settings to every span
// before next, the exporter's batcher, sees it, whoever started it: anfra's
// own spans and a library's. Each span follows the settings of the context it
// was started in (WithSettings), or fallback, the host's, when that has none.
//
// A declared attribute or event that is data is recorded only when data is. A
// key declared as left out never is. A key that isn't declared is left out too, and named in
// anfra.telemetry.dropped_keys: the registry test fails on it before it
// ships, and one it misses fails safe. A span's error status keeps its text
// only when that is a public error code or PanicStatus.
//
// The code that records a payload checks the settings first too, so a payload
// that is off is never encoded (apptracing.RecordPayload). Filter is what
// makes the settings hold for every attribute, however it was set.
func Filter(next sdktrace.SpanProcessor, fallback Settings) sdktrace.SpanProcessor {
	return &filter{next: next, fallback: fallback}
}

type filter struct {
	next     sdktrace.SpanProcessor
	fallback Settings
	started  sync.Map // trace.SpanID → Settings, from start to end
}

func (f *filter) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	settings, ok := SettingsFrom(parent)
	if !ok {
		settings = f.fallback
	}
	f.started.Store(s.SpanContext().SpanID(), settings)
	f.next.OnStart(parent, s)
}

func (f *filter) OnEnd(s sdktrace.ReadOnlySpan) {
	settings := f.fallback
	if v, ok := f.started.LoadAndDelete(s.SpanContext().SpanID()); ok {
		settings = v.(Settings)
	}
	f.next.OnEnd(Apply(s, settings))
}

func (f *filter) Shutdown(ctx context.Context) error   { return f.next.Shutdown(ctx) }
func (f *filter) ForceFlush(ctx context.Context) error { return f.next.ForceFlush(ctx) }

// Apply is s as settings let it be recorded.
func Apply(s sdktrace.ReadOnlySpan, settings Settings) sdktrace.ReadOnlySpan {
	var dropped []string
	out := filtered{ReadOnlySpan: s, attrs: applyAttrs(s.Attributes(), settings, &dropped), status: s.Status()}
	for _, e := range s.Events() {
		d, ok := classify(e.Name)
		if !ok {
			dropped = append(dropped, e.Name)
			continue
		}
		if !records(d, settings) {
			continue
		}
		e.Attributes = applyAttrs(e.Attributes, settings, &dropped)
		out.events = append(out.events, e)
	}
	if len(dropped) > 0 {
		slices.Sort(dropped)
		out.attrs = append(out.attrs, attribute.StringSlice(DroppedKeys.String(), slices.Compact(dropped)))
	}
	if out.status.Code == codes.Error && !holdsNoData(out.status.Description) && !settings.RecordsData() {
		out.status.Description = ""
	}
	return out
}

// records reports whether a declared key is recorded under settings.
func records(d keyDef, settings Settings) bool {
	return !d.leftOut && (!d.data || settings.RecordsData())
}

// holdsNoData reports whether a status text is one code chose: a public error
// code, or PanicStatus. Anything else is a message, which may hold data.
func holdsNoData(status string) bool {
	return status == PanicStatus || apperr.IsPublicName(status)
}

// applyAttrs is attrs as settings let them be recorded, adding the name of
// each undeclared key it leaves out to dropped.
func applyAttrs(attrs []attribute.KeyValue, settings Settings, dropped *[]string) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, a := range attrs {
		d, ok := classify(string(a.Key))
		if !ok {
			*dropped = append(*dropped, string(a.Key))
			continue
		}
		if records(d, settings) {
			out = append(out, a)
		}
	}
	return out
}

// filtered is a span with what the settings leave of its attributes, events
// and status.
type filtered struct {
	sdktrace.ReadOnlySpan
	attrs  []attribute.KeyValue
	events []sdktrace.Event
	status sdktrace.Status
}

func (s filtered) Attributes() []attribute.KeyValue { return s.attrs }
func (s filtered) Events() []sdktrace.Event         { return s.events }
func (s filtered) Status() sdktrace.Status          { return s.status }

// RecordError records err on span as an exception event when s records data,
// with its stack when stack is set. The status is the caller's to set.
func RecordError(span trace.Span, s Settings, err error, stack bool) {
	if err == nil || !span.IsRecording() || !s.RecordsData() {
		return
	}
	span.RecordError(err, trace.WithStackTrace(stack))
}
