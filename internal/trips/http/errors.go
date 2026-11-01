package tripshttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Penlk/avito-labs/api"
)

type RfcError struct {
	api.Problem
}

var (
	InvalidRequest = RfcError{
		Type:   "https://tripgo.example/problems/invalid-request",
		Title:  "Invalid request",
		Status: http.StatusBadRequest,
		Code:   "invalid_request",
	}
	TripNotFound = RfcError{
		Type:   "https://tripgo.example/problems/trip-not-found",
		Title:  "Trip not found",
		Status: http.StatusNotFound,
		Code:   "trip_not_found",
	}
	TripCompleted = RfcError{
		Type:   "https://tripgo.example/problems/trip-completed",
		Title:  "Trip completed",
		Status: http.StatusConflict,
		Code:   "trip_completed",
	}
	DriverBusy = RfcError{
		Type:   "https://tripgo.example/problems/driver-busy",
		Title:  "Driver busy",
		Status: http.StatusConflict,
		Code:   "driver_busy",
	}
	IdempotencyConflict = RfcError{
		Type:   "https://tripgo.example/problems/idempotency-conflict",
		Title:  "Idempotency conflict",
		Status: http.StatusConflict,
		Code:   "idempotency_conflict",
	}
	InternalError = RfcError{
		Type:   "https://tripgo.example/problems/internal-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Code:   "internal_error",
	}
)

func (r RfcError) Error() string {
	if r.Detail != nil && *r.Detail != "" {
		return r.Title + ": " + *r.Detail
	}
	return r.Title
}

func (r RfcError) WithDetail(detail string) RfcError {
	r.Detail = &detail
	return r
}

func (r RfcError) WithInstance(instance string) RfcError {
	r.Instance = &instance
	return r
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

var _ error = RfcError{}
