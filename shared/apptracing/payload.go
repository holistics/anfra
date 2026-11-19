package apptracing

import (
	"context"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/holistics/anfra/shared/telemetry"
)

// Payloads are a command's arguments, an op's whole input and answer, and a
// query's SQL: each recorded as its category says (shared/telemetry), because
// they can hold the user's data (docs/designs/telemetry.md). Each is cut at
// the payload limit, and marked when it is.

// cutMark ends a payload that was cut, so that it reads as partial.
const cutMark = "…"

// Payloader is an answer that says what of it a trace may record, when that is
// less than all of it: a query's answer is recorded without its rows. sql is
// whether the SQL in it may be recorded: only when SQL is raw.
type Payloader interface {
	TracePayload(sql bool) any
}

// RecordPayload records v as the span event on ctx's span, as JSON, cut at the
// limit, when the event's category is recorded: []byte as it is (an op's
// input, as sent), a Payloader as its TracePayload, anything else encoded.
func RecordPayload(ctx context.Context, event telemetry.Key, v any) {
	settings := telemetry.SettingsOf(ctx)
	if c := event.Category(); c != "" && settings.Level(c) == telemetry.Off {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	var text string
	switch v := v.(type) {
	case []byte:
		text = string(v)
	default:
		if p, ok := v.(Payloader); ok {
			v = p.TracePayload(settings.Level(telemetry.SQL) == telemetry.Raw)
		}
		b, err := jsonkit.Marshal(v)
		if err != nil {
			return
		}
		text = string(b)
	}
	attrs := []attribute.KeyValue{attribute.Int(telemetry.PayloadSize.String(), len(text))}
	text, wasCut := CutPayload(ctx, text)
	if wasCut {
		attrs = append(attrs, attribute.Bool(telemetry.PayloadTruncated.String(), true))
	}
	span.AddEvent(event.String(), trace.WithAttributes(append(attrs, attribute.String(telemetry.Payload.String(), text))...))
}

// CutPayload is text cut at ctx's payload limit, marked when it was cut, never
// in the middle of a character.
func CutPayload(ctx context.Context, text string) (_ string, wasCut bool) {
	limit := telemetry.SettingsOf(ctx).PayloadLimit()
	if len(text) <= limit {
		return text, false
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit] + cutMark, true
}
