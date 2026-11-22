package position

import (
	"github.com/Penlk/avito-labs/internal/domain/position"
	api "github.com/Penlk/avito-labs/internal/generated"
)

func ToDTO(position position.Position) api.TripPosition {
	return api.TripPosition{
		Coordinates: api.Coordinates{
			Longitude: new(position.Coordinates().Longitude()),
			Latitude:  new(position.Coordinates().Latitude()),
		},
		Id: new(position.Id()),

		RecordedAt: position.RecordedAt(),
		TripId:     position.TripId(),
	}
}
