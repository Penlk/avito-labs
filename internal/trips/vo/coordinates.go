package vo

import "errors"

type Coordinates struct {
	latitude  float64
	longitude float64
}

func NewCoordinates(latitude float64, longitude float64) (Coordinates, error) {
	if latitude < -90 || latitude > 90 {
		return Coordinates{}, errors.New("latitude must be between -90 and 90")
	}
	if longitude < -180 || longitude > 180 {
		return Coordinates{}, errors.New("longitude must be between -180 and 180")
	}

	return Coordinates{latitude: latitude, longitude: longitude}, nil
}

func (c Coordinates) Latitude() float64 {
	return c.latitude
}

func (c Coordinates) Longitude() float64 {
	return c.longitude
}
