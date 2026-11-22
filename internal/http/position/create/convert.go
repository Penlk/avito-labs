package create

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	"github.com/Penlk/avito-labs/internal/domain/validation"
	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/domainerror"
)

type requestBody struct {
	Coordinates api.Coordinates `json:"coordinates"`
	RecordedAt  *time.Time      `json:"recorded_at"`
}

func toDomain(body requestBody, tripID uuid.UUID) (*domain.Position, error) {
	if body.Coordinates.Latitude == nil || body.Coordinates.Longitude == nil {
		return nil, domainerror.InvalidRequest.WithDetail("coordinates latitude and longitude are required")
	}
	if body.RecordedAt == nil {
		return nil, domainerror.InvalidRequest.WithDetail("recorded_at is required")
	}

	coordinates, err := vo.NewCoordinates(*body.Coordinates.Latitude, *body.Coordinates.Longitude)
	if err != nil {
		var validationErr validation.Error
		if errors.As(err, &validationErr) {
			validationErr.Field = "coordinates." + validationErr.Field
			return nil, validationErr
		}
		return nil, err
	}

	return domain.NewPosition(tripID, coordinates, *body.RecordedAt)
}

func jsonErrorDetail(err error) string {
	var timeErr *time.ParseError
	if errors.As(err, &timeErr) {
		return "recorded_at must be a valid RFC3339 timestamp"
	}

	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return err.Error()
	}

	switch typeErr.Field {
	case "coordinates.latitude", "coordinates.longitude":
		return typeErr.Field + " must be a number"
	case "coordinates":
		return "coordinates must be an object"
	case "recorded_at":
		return "recorded_at must be a timestamp string"
	case "":
		return "request body must be a JSON object"
	default:
		return "request body contains a value of the wrong type"
	}
}
