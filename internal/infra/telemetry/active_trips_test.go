package telemetry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/Penlk/avito-labs/internal/infra/telemetry"
)

func TestActiveTripsGauge(t *testing.T) {
	failed := errors.New("database unavailable")
	var reportedErr error
	previousHandler := otel.GetErrorHandler()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { reportedErr = err }))
	t.Cleanup(func() { otel.SetErrorHandler(previousHandler) })
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
		reportedErr = nil
		var data metricdata.ResourceMetrics
		if err := reader.Collect(t.Context(), &data); err != nil {
			t.Fatalf("collection failed: %v", err)
		}
		if !errors.Is(reportedErr, test.err) {
			t.Fatalf("reported error=%v, want %v", reportedErr, test.err)
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

func TestActiveTripsFailureDoesNotBlockExport(t *testing.T) {
	failed := errors.New("database unavailable")
	reported := make(chan error, 2)
	previousHandler := otel.GetErrorHandler()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { reported <- err }))
	t.Cleanup(func() { otel.SetErrorHandler(previousHandler) })
	trips := NewMockactiveTripsCounter(gomock.NewController(t))
	trips.EXPECT().CountActive(gomock.Any()).Return(int64(0), failed).AnyTimes()

	// ForceFlush and Shutdown each trigger a collection and export.
	exported := make(chan uint64, 2)
	exporter := metricTestExporter{onExport: func(data *metricdata.ResourceMetrics) {
		var requests uint64
		for _, scope := range data.ScopeMetrics {
			for _, value := range scope.Metrics {
				switch value.Name {
				case "tripgo.trips.active":
					t.Error("failed query produced an active trips measurement")
				case "http.server.request.duration":
					for _, point := range value.Data.(metricdata.Histogram[float64]).DataPoints {
						requests += point.Count
					}
				}
			}
		}
		exported <- requests
	}}
	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(time.Hour))
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	if err := telemetry.RegisterActiveTrips(provider.Meter("trips"), trips); err != nil {
		t.Fatal(err)
	}
	httpMetrics, err := telemetry.NewHTTPMetrics(provider.Meter("http"))
	if err != nil {
		t.Fatal(err)
	}
	httpMetrics.Duration.Record(t.Context(), 0.01)
	if err := provider.ForceFlush(t.Context()); err != nil {
		t.Fatalf("export failed because of an unrelated gauge: %v", err)
	}
	select {
	case count := <-exported:
		if count != 1 {
			t.Fatalf("exported HTTP measurements=%d, want 1", count)
		}
	default:
		t.Fatal("HTTP histogram did not reach the exporter")
	}
	select {
	case err := <-reported:
		if !errors.Is(err, failed) {
			t.Fatalf("reported error=%v, want %v", err, failed)
		}
	default:
		t.Fatal("database failure was not reported")
	}
}

type metricTestExporter struct {
	onExport func(*metricdata.ResourceMetrics)
}

func (metricTestExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return sdkmetric.DefaultTemporalitySelector(kind)
}

func (metricTestExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

func (e metricTestExporter) Export(_ context.Context, data *metricdata.ResourceMetrics) error {
	e.onExport(data)
	return nil
}

func (metricTestExporter) ForceFlush(context.Context) error { return nil }
func (metricTestExporter) Shutdown(context.Context) error   { return nil }
