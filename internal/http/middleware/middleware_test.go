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
		name             string
		err              error
		panic            bool
		panicBeforeWrite bool
		status           int
		errorType        string
	}{
		{name: "success", status: 200},
		{name: "client error", err: domain.ErrDriverBusy, status: 409, errorType: "409"},
		{name: "internal error", err: errors.New("private\r\nFORGED"), status: 500, errorType: "500"},
		{name: "panic before write", panicBeforeWrite: true, status: 500, errorType: "panic"},
		{name: "panic after write", panic: true, status: 500, errorType: "panic"},
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
			httpMetrics, err := telemetry.NewHTTPMetrics(meters.Meter("test"))
			if err != nil {
				t.Fatal(err)
			}
			router := chi.NewRouter()
			router.Use(
				middleware.Recover(logger),
				middleware.Tracing(traces.Tracer("test")),
				middleware.Logging(logger),
				middleware.Metrics(httpMetrics.Duration, httpMetrics.ActiveRequests),
			)
			router.Get(routePattern, func(w http.ResponseWriter, r *http.Request) {
				if test.panicBeforeWrite {
					panic("private panic")
				}
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
			req := httptest.NewRequest(http.MethodGet, "/trips/secret-id?secret=value", nil)
			req.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != test.status {
				t.Fatalf("status=%d", recorder.Code)
			}
			if test.status == http.StatusOK && recorder.Body.String() != "ok" {
				t.Fatalf("response body=%q, want ok", recorder.Body.String())
			}
			if test.status >= http.StatusBadRequest && recorder.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("problem content type=%q", recorder.Header().Get("Content-Type"))
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
			errorType, hasErrorType := attrs.Value("error.type")
			if errorType.AsString() != test.errorType || hasErrorType != (test.errorType != "") {
				t.Fatalf("error.type=%v, want %q", errorType, test.errorType)
			}
			if test.errorType == "" && span.Status.Code == codes.Error {
				t.Fatal("successful request marked as error")
			}
			assertLogs(t, logs.String(), span, test.status, test.panic || test.panicBeforeWrite, test.err)
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

func assertLogs(t *testing.T, logs string, span tracetest.SpanStub, status int, panicked bool, requestErr error) {
	t.Helper()
	accesses, panics := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid log: %q", line)
		}
		if record["service"] != "test-service" {
			t.Fatalf("missing service: %v", record)
		}
		if record["msg"] == "request panic" {
			panics++
			stack, _ := record["stack"].(string)
			if record["level"] != "ERROR" || record["panic"] != "private panic" || stack == "" {
				t.Fatalf("incorrect panic log: %v", record)
			}
			continue
		}
		if record["trace_id"] != span.SpanContext.TraceID().String() ||
			record["span_id"] != span.SpanContext.SpanID().String() {
			t.Fatalf("missing correlation: %v", record)
		}
		accesses++
		if status >= http.StatusInternalServerError && requestErr != nil {
			if record["error"] != requestErr.Error() {
				t.Fatalf("original error missing from access log: %v", record)
			}
		} else if _, ok := record["error"]; ok {
			t.Fatalf("unexpected error details in access log: %v", record)
		}
		level, message := "INFO", "request handled"
		if status >= 500 {
			level, message = "ERROR", "request can't be served"
		}
		if record["msg"] != message || record["level"] != level ||
			record["status"] != float64(status) || record["route"] != routePattern {
			t.Fatalf("incorrect access log: %v", record)
		}
	}
	if accesses != 1 {
		t.Fatalf("access records=%d", accesses)
	}
	wantPanics := 0
	if panicked {
		wantPanics = 1
	}
	if panics != wantPanics {
		t.Fatalf("panic records=%d, want %d", panics, wantPanics)
	}
}
