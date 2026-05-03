package logger

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// traceHandler decorates a slog.Handler so every record gains trace_id and
// span_id attributes when the log call is made with a context that carries an
// active span. Use slog.Logger.*Context methods (e.g. InfoContext) so the ctx
// reaches Handle.
type traceHandler struct {
	slog.Handler
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs and WithGroup must re-wrap the inner handler so chained loggers
// (log.With(...)) keep the trace-correlation behavior.
func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithGroup(name)}
}

func NewLogger() *slog.Logger {
	return slog.New(&traceHandler{Handler: slog.NewJSONHandler(os.Stdout, nil)})
}
