package create_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip"
	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/position/create"
)

const (
	recordedAtText = "2026-09-01T10:05:00Z"
	validBody      = `{"coordinates":{"latitude":59.93,"longitude":30.33},"recorded_at":"` + recordedAtText + `"}`
)

func TestCreateTripPosition(t *testing.T) {
	for _, timestamp := range []string{recordedAtText, "0001-01-01T00:00:00Z"} {
		t.Run(timestamp, func(t *testing.T) {
			tripID := uuid.New()
			input := strings.Replace(validBody, recordedAtText, timestamp, 1)
			path := "/api/v1/trips/" + tripID.String() + "/positions"
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(input))
			recorder := httptest.NewRecorder()
			usecase := NewMockPositionCreator(gomock.NewController(t))
			recordedAt, err := time.Parse(time.RFC3339, timestamp)
			if err != nil {
				t.Fatal(err)
			}
			usecase.EXPECT().Create(req.Context(), gomock.Any()).DoAndReturn(
				func(_ context.Context, pos domain.Position) (domain.Position, error) {
					if pos.Id() != 0 || pos.TripId() != tripID || !pos.RecordedAt().Equal(recordedAt) ||
						pos.Coordinates().Latitude() != 59.93 || pos.Coordinates().Longitude() != 30.33 {
						t.Fatalf("request was converted incorrectly: %+v", pos)
					}
					return *domain.RestorePosition(42, tripID, pos.Coordinates(), pos.RecordedAt()), nil
				},
			)

			create.New(usecase).CreateTripPosition(recorder, req, tripID)
			if recorder.Code != http.StatusCreated {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("content type=%q", got)
			}
			var body api.TripPosition
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Id == nil || *body.Id != 42 || body.TripId != tripID || !body.RecordedAt.Equal(recordedAt) ||
				body.Coordinates.Latitude == nil || *body.Coordinates.Latitude != 59.93 ||
				body.Coordinates.Longitude == nil || *body.Coordinates.Longitude != 30.33 {
				t.Fatalf("unexpected response: %+v", body)
			}
		})
	}
}

func TestCreateTripPositionValidation(t *testing.T) {
	future := time.Now().Add(2 * time.Minute).UTC().Format(time.RFC3339)
	tests := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"null", "null"},
		{"missing fields", "{}"},
		{"array", "[]"},
		{"multiple objects", validBody + "{}"},
		{"trailing garbage", validBody + "garbage"},
		{"unknown field", strings.TrimSuffix(validBody, "}") + `,"extra":1}`},
		{"unknown coordinate", `{"coordinates":{"latitude":0,"longitude":0,"extra":1}}`},
		{"missing coordinates", `{"recorded_at":"` + recordedAtText + `"}`},
		{"null coordinates", `{"coordinates":null,"recorded_at":"` + recordedAtText + `"}`},
		{"missing latitude", `{"coordinates":{"longitude":0}}`},
		{"null latitude", `{"coordinates":{"latitude":null,"longitude":0}}`},
		{"missing longitude", `{"coordinates":{"latitude":0}}`},
		{"null longitude", `{"coordinates":{"latitude":0,"longitude":null}}`},
		{"invalid latitude", strings.Replace(validBody, "59.93", "91", 1)},
		{"invalid longitude", strings.Replace(validBody, "30.33", "181", 1)},
		{"wrong coordinate type", `{"coordinates":{"latitude":"invalid","longitude":0}}`},
		{"wrong coordinates type", `{"coordinates":1}`},
		{"missing recorded_at", `{"coordinates":{"latitude":0,"longitude":0}}`},
		{"null recorded_at", strings.Replace(validBody, `"`+recordedAtText+`"`, "null", 1)},
		{"invalid recorded_at", strings.Replace(validBody, recordedAtText, "invalid", 1)},
		{"wrong recorded_at type", strings.Replace(validBody, `"`+recordedAtText+`"`, "123", 1)},
		{"future recorded_at", strings.Replace(validBody, recordedAtText, future, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tripID := uuid.New()
			usecase := NewMockPositionCreator(gomock.NewController(t))
			path := "/api/v1/trips/" + tripID.String() + "/positions"
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()
			create.New(usecase).CreateTripPosition(recorder, req, tripID)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var problem api.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != "invalid_request" || problem.Detail == nil || *problem.Detail == "" {
				t.Fatalf("unexpected problem: %+v", problem)
			}
			if tt.name == "future recorded_at" && !strings.Contains(*problem.Detail, "recorded_at") {
				t.Fatalf("wrong validation field: %s", *problem.Detail)
			}
		})
	}
}

func TestCreateTripPositionUsecaseErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{"completed", trip.ErrTripCompleted, http.StatusConflict, "trip_completed"},
		{"internal", errors.New("private database details"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tripID := uuid.New()
			path := "/api/v1/trips/" + tripID.String() + "/positions"
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(validBody))
			recorder := httptest.NewRecorder()
			usecase := NewMockPositionCreator(gomock.NewController(t))
			usecase.EXPECT().Create(req.Context(), gomock.Any()).
				Return(domain.Position{}, fmt.Errorf("create position: %w", tt.err))

			create.New(usecase).CreateTripPosition(recorder, req, tripID)
			if recorder.Code != tt.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Fatalf("content type=%q", got)
			}
			var problem api.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != tt.code || problem.Instance == nil || *problem.Instance != path {
				t.Fatalf("unexpected problem: %+v", problem)
			}
			if tt.status == http.StatusInternalServerError && strings.Contains(recorder.Body.String(), tt.err.Error()) {
				t.Fatal("internal error appeared in the response")
			}
		})
	}
}
