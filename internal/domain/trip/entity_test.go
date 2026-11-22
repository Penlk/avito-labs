package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
)

func TestNewTrip(t *testing.T) {
	for _, test := range []struct {
		name         string
		driver, user uuid.UUID
		field        string
	}{
		{"valid", uuid.New(), uuid.New(), ""},
		{"zero driver", uuid.Nil, uuid.New(), "driver_id"},
		{"zero user", uuid.New(), uuid.Nil, "user_id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := trip.NewTrip(test.driver, test.user, vo.Coordinates{}, vo.Price{}, vo.Coordinates{})
			if test.field != "" {
				var validation trip.ValidationError
				if !errors.As(err, &validation) || validation.Field != test.field {
					t.Fatalf("validation=%v, want field %s", err, test.field)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Id() == uuid.Nil || got.DriverId() != test.driver || got.UserId() != test.user ||
				got.State().String() != "active" || got.FinishedAt() != nil || got.StartedAt().IsZero() {
				t.Fatalf("invalid new trip: %+v", got)
			}
		})
	}
}

func TestFinish(t *testing.T) {
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name  string
		state trip.TripStatusState
		want  error
	}{
		{"active", trip.ActiveState{}, nil},
		{"completed", trip.CompletedState{}, trip.ErrTripCompleted},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := at.Add(-time.Hour)
			var finished *time.Time
			if test.want != nil {
				finished = &original
			}
			entity := trip.RestoreTrip(uuid.New(), uuid.New(), uuid.New(), vo.Coordinates{},
				finished, nil, vo.Price{}, vo.Coordinates{}, original, test.state)
			err := entity.Finish(at)
			if !errors.Is(err, test.want) {
				t.Fatalf("finish=%v", err)
			}
			wantAt := at
			if test.want != nil {
				wantAt = original
			}
			if entity.State().String() != "completed" || !entity.FinishedAt().Equal(wantAt) {
				t.Fatalf("finish changed wrong state/time: %+v", entity)
			}
			if err := entity.Finish(at.Add(time.Hour)); !errors.Is(err, trip.ErrTripCompleted) {
				t.Fatalf("repeat finish=%v", err)
			}
			if !entity.FinishedAt().Equal(wantAt) {
				t.Fatal("repeat finish changed timestamp")
			}
		})
	}
}
