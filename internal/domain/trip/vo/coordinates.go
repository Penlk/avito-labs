package vo

import "math"

type Coordinates struct {
	latitude  float64
	longitude float64
}

func NewCoordinates(latitude, longitude float64) (Coordinates, error) {
	if math.IsNaN(latitude) || latitude < -90 || latitude > 90 {
		return Coordinates{}, ValidationError{Field: "latitude", Reason: "must be between -90 and 90"}
	}
	if math.IsNaN(longitude) || longitude < -180 || longitude > 180 {
		return Coordinates{}, ValidationError{Field: "longitude", Reason: "must be between -180 and 180"}
	}

	return Coordinates{latitude: latitude, longitude: longitude}, nil
}

func (c Coordinates) Latitude() float64 {
	return c.latitude
}

func (c Coordinates) Longitude() float64 {
	return c.longitude
}
