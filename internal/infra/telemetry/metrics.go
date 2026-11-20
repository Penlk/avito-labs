package telemetry

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type HTTPMetrics struct {
	Duration       metric.Float64Histogram
	ActiveRequests metric.Int64UpDownCounter
}

func NewHTTPMetrics(meter metric.Meter) (*HTTPMetrics, error) {
	duration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(DurationBuckets...))
	if err != nil {
		return nil, fmt.Errorf("HTTP duration histogram: %w", err)
	}
	active, err := meter.Int64UpDownCounter("http.server.active_requests")
	if err != nil {
		return nil, fmt.Errorf("HTTP active requests counter: %w", err)
	}
	return &HTTPMetrics{Duration: duration, ActiveRequests: active}, nil
}
