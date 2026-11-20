package middleware

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const unmatchedRoute = "/unmatched"

func Tracing(tracer trace.Tracer) func(http.Handler) http.Handler {
	propagator := propagation.TraceContext{}
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routes := chi.RouteContext(r.Context()).Routes
			matched := chi.NewRouteContext()
			bufferWriter := NewBufferWriter()

			route := unmatchedRoute
			if routes.Match(matched, r.Method, r.URL.Path) {
				route = matched.RoutePattern()
			}

			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := tracer.Start(
				ctx,
				r.Method+" "+route,
				trace.WithSpanKind(trace.SpanKindServer),
			)
			defer span.End()
			defer func() {
				rec := recover()
				status := bufferWriter.status
				errorType := ""
				if rec != nil {
					status = http.StatusInternalServerError
					errorType = "panic"
				} else if status >= http.StatusBadRequest {
					errorType = strconv.Itoa(status)
				}

				span.SetAttributes(attribute.Int("http.response.status_code", status))
				if errorType != "" {
					span.SetStatus(codes.Error, http.StatusText(status))
					span.SetAttributes(attribute.String("error.type", errorType))
				}
				if rec != nil {
					panic(rec)
				}
				if err := bufferWriter.WriteToOriginal(w); err != nil {
					span.SetStatus(codes.Error, "write response failed")
					span.SetAttributes(attribute.String("error.type", "response_write"))
					span.RecordError(err)
				}
			}()

			handler.ServeHTTP(bufferWriter, r.WithContext(ctx))
		})
	}
}
