package ready_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	api "github.com/Penlk/avito-labs/internal/generated"
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
			ready.New(db, time.Second).Ready(recorder, req)
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
		})
	}
}
