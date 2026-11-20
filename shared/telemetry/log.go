package telemetry

import (
	"context"
	"log/slog"
	"maps"
	"slices"
)

// LogHandler wraps h so that a record logged with a context that names its
// caller (callerFields) carries its fields, each under its LogKey:
// gen_ai.conversation.id as gen_ai_conversation_id.
func LogHandler(h slog.Handler) slog.Handler { return logHandler{h} }

type logHandler struct{ slog.Handler }

func (h logHandler) Handle(ctx context.Context, r slog.Record) error {
	if fields := callerFields(ctx); len(fields) > 0 {
		r = r.Clone()
		for _, k := range slices.Sorted(maps.Keys(fields)) {
			r.AddAttrs(slog.String(LogKey(k), fields[k]))
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return logHandler{h.Handler.WithAttrs(attrs)}
}

func (h logHandler) WithGroup(name string) slog.Handler {
	return logHandler{h.Handler.WithGroup(name)}
}
