package telemetry_test

import (
	"io"
	"log/slog"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/Penlk/avito-labs/internal/infra/telemetry"
)

func TestPositionSavedCounter(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(t.Context()); err != nil {
			t.Error(err)
		}
	})
	events, err := telemetry.NewEvents(provider.Meter("test"), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	events.PositionSaved(t.Context())
	events.PositionSaved(t.Context())
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "tripgo.positions.saved" {
				continue
			}
			values, ok := metric.Data.(metricdata.Sum[int64])
			if !ok || !values.IsMonotonic || len(values.DataPoints) != 1 || values.DataPoints[0].Value != 2 ||
				values.DataPoints[0].Attributes.Len() != 0 {
				t.Fatalf("unexpected saved positions counter: %+v", metric.Data)
			}
			return
		}
	}
	t.Fatal("saved positions counter was not exported")
}
