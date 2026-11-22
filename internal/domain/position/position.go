package position

import (
	"time"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
)

type Position struct {
	id     int64
	tripId uuid.UUID

	coordinates vo.Coordinates
	recordedAt  time.Time
}

type RecordedAtAllowed struct {
	Now time.Time
}

func (s RecordedAtAllowed) IsSatisfiedBy(recordedAt time.Time) bool {
	return !recordedAt.After(s.Now.Add(time.Minute))
}

func NewPosition(tripId uuid.UUID, coordinates vo.Coordinates, recordedAt time.Time) (*Position, error) {
	if tripId == uuid.Nil {
		return nil, ValidationError{Field: "trip_id", Reason: "must not be zero"}
	}

	if !(RecordedAtAllowed{Now: time.Now()}).IsSatisfiedBy(recordedAt) {
		return nil, ValidationError{Field: "recorded_at", Reason: "must not be more than one minute in the future"}
	}

	return &Position{
		id:     0,
		tripId: tripId,

		coordinates: coordinates,
		recordedAt:  recordedAt,
	}, nil
}

func RestorePosition(id int64, tripId uuid.UUID, coordinates vo.Coordinates, recordedAt time.Time) *Position {
	return &Position{
		id:     id,
		tripId: tripId,

		coordinates: coordinates,
		recordedAt:  recordedAt,
	}
}

func (p *Position) Id() int64 {
	return p.id
}

func (p *Position) TripId() uuid.UUID {
	return p.tripId
}

func (p *Position) Coordinates() vo.Coordinates {
	return p.coordinates
}

func (p *Position) RecordedAt() time.Time {
	return p.recordedAt
}
