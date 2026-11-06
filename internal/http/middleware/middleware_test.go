package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/http/domainerror"
	"github.com/Penlk/avito-labs/internal/http/middleware"
	"github.com/Penlk/avito-labs/internal/infra/telemetry"
)

const routePattern = "/trips/{tripId}"

func TestMiddleware(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		panic  bool
		status int
	}{
		{name: "success", status: 200},
		{name: "client error", err: domain.ErrDriverBusy, status: 409},
		{name: "internal error", err: errors.New("private\r\nFORGED"), status: 500},
		{name: "panic after write", panic: true, status: 500},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger, err := telemetry.NewLogger(&logs, "info", "test-service")
			if err != nil {
				t.Fatal(err)
			}
			exporter := tracetest.NewInMemoryExporter()
			traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			reader := sdkmetric.NewManualReader()
			meters := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() {
				if err := traces.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
				if err := meters.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			})
			metrics, err := middleware.NewMetrics(meters.Meter("test"))
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			router.Get(routePattern, func(w http.ResponseWriter, r *http.Request) {
				if test.err != nil {
					domainerror.Write(w, r, test.err)
					return
				}
				if _, err := w.Write([]byte("ok")); err != nil {
					t.Fatal(err)
				}
				if test.panic {
					panic("private panic")
				}
			})
			handler := middleware.Recover(logger, router)(middleware.Tracing(traces)(
				middleware.Logging(logger)(metrics.Handler(router))))
			req := httptest.NewRequest(http.MethodGet, "/trips/secret-id?secret=value", nil)
			req.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != test.status {
				t.Fatalf("status=%d", recorder.Code)
			}
			if test.status == 500 && (strings.Contains(recorder.Body.String(), "private") ||
				!json.Valid(recorder.Body.Bytes())) {
				t.Fatalf("unsafe response: %s", recorder.Body.String())
			}
			spans := exporter.GetSpans()
			if len(spans) != 1 || spans[0].Name != "GET "+routePattern {
				t.Fatalf("spans=%+v", spans)
			}
			span := spans[0]
			if span.Parent.SpanID().String() != "2222222222222222" {
				t.Fatal("parent lost")
			}
			if test.status >= 400 && span.Status.Code != codes.Error {
				t.Fatalf("status=%v", span.Status)
			}
			attrs := attribute.NewSet(span.Attributes...)
			value, _ := attrs.Value("http.response.status_code")
			if value.AsInt64() != int64(test.status) {
				t.Fatalf("span status=%v", value)
			}
			if test.status >= 400 {
				if _, ok := attrs.Value("error.type"); !ok {
					t.Fatal("missing error.type")
				}
			}
			assertLogs(t, logs.String(), span, test.status)
			var data metricdata.ResourceMetrics
			if err := reader.Collect(t.Context(), &data); err != nil {
				t.Fatal(err)
			}
			assertMetrics(t, data, test.status)
		})
	}
}

func assertMetrics(t *testing.T, data metricdata.ResourceMetrics, status int) {
	t.Helper()
	foundDuration, foundActive := false, false
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch value := m.Data.(type) {
			case metricdata.Histogram[float64]:
				foundDuration = m.Name == "http.server.request.duration" && m.Unit == "s"
				if len(value.DataPoints) != 1 {
					t.Fatalf("histogram=%v", value)
				}
				point := value.DataPoints[0]
				route, _ := point.Attributes.Value("http.route")
				code, _ := point.Attributes.Value("http.response.status_code")
				if point.Count != 1 || point.Sum < 0 || route.AsString() != routePattern || code.AsInt64() != int64(status) {
					t.Fatalf("histogram point=%v", point)
				}
			case metricdata.Sum[int64]:
				foundActive = m.Name == "http.server.active_requests" && !value.IsMonotonic
				if len(value.DataPoints) != 1 || value.DataPoints[0].Value != 0 {
					t.Fatalf("active=%v", value)
				}
			}
		}
	}
	if !foundDuration || !foundActive {
		t.Fatalf("missing metrics: %+v", data)
	}
}

func assertLogs(t *testing.T, logs string, span tracetest.SpanStub, status int) {
	t.Helper()
	accesses := 0
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid log: %q", line)
		}
		if record["trace_id"] != span.SpanContext.TraceID().String() ||
			record["span_id"] != span.SpanContext.SpanID().String() || record["service"] != "test-service" {
			t.Fatalf("missing correlation: %v", record)
		}
		if record["msg"] != "http request" {
			continue
		}
		accesses++
		level := "INFO"
		if status >= 500 {
			level = "ERROR"
		}
		if record["level"] != level || record["status"] != float64(status) || record["route"] != routePattern {
			t.Fatalf("incorrect access log: %v", record)
		}
	}
	if accesses != 1 {
		t.Fatalf("access records=%d", accesses)
	}
}
