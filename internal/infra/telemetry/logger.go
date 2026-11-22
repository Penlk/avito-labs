package telemetry

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type traceHandler struct{ slog.Handler }

func NewLogger(out io.Writer, level, service string) (*slog.Logger, error) {
	var severity slog.Level
	if err := severity.UnmarshalText([]byte(level)); err != nil {
		return nil, err
	}
	return newLogger(out, severity, service), nil
}

// Config parsing can fail before the application logger exists.
func LogError(ctx context.Context, out io.Writer, service string, err error) {
	if service == "" {
		service = "trip-service"
	}
	newLogger(out, slog.LevelError, service).ErrorContext(ctx, "service failed", "error", err)
}

func newLogger(out io.Writer, level slog.Level, service string) *slog.Logger {
	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level})
	return slog.New(traceHandler{handler}).With("service", service)
}

func (h traceHandler) Handle(ctx context.Context, record slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	traceID, spanID := "", ""
	if sc.IsValid() {
		traceID, spanID = sc.TraceID().String(), sc.SpanID().String()
	}
	record.AddAttrs(slog.String("trace_id", traceID), slog.String("span_id", spanID))
	return h.Handler.Handle(ctx, record)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}
