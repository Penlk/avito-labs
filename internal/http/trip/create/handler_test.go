package create_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/trip/create"
)

const validBody = `{
    "driver_id":"11111111-1111-4111-8111-111111111111",
    "user_id":"22222222-2222-4222-8222-222222222222",
    "start_point":{"latitude":55,"longitude":37},
    "end_point":{"latitude":56,"longitude":38},
    "price":1000
}`

func TestCreateTrip(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(validBody))
	recorder := httptest.NewRecorder()
	usecase := NewMocktripCreator(gomock.NewController(t))
	var createdID uuid.UUID
	usecase.EXPECT().Create(req.Context(), gomock.Any()).DoAndReturn(
		func(_ context.Context, trip domain.Trip) (domain.Trip, error) {
			if trip.DriverId().String() != "11111111-1111-4111-8111-111111111111" ||
				trip.UserId().String() != "22222222-2222-4222-8222-222222222222" ||
				trip.Price().Rubles() != 1000 ||
				trip.StartPoint().Latitude() != 55 || trip.EndPoint().Longitude() != 38 {
				t.Fatalf("request was converted incorrectly: %+v", trip)
			}
			createdID = trip.Id()
			return trip, nil
		},
	)

	create.New(usecase).CreateTrip(recorder, req, api.CreateTripParams{})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Location"); got != "/api/v1/trips/"+createdID.String() {
		t.Fatalf("location=%q", got)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatal("wrong response content type")
	}
	var body api.Trip
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Id != createdID || body.Status != api.Active ||
		body.Price == nil || *body.Price != 1000 ||
		body.StartPoint.Latitude == nil || *body.StartPoint.Latitude != 55 ||
		body.EndPoint.Longitude == nil || *body.EndPoint.Longitude != 38 {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestCreateTripValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"null", "null"},
		{"missing fields", "{}"},
		{"array", "[]"},
		{"multiple objects", validBody + "{}"},
		{"unknown field", strings.Replace(validBody, "\"price\":1000", "\"extra\":1,\"price\":1000", 1)},
		{"missing price", strings.Replace(validBody, ",\n    \"price\":1000", "", 1)},
		{"null price", strings.Replace(validBody, "\"price\":1000", "\"price\":null", 1)},
		{"fractional price", strings.Replace(validBody, "1000", "1.5", 1)},
		{"negative price", strings.Replace(validBody, "1000", "-1", 1)},
		{"invalid latitude", strings.Replace(validBody, "\"latitude\":55", "\"latitude\":91", 1)},
		{"missing latitude", strings.Replace(validBody, "\"latitude\":55,", "", 1)},
		{"zero driver", strings.Replace(validBody, "11111111-1111-4111-8111-111111111111", uuid.Nil.String(), 1)},
		{"invalid uuid", strings.Replace(validBody, "11111111-1111-4111-8111-111111111111", "invalid", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usecase := NewMocktripCreator(gomock.NewController(t))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()
			create.New(usecase).CreateTrip(recorder, req, api.CreateTripParams{})
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var problem api.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != "invalid_request" || problem.Detail == nil {
				t.Fatalf("unexpected problem: %+v", problem)
			}
		})
	}
}

func TestCreateTripDriverBusy(t *testing.T) {
	usecase := NewMocktripCreator(gomock.NewController(t))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(validBody))
	recorder := httptest.NewRecorder()
	usecase.EXPECT().Create(req.Context(), gomock.Any()).
		Return(domain.Trip{}, fmt.Errorf("create trip: %w", domain.ErrDriverBusy))
	create.New(usecase).CreateTrip(recorder, req, api.CreateTripParams{})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var problem api.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "driver_busy" {
		t.Fatalf("code=%q", problem.Code)
	}
}
