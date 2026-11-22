package position

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
)

type PositionRow struct {
	Id         int64     `db:"id"`
	TripId     uuid.UUID `db:"trip_id"`
	Latitude   float64   `db:"latitude"`
	Longitude  float64   `db:"longitude"`
	RecordedAt time.Time `db:"recorded_at"`
}

func (p *PositionRow) ToPosition() (domain.Position, error) {
	coordinates, err := vo.NewCoordinates(p.Latitude, p.Longitude)
	if err != nil {
		return domain.Position{}, err
	}
	return *domain.RestorePosition(p.Id, p.TripId, coordinates, p.RecordedAt), nil
}
