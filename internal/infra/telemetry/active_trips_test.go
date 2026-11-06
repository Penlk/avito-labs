package telemetry_test

import (
	"context"
	"errors"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/Penlk/avito-labs/internal/infra/telemetry"
)

func TestActiveTripsGauge(t *testing.T) {
	failed := errors.New("database unavailable")
	trips := NewMockactiveTripsCounter(gomock.NewController(t))
	gomock.InOrder(
		trips.EXPECT().CountActive(gomock.Any()).Return(int64(7), nil),
		trips.EXPECT().CountActive(gomock.Any()).Return(int64(2), nil),
		trips.EXPECT().CountActive(gomock.Any()).Return(int64(0), failed),
		trips.EXPECT().CountActive(gomock.Any()).Return(int64(0), nil),
	)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})
	if err := telemetry.RegisterActiveTrips(provider.Meter("test"), trips); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		value int64
		err   error
	}{
		{7, nil}, {2, nil}, {0, failed}, {0, nil},
	} {
		var data metricdata.ResourceMetrics
		err := reader.Collect(t.Context(), &data)
		if !errors.Is(err, test.err) {
			t.Fatalf("collect error=%v, want %v", err, test.err)
		}
		var points []metricdata.DataPoint[int64]
		for _, scope := range data.ScopeMetrics {
			for _, value := range scope.Metrics {
				if value.Name == "tripgo.trips.active" {
					points = value.Data.(metricdata.Gauge[int64]).DataPoints
				}
			}
		}
		if test.err != nil {
			if len(points) != 0 {
				t.Fatalf("failed query produced a misleading value: %v", points)
			}
			continue
		}
		if len(points) != 1 || points[0].Value != test.value {
			t.Fatalf("gauge=%v, want %d", points, test.value)
		}
		if points[0].Attributes.Len() != 0 {
			t.Fatal("business gauge must have no labels")
		}
	}
}
