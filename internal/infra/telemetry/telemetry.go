package telemetry

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Providers struct {
	Traces  *sdktrace.TracerProvider
	Metrics *sdkmetric.MeterProvider
}

// Exporters read endpoint, TLS and timeout settings from standard OTEL_* env.
// Construction does not wait for the collector; exports happen asynchronously.
func New(ctx context.Context, protocol, service string) (*Providers, error) {
	traces, metrics, err := exporters(ctx, protocol)
	if err != nil {
		return nil, err
	}
	res, err := newResource(ctx, service)
	if err != nil {
		return nil, errors.Join(err, traces.Shutdown(ctx), metrics.Shutdown(ctx))
	}
	return &Providers{
		Traces: sdktrace.NewTracerProvider(sdktrace.WithResource(res),
			sdktrace.WithBatcher(traces)),
		Metrics: sdkmetric.NewMeterProvider(sdkmetric.WithResource(res),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metrics))),
	}, nil
}

func exporters(ctx context.Context, protocol string) (
	sdktrace.SpanExporter, sdkmetric.Exporter, error,
) {
	var traces sdktrace.SpanExporter
	var metrics sdkmetric.Exporter
	var err error
	switch protocol {
	case "grpc":
		traces, err = otlptracegrpc.New(ctx)
	case "http/protobuf":
		traces, err = otlptracehttp.New(ctx)
	default:
		return nil, nil, fmt.Errorf("unsupported OTEL_EXPORTER_OTLP_PROTOCOL: %q", protocol)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("trace exporter: %w", err)
	}
	if protocol == "grpc" {
		metrics, err = otlpmetricgrpc.New(ctx)
	} else {
		metrics, err = otlpmetrichttp.New(ctx)
	}
	if err != nil {
		return nil, nil, errors.Join(err, traces.Shutdown(ctx))
	}
	return traces, metrics, nil
}

func (p *Providers) Shutdown(ctx context.Context) error {
	// Both exporters share the shutdown budget, neither can starve the other.
	results := make(chan error, 2)
	go func() { results <- p.Traces.Shutdown(ctx) }()
	go func() { results <- p.Metrics.Shutdown(ctx) }()
	var err error
	for range 2 {
		select {
		case e := <-results:
			err = errors.Join(err, e)
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		}
	}
	return err
}

// DurationBuckets are seconds, shared by HTTP and database histograms.
var DurationBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
