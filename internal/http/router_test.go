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
	positioncreate "github.com/Penlk/avito-labs/internal/http/position/create"
	positionget "github.com/Penlk/avito-labs/internal/http/position/get"
	"github.com/Penlk/avito-labs/internal/http/ready"
	"github.com/Penlk/avito-labs/internal/http/trip/create"
	"github.com/Penlk/avito-labs/internal/http/trip/finish"
	"github.com/Penlk/avito-labs/internal/http/trip/get"
)

func TestRouter(t *testing.T) {
	const (
		tripsPath            = "/api/v1/trips"
		invalidRequestCode   = "invalid_request"
		methodNotAllowedCode = "method_not_allowed"
	)
	// These requests must be handled before calling a usecase or the database.
	router := transport.NewRouter(
		create.New(nil), get.New(nil), finish.New(nil),
		positioncreate.New(nil), positionget.New(nil),
		health.New(), ready.New(nil, time.Second),
		nil,
	)
	tests := []struct {
		name           string
		method         string
		path           string
		idempotencyKey string
		status         int
		code           string
		allow          string
	}{
		{"health", http.MethodGet, "/health", "", 200, "", ""},
		{"invalid get ID", http.MethodGet, tripsPath + "/invalid", "", 400, invalidRequestCode, ""},
		{"invalid finish ID", http.MethodPost, tripsPath + "/invalid/finish", "", 400, invalidRequestCode, ""},
		{"invalid position get ID", http.MethodGet, tripsPath + "/invalid/positions", "", 400, invalidRequestCode, ""},
		{"invalid position create ID", http.MethodPost, tripsPath + "/invalid/positions", "", 400, invalidRequestCode, ""},
		{"invalid key", http.MethodPost, tripsPath, "invalid", 400, invalidRequestCode, ""},
		{"unknown route", http.MethodGet, "/not-a-route", "", 404, "not_found", ""},
		{"wrong method", http.MethodDelete, tripsPath, "", 405, methodNotAllowedCode, http.MethodPost},
		{"unknown method", "FOO", tripsPath, "", 405, methodNotAllowedCode, http.MethodPost},
		{"wrong health method", http.MethodPost, "/health", "", 405, methodNotAllowedCode, http.MethodGet},
		{
			"wrong positions method", http.MethodDelete,
			tripsPath + "/11111111-1111-4111-8111-111111111111/positions", "",
			405, methodNotAllowedCode, "GET, POST",
		},
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
			if tt.status >= http.StatusBadRequest {
				var problem api.Problem
				if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if problem.Code != tt.code || int(problem.Status) != tt.status || problem.Type == "" || problem.Title == "" ||
					problem.Instance == nil || *problem.Instance != tt.path {
					t.Fatalf("unexpected problem: %+v", problem)
				}
				if recorder.Header().Get("Content-Type") != "application/problem+json" {
					t.Fatal("HTTP error bypassed the shared mapper")
				}
			}
			if recorder.Header().Get("Allow") != tt.allow {
				t.Fatalf("Allow=%q, want %q", recorder.Header().Get("Allow"), tt.allow)
			}
		})
	}
}
