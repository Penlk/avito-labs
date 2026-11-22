package position_test

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
)

func TestRecordedAtAllowed(t *testing.T) {
	now := time.Date(2026, time.November, 22, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		at   time.Time
		want bool
	}{
		{"past", now.Add(-time.Hour), true},
		{"now", now, true},
		{"future boundary", now.Add(time.Minute), true},
		{"beyond boundary", now.Add(time.Minute + time.Nanosecond), false},
		{"zero date", time.Time{}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := (position.RecordedAtAllowed{Now: now}).IsSatisfiedBy(test.at); got != test.want {
				t.Fatalf("allowed=%v want=%v", got, test.want)
			}
		})
	}
}

func TestNewPosition(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	for _, test := range []struct {
		name, field string
		tripID      uuid.UUID
		offset      time.Duration
	}{
		{name: "valid", tripID: tripID},
		{name: "future boundary", tripID: tripID, offset: time.Minute},
		{name: "missing trip", field: "trip_id"},
		{name: "future rejected", field: "recorded_at", tripID: tripID, offset: time.Minute + time.Nanosecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				coordinates, err := vo.NewCoordinates(55, 37)
				if err != nil {
					t.Fatal(err)
				}
				at := time.Now().Add(test.offset)
				pos, err := position.NewPosition(test.tripID, coordinates, at)
				if test.field != "" {
					var invalid position.ValidationError
					if !errors.As(err, &invalid) || invalid.Field != test.field || pos != nil {
						t.Fatalf("position=%+v error=%v; want validation on %s", pos, err, test.field)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if pos.Id() != 0 || pos.TripId() != tripID || pos.Coordinates() != coordinates || !pos.RecordedAt().Equal(at) {
					t.Fatalf("unexpected position: %+v", pos)
				}
			})
		})
	}
}

func TestRestorePositionKeepsStoredFields(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	at := time.Date(2026, time.November, 22, 12, 0, 0, 0, time.UTC)
	coordinates, err := vo.NewCoordinates(55, 37)
	if err != nil {
		t.Fatal(err)
	}
	pos := position.RestorePosition(42, tripID, coordinates, at)
	if pos.Id() != 42 || pos.TripId() != tripID || pos.Coordinates() != coordinates || !pos.RecordedAt().Equal(at) {
		t.Fatalf("unexpected restored position: %+v", pos)
	}
}
