package push_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Penlk/avito-labs/internal/gateway/push"
)

const acceptedResponse = `{"message_id":"message-42","accepted_at":"2026-11-22T16:42:22Z"}`

func TestPush(t *testing.T) {
	tripID := uuid.New()
	driverID := uuid.NewString()
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/push" {
			t.Errorf("method=%s path=%s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type=%q", got)
		}
		remote := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		if got := trace.SpanContextFromContext(remote); got.TraceID() != spanContext.TraceID() ||
			got.SpanID() != spanContext.SpanID() {
			t.Error("push request lost its trace context")
		}
		var body struct {
			RequestID   string            `json:"request_id"`
			RecipientID string            `json:"recipient_id"`
			Kind        string            `json:"kind"`
			Data        map[string]string `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.RequestID != "request-42" || body.RecipientID != driverID ||
			body.Kind != "REQUEST_POSITION" || body.Data["trip_id"] != tripID.String() {
			t.Errorf("unexpected push request: %+v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(acceptedResponse))
	}))
	t.Cleanup(server.Close)
	gateway := push.NewGateway(server.URL+"/", time.Second)
	req := push.NewRequest("request-42", driverID, push.Data{TripId: tripID})

	result, err := gateway.Push(trace.ContextWithSpanContext(t.Context(), spanContext), req)
	if err != nil {
		t.Fatal(err)
	}
	expectedAt := time.Date(2026, time.November, 22, 16, 42, 22, 0, time.UTC)
	if result.MessageId != "message-42" || !result.AcceptedAt.Equal(expectedAt) {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestPushHTTPError(t *testing.T) {
	for _, tt := range []struct {
		status int
		err    error
	}{
		{http.StatusBadRequest, push.ErrInvalidRequest},
		{http.StatusInternalServerError, push.ErrUnavailable},
		{http.StatusServiceUnavailable, push.ErrUnavailable},
	} {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(server.Close)
			gateway := push.NewGateway(server.URL, time.Second)
			result, err := gateway.Push(t.Context(), push.Request{})
			if !errors.Is(err, tt.err) {
				t.Fatalf("error=%v, want %v", err, tt.err)
			}
			if result != (push.Response{}) {
				t.Fatalf("unexpected result on error: %+v", result)
			}
		})
	}
}

func TestPushTooManyRequests(t *testing.T) {
	for _, tt := range []struct {
		name   string
		header string
		delay  time.Duration
		known  bool
	}{
		{"seconds", "2", 2 * time.Second, true},
		{"zero", "0", 0, true},
		{"missing", "", 0, false},
		{"invalid", "invalid", 0, false},
		{"negative", "-1", 0, false},
		{"overflow", "999999999999999999999", 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.header != "" {
					w.Header().Set("Retry-After", tt.header)
				}
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			t.Cleanup(server.Close)
			gateway := push.NewGateway(server.URL, time.Second)
			before := time.Now()
			_, err := gateway.Push(t.Context(), push.Request{})
			after := time.Now()
			var limited push.TooManyRequests
			if !errors.As(err, &limited) {
				t.Fatalf("error=%v, want TooManyRequests", err)
			}
			if !tt.known {
				if !limited.RetryAfter.IsZero() {
					t.Fatalf("unknown Retry-After became %s", limited.RetryAfter)
				}
				return
			}
			if limited.RetryAfter.Before(before.Add(tt.delay)) || limited.RetryAfter.After(after.Add(tt.delay)) {
				t.Fatalf("retry time=%s is outside expected interval", limited.RetryAfter)
			}
		})
	}
}

func TestPushInvalidResponse(t *testing.T) {
	for _, body := range []string{"", "not JSON", acceptedResponse + "{}"} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(server.Close)
			gateway := push.NewGateway(server.URL, time.Second)
			result, err := gateway.Push(t.Context(), push.Request{})
			if err == nil || !strings.Contains(err.Error(), "decode push response") {
				t.Fatalf("error=%v, want decode error", err)
			}
			if result != (push.Response{}) {
				t.Fatalf("unexpected result on error: %+v", result)
			}
		})
	}
}

func TestPushUnexpectedStatus(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)
			gateway := push.NewGateway(server.URL, time.Second)
			_, err := gateway.Push(t.Context(), push.Request{})
			if err == nil || !strings.Contains(err.Error(), "unexpected push service status") {
				t.Fatalf("error=%v, want unexpected status", err)
			}
		})
	}
}

func TestPushCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(acceptedResponse))
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	gateway := push.NewGateway(server.URL, time.Second)
	_, err := gateway.Push(ctx, push.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestPushClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	gateway := push.NewGateway(server.URL, 50*time.Millisecond)
	_, err := gateway.Push(t.Context(), push.Request{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("client timeout lost: %v", err)
	}
}

func TestPushInvalidURL(t *testing.T) {
	gateway := push.NewGateway("http://[invalid", time.Second)
	_, err := gateway.Push(t.Context(), push.Request{})
	if err == nil || !strings.Contains(err.Error(), "create push request") {
		t.Fatalf("error=%v, want request creation error", err)
	}
}

func TestPushTransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	gateway := push.NewGateway(server.URL, time.Second)
	_, err := gateway.Push(t.Context(), push.Request{})
	var transportErr *url.Error
	if !errors.As(err, &transportErr) || errors.Is(err, push.ErrUnavailable) || errors.Is(err, push.ErrInvalidRequest) {
		t.Fatalf("transport error was not preserved: %v", err)
	}
}
