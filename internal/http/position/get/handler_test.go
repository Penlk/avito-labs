package get_test

import (
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
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/position/get"
)

func TestListTripPositions(t *testing.T) {
	tripID := uuid.New()
	coordinates, err := vo.NewCoordinates(55, 37)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, time.September, 1, 10, 5, 0, 0, time.UTC)
	positions := []domain.Position{
		*domain.RestorePosition(41, tripID, coordinates, at),
		*domain.RestorePosition(42, tripID, coordinates, at.Add(time.Second)),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String()+"/positions", nil)
	recorder := httptest.NewRecorder()
	usecase := NewMockpositionLister(gomock.NewController(t))
	usecase.EXPECT().GetByTripId(req.Context(), tripID).Return(positions, nil)

	get.New(usecase).ListTripPositions(recorder, req, tripID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	var body api.TripPositionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response does not match the contract: %v; body=%s", err, recorder.Body.String())
	}
	if len(body.Positions) != len(positions) {
		t.Fatalf("positions=%+v", body.Positions)
	}
	for i, expected := range positions {
		got := body.Positions[i]
		if got.Id == nil || *got.Id != expected.Id() || got.TripId != tripID || !got.RecordedAt.Equal(expected.RecordedAt()) ||
			got.Coordinates.Latitude == nil || *got.Coordinates.Latitude != expected.Coordinates().Latitude() ||
			got.Coordinates.Longitude == nil || *got.Coordinates.Longitude != expected.Coordinates().Longitude() {
			t.Fatalf("position %d: got=%+v want=%+v", i, got, expected)
		}
	}
}

func TestListTripPositionsEmpty(t *testing.T) {
	for _, tt := range []struct {
		name      string
		positions []domain.Position
	}{
		{"nil slice", nil},
		{"empty slice", []domain.Position{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tripID := uuid.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String()+"/positions", nil)
			recorder := httptest.NewRecorder()
			usecase := NewMockpositionLister(gomock.NewController(t))
			usecase.EXPECT().GetByTripId(req.Context(), tripID).Return(tt.positions, nil)

			get.New(usecase).ListTripPositions(recorder, req, tripID)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var body api.TripPositionsResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Positions == nil || len(body.Positions) != 0 {
				t.Fatalf("expected positions to be an empty array: %s", recorder.Body.String())
			}
		})
	}
}

func TestListTripPositionsErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{"internal", errors.New("private database details"), http.StatusInternalServerError, "internal_error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tripID := uuid.New()
			path := "/api/v1/trips/" + tripID.String() + "/positions"
			req := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()
			usecase := NewMockpositionLister(gomock.NewController(t))
			usecase.EXPECT().GetByTripId(req.Context(), tripID).
				Return(nil, fmt.Errorf("get route: %w", tt.err))

			get.New(usecase).ListTripPositions(recorder, req, tripID)
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
