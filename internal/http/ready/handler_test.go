package ready_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/mock/gomock"

	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/middleware"
	"github.com/Penlk/avito-labs/internal/http/ready"
)

func TestReady(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		status     int
		bodyStatus api.HealthResponseStatus
	}{
		{"available", nil, http.StatusOK, api.Ok},
		{"unavailable", errors.New("database unavailable"), http.StatusServiceUnavailable, api.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			recorder := httptest.NewRecorder()
			db := NewMockpinger(gomock.NewController(t))
			db.EXPECT().Ping(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > time.Second {
					t.Fatal("readiness timeout was not applied")
				}
				return tt.err
			})
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			router := chi.NewRouter()
			router.Use(middleware.Logging(logger))
			router.Get("/ready", ready.New(db, time.Second).Ready)
			router.ServeHTTP(recorder, req)
			if recorder.Code != tt.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var body api.HealthResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != tt.bodyStatus {
				t.Fatalf("health status=%q", body.Status)
			}
			var record map[string]any
			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatalf("expected one JSON access log: %v", err)
			}
			if tt.err != nil {
				if record["error"] != tt.err.Error() || record["level"] != "ERROR" {
					t.Fatalf("readiness error missing from access log: %v", record)
				}
			} else if _, ok := record["error"]; ok {
				t.Fatalf("unexpected readiness error: %v", record)
			}
		})
	}
}
