package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// Read committed database state on every collection, including after restart.
func RegisterActiveTrips(meter metric.Meter, trips activeTripsCounter) error {
	_, err := meter.Int64ObservableGauge("tripgo.trips.active",
		metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			count, err := trips.CountActive(ctx)
			if err != nil {
				return fmt.Errorf("observe active trips: %w", err)
			}
			observer.Observe(count)
			return nil
		}))
	if err != nil {
		return fmt.Errorf("active trips gauge: %w", err)
	}
	return nil
}
