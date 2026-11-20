package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func Metrics(durationMetric metric.Float64Histogram, activeMetric metric.Int64UpDownCounter) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bufferWriter := NewBufferWriter()
			oldTime := time.Now()
			var newTime time.Time
			method := metricMethod(r.Method)

			routes := chi.RouteContext(r.Context()).Routes
			matched := chi.NewRouteContext()

			route := unmatchedRoute
			if routes.Match(matched, r.Method, r.URL.Path) {
				route = matched.RoutePattern()
			}

			activeMetric.Add(r.Context(), 1, metric.WithAttributes(
				attribute.String("http.request.method", method),
				attribute.String("http.route", route),
			))

			handlerReturned := false

			defer func() {
				activeMetric.Add(r.Context(), -1, metric.WithAttributes(
					attribute.String("http.request.method", method),
					attribute.String("http.route", route),
				))
				var requestDuration time.Duration
				if handlerReturned {
					durationMetric.Record(r.Context(), newTime.Sub(oldTime).Seconds(), metric.WithAttributes(
						attribute.String("http.request.method", method),
						attribute.String("http.route", route),
						attribute.Int("http.response.status_code", bufferWriter.status),
					))
				} else {
					requestDuration = time.Since(oldTime)
					durationMetric.Record(r.Context(), requestDuration.Seconds(), metric.WithAttributes(
						attribute.String("http.request.method", method),
						attribute.String("http.route", route),
						attribute.Int("http.response.status_code", http.StatusInternalServerError),
					))
				}
			}()

			handler.ServeHTTP(bufferWriter, r)
			newTime = time.Now()
			handlerReturned = true

			if err := bufferWriter.WriteToOriginal(w); err != nil {
				otel.Handle(fmt.Errorf("write response in metrics middleware: %w", err))
			}
		})
	}
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch:
		return method
	default:
		return "_OTHER"
	}
}
