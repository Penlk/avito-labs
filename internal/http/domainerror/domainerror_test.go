package domainerror_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", domain.ErrTripNotFound, 404, "trip_not_found"},
		{"completed", domain.ErrTripCompleted, 409, "trip_completed"},
		{"busy", domain.ErrDriverBusy, 409, "driver_busy"},
		{"domain validation", domain.ValidationError{Field: "price", Reason: "is required"}, 400, "invalid_request"},
		{"internal", errors.New("private database details"), 500, "internal_error"},
		{
			"invalid request",
			domainerror.InvalidRequest.WithDetail("price is required"),
			400,
			"invalid_request",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := domainerror.Problem(fmt.Errorf("usecase: %w", tt.err))
			if status != tt.status || int(body.Status) != tt.status || body.Code != tt.code {
				t.Fatalf("unexpected problem: status=%d body=%+v", status, body)
			}
			if body.Type == "" || body.Title == "" {
				t.Fatalf("incomplete problem: %+v", body)
			}
			if tt.status == 500 && body.Detail != nil {
				t.Fatalf("internal details leaked: %q", *body.Detail)
			}
			if tt.status == 400 &&
				(body.Detail == nil || (*body.Detail != "price is required" && *body.Detail != "price: is required")) {
				t.Fatalf("validation details lost: %+v", body)
			}
		})
	}
}

func TestWriteHidesInternalErrorAndSetsInstance(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/example", nil)
	recorder := httptest.NewRecorder()
	domainerror.Write(recorder, req, errors.New("private database details"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content type=%q", got)
	}
	if strings.Contains(recorder.Body.String(), "private database details") {
		t.Fatal("internal error appeared in the response")
	}
	var problem api.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Instance == nil || *problem.Instance != req.URL.Path {
		t.Fatalf("unexpected instance: %v", problem.Instance)
	}
	if domainerror.InternalError.Instance != nil {
		t.Fatal("request modified the shared error template")
	}
}
