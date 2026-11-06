package create

import (
	"encoding/json"
	"errors"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/domainerror"
)

func toDomain(body api.CreateTripJSONRequestBody) (*domain.Trip, error) {
	if body.StartPoint.Latitude == nil || body.StartPoint.Longitude == nil {
		return nil, domainerror.InvalidRequest.WithDetail("start_point coordinates are required")
	}
	if body.EndPoint.Latitude == nil || body.EndPoint.Longitude == nil {
		return nil, domainerror.InvalidRequest.WithDetail("end_point coordinates are required")
	}
	if body.Price == nil {
		return nil, domainerror.InvalidRequest.WithDetail("price is required")
	}

	startPoint, err := vo.NewCoordinates(*body.StartPoint.Latitude, *body.StartPoint.Longitude)
	if err != nil {
		return nil, domainerror.InvalidRequest.WithDetail("start_point: " + err.Error())
	}
	endPoint, err := vo.NewCoordinates(*body.EndPoint.Latitude, *body.EndPoint.Longitude)
	if err != nil {
		return nil, domainerror.InvalidRequest.WithDetail("end_point: " + err.Error())
	}
	price, err := vo.NewPrice(*body.Price)
	if err != nil {
		return nil, domainerror.InvalidRequest.WithDetail(err.Error())
	}

	trip, err := domain.NewTrip(body.DriverId, body.UserId, endPoint, price, startPoint)
	if err != nil {
		return nil, domainerror.InvalidRequest.WithDetail(err.Error())
	}

	return trip, nil
}

func jsonErrorDetail(err error) string {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return err.Error()
	}

	switch typeErr.Field {
	case "price":
		return "price must be an integer"
	case "start_point.latitude", "start_point.longitude",
		"end_point.latitude", "end_point.longitude":
		return typeErr.Field + " must be a number"
	case "start_point", "end_point":
		return typeErr.Field + " must be an object"
	case "user_id", "driver_id":
		return typeErr.Field + " must be a UUID string"
	case "":
		return "request body must be a JSON object"
	default:
		return "request body contains a value of the wrong type"
	}
}
