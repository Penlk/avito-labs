package finish_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/trip/finish"
)

func TestFinishTrip(t *testing.T) {
	trip, err := domain.NewTrip(
		uuid.New(), uuid.New(), vo.Coordinates{}, vo.Price{}, vo.Coordinates{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !trip.TryComplete() {
		t.Fatal("could not prepare completed trip")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+trip.Id().String()+"/finish", nil)
	recorder := httptest.NewRecorder()
	usecase := NewMocktripFinisher(gomock.NewController(t))
	usecase.EXPECT().Finish(req.Context(), trip.Id()).Return(*trip, nil)

	finish.New(usecase).FinishTrip(recorder, req, trip.Id())
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body api.Trip
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Id != trip.Id() || body.Status != api.Completed ||
		body.FinishedAt == nil || !body.FinishedAt.Equal(*trip.FinishedAt()) {
		t.Fatalf("unexpected trip: %+v", body)
	}
}

func TestFinishTripErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"completed", domain.ErrTripCompleted, 409, "trip_completed"},
		{"not found", domain.ErrTripNotFound, 404, "trip_not_found"},
		{"internal", errors.New("private database details"), 500, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil)
			recorder := httptest.NewRecorder()
			usecase := NewMocktripFinisher(gomock.NewController(t))
			usecase.EXPECT().Finish(req.Context(), id).
				Return(domain.Trip{}, fmt.Errorf("finish trip: %w", tt.err))
			finish.New(usecase).FinishTrip(recorder, req, id)
			if recorder.Code != tt.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var problem api.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if problem.Code != tt.code {
				t.Fatalf("code=%q", problem.Code)
			}
			if tt.status == 500 && strings.Contains(recorder.Body.String(), tt.err.Error()) {
				t.Fatal("internal details leaked")
			}
		})
	}
}

func TestFinishTripRejectsBody(t *testing.T) {
	id := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", strings.NewReader("{}"))
	recorder := httptest.NewRecorder()
	usecase := NewMocktripFinisher(gomock.NewController(t))

	finish.New(usecase).FinishTrip(recorder, req, id)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
