package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Penlk/avito-labs/internal/http/middleware"
)

func TestTracingPropagation(t *testing.T) {
	const parentTraceID = "11111111111111111111111111111111"
	const parentSpanID = "2222222222222222"
	for _, test := range []struct {
		name, traceparent, tracestate string
		parent, sampled               bool
	}{
		{
			name: "remote parent", traceparent: "00-" + parentTraceID + "-" + parentSpanID + "-01",
			tracestate: "vendor=opaque", parent: true, sampled: true,
		},
		{name: "no header", sampled: true},
		{name: "invalid header", traceparent: "invalid", sampled: true},
		{
			name: "unsampled parent", traceparent: "00-" + parentTraceID + "-" + parentSpanID + "-00",
			parent: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(
				sdktrace.WithSyncer(exporter), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())))
			t.Cleanup(func() {
				if err := provider.Shutdown(context.WithoutCancel(t.Context())); err != nil {
					t.Error(err)
				}
			})
			var handlerSpan trace.SpanContext
			router := chi.NewRouter()
			router.Use(middleware.Tracing(provider.Tracer("test")))
			router.Get(routePattern, func(w http.ResponseWriter, r *http.Request) {
				handlerSpan = trace.SpanContextFromContext(r.Context())
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/trips/trip-id", nil)
			if test.traceparent != "" {
				req.Header.Set("traceparent", test.traceparent)
			}
			if test.tracestate != "" {
				req.Header.Set("tracestate", test.tracestate)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status=%d", recorder.Code)
			}
			if !handlerSpan.IsValid() || handlerSpan.IsRemote() || handlerSpan.IsSampled() != test.sampled {
				t.Fatalf("invalid HTTP span context in handler: %v", handlerSpan)
			}
			if test.parent && handlerSpan.TraceID().String() != parentTraceID {
				t.Fatalf("trace ID=%s, want %s", handlerSpan.TraceID(), parentTraceID)
			}
			if handlerSpan.SpanID().String() == parentSpanID {
				t.Fatal("HTTP span reused the parent's span ID")
			}
			if handlerSpan.TraceState().String() != test.tracestate {
				t.Fatalf("tracestate=%q, want %q", handlerSpan.TraceState(), test.tracestate)
			}
			spans := exporter.GetSpans()
			if !test.sampled {
				if len(spans) != 0 {
					t.Fatalf("unsampled spans were exported: %v", spans)
				}
				return
			}
			if len(spans) != 1 {
				t.Fatalf("exported spans=%d, want 1", len(spans))
			}
			span := spans[0]
			if span.Name != "GET "+routePattern || span.SpanKind != trace.SpanKindServer {
				t.Fatalf("unexpected HTTP span: %+v", span)
			}
			if !span.SpanContext.Equal(handlerSpan) {
				t.Fatal("handler did not receive the HTTP span context")
			}
			if test.parent {
				if span.Parent.SpanID().String() != parentSpanID || !span.Parent.IsRemote() {
					t.Fatalf("remote parent lost: %v", span.Parent)
				}
			} else if span.Parent.IsValid() {
				t.Fatalf("expected a root span, got parent %v", span.Parent)
			}
		})
	}
}
