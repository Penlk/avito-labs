package tripshttp

import (
	"github.com/Penlk/avito-labs/api"
	"github.com/Penlk/avito-labs/internal/trips"
)

func TripToDTo(trip trips.Trip) api.Trip {
	return api.Trip{
		DriverId: trip.DriverId(),

		EndPoint: api.Coordinates{
			Latitude:  new(trip.EndPoint().Latitude()),
			Longitude: new(trip.EndPoint().Longitude()),
		},
		FinishedAt:     trip.FinishedAt(),
		Id:             trip.Id(),
		LastPositionAt: trip.LastPositionAt(),

		Price: new(trip.Price().Rubles()),

		StartPoint: api.Coordinates{
			Latitude:  new(trip.StartPoint().Latitude()),
			Longitude: new(trip.StartPoint().Longitude()),
		},
		StartedAt: trip.StartedAt(),
		Status:    api.TripStatus(trip.State().String()),
		UserId:    trip.UserId(),
	}
}
