package get_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/trip/get"
)

func TestGetTrip(t *testing.T) {
	trip, err := domain.NewTrip(
		uuid.New(), uuid.New(), vo.Coordinates{}, vo.Price{}, vo.Coordinates{},
	)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+trip.Id().String(), nil)
	recorder := httptest.NewRecorder()
	usecase := NewMocktripGetter(gomock.NewController(t))
	usecase.EXPECT().Get(req.Context(), trip.Id()).Return(*trip, nil)

	get.New(usecase).GetTrip(recorder, req, trip.Id())
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body api.Trip
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Id != trip.Id() || body.DriverId != trip.DriverId() || body.Status != api.Active {
		t.Fatalf("unexpected trip: %+v", body)
	}
}

func TestGetTripNotFound(t *testing.T) {
	id := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil)
	recorder := httptest.NewRecorder()
	usecase := NewMocktripGetter(gomock.NewController(t))
	usecase.EXPECT().Get(req.Context(), id).
		Return(domain.Trip{}, fmt.Errorf("get trip: %w", domain.ErrTripNotFound))

	get.New(usecase).GetTrip(recorder, req, id)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var problem api.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "trip_not_found" {
		t.Fatalf("code=%q", problem.Code)
	}
}
