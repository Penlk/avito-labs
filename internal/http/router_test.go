package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	api "github.com/Penlk/avito-labs/internal/generated"
	transport "github.com/Penlk/avito-labs/internal/http"
	"github.com/Penlk/avito-labs/internal/http/health"
	"github.com/Penlk/avito-labs/internal/http/ready"
	"github.com/Penlk/avito-labs/internal/http/trip/create"
	"github.com/Penlk/avito-labs/internal/http/trip/finish"
	"github.com/Penlk/avito-labs/internal/http/trip/get"
)

func TestRouter(t *testing.T) {
	// These requests must be handled before calling a usecase or the database.
	router := transport.NewRouter(
		create.New(nil), get.New(nil), finish.New(nil),
		health.New(), ready.New(nil, time.Second),
	)
	tests := []struct {
		name           string
		method         string
		path           string
		idempotencyKey string
		status         int
	}{
		{"health", http.MethodGet, "/health", "", 200},
		{"invalid get ID", http.MethodGet, "/api/v1/trips/invalid", "", 400},
		{"invalid finish ID", http.MethodPost, "/api/v1/trips/invalid/finish", "", 400},
		{"invalid key", http.MethodPost, "/api/v1/trips", "invalid", 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.idempotencyKey != "" {
				req.Header.Set("Idempotency-Key", tt.idempotencyKey)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != tt.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if tt.status == http.StatusBadRequest {
				var problem api.Problem
				if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if problem.Code != "invalid_request" ||
					problem.Instance == nil || *problem.Instance != tt.path {
					t.Fatalf("unexpected problem: %+v", problem)
				}
				if recorder.Header().Get("Content-Type") != "application/problem+json" {
					t.Fatal("parameter error bypassed the shared mapper")
				}
			}
		})
	}
}
