package middleware

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/Penlk/avito-labs/internal/http/requeststate"
)

type Metrics struct {
	duration metric.Float64Histogram
	active   metric.Int64UpDownCounter
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	duration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(
			.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10))
	if err != nil {
		return nil, fmt.Errorf("HTTP histogram: %w", err)
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests")
	if err != nil {
		return nil, fmt.Errorf("active requests: %w", err)
	}
	return &Metrics{duration: duration, active: active}, nil
}

func (m *Metrics) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := requeststate.From(r.Context())
		attrs := []attribute.KeyValue{
			attribute.String("http.request.method", r.Method),
			attribute.String("http.route", state.Route),
		}
		m.active.Add(r.Context(), 1, metric.WithAttributes(attrs...))
		start := time.Now()
		completed := false
		defer func() {
			if !completed {
				state.Status = http.StatusInternalServerError
			}
			m.active.Add(r.Context(), -1, metric.WithAttributes(attrs...))
			attrs = append(attrs, attribute.Int("http.response.status_code", state.Status))
			m.duration.Record(r.Context(), time.Since(start).Seconds(),
				metric.WithAttributes(attrs...))
		}()
		next.ServeHTTP(w, r)
		completed = true
	})
}
