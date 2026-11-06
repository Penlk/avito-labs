package middleware

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Penlk/avito-labs/internal/http/requeststate"
)

func Tracing(provider trace.TracerProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := requeststate.From(r.Context())
			state.TraceContext = r.Context()
			span := trace.SpanFromContext(r.Context())
			span.SetAttributes(attribute.String("http.route", state.Route))
			completed := false
			defer func() {
				if !completed {
					state.Status, state.ErrorType = 500, "panic"
				}
				if state.Status >= 400 && state.ErrorType == "" {
					state.ErrorType = strconv.Itoa(state.Status)
				}
				span.SetAttributes(attribute.Int("http.response.status_code", state.Status))
				if state.ErrorType != "" {
					span.SetStatus(codes.Error, state.ErrorType)
					span.SetAttributes(attribute.String("error.type", state.ErrorType))
					if state.Err != nil {
						span.RecordError(state.Err)
					}
				}
			}()
			next.ServeHTTP(w, r)
			completed = true
		})
		return otelhttp.NewHandler(handler, "HTTP",
			otelhttp.WithTracerProvider(provider),
			otelhttp.WithMeterProvider(noop.NewMeterProvider()),
			otelhttp.WithPropagators(propagation.TraceContext{}),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + requeststate.From(r.Context()).Route
			}))
	}
}
