package domainerror

import (
	"net/http"

	api "github.com/Penlk/avito-labs/internal/generated"
)

type RfcError struct {
	api.Problem
}

var (
	InvalidRequest = RfcError{
		Problem: api.Problem{
			Type:   "https://tripgo.example/problems/invalid-request",
			Title:  "Invalid request",
			Status: http.StatusBadRequest,
			Code:   "invalid_request",
		},
	}
	TripNotFound = RfcError{
		Problem: api.Problem{
			Type:   "https://tripgo.example/problems/trip-not-found",
			Title:  "Trip not found",
			Status: http.StatusNotFound,
			Code:   "trip_not_found",
		},
	}
	TripCompleted = RfcError{
		Problem: api.Problem{
			Type:   "https://tripgo.example/problems/trip-completed",
			Title:  "Trip completed",
			Status: http.StatusConflict,
			Code:   "trip_completed",
		},
	}
	DriverBusy = RfcError{
		Problem: api.Problem{
			Type:   "https://tripgo.example/problems/driver-busy",
			Title:  "Driver busy",
			Status: http.StatusConflict,
			Code:   "driver_busy",
		},
	}
	IdempotencyConflict = RfcError{
		Problem: api.Problem{
			Type:   "https://tripgo.example/problems/idempotency-conflict",
			Title:  "Idempotency conflict",
			Status: http.StatusConflict,
			Code:   "idempotency_conflict",
		},
	}
	InternalError = RfcError{
		Problem: api.Problem{
			Type:   "https://tripgo.example/problems/internal-error",
			Title:  "Internal Server Error",
			Status: http.StatusInternalServerError,
			Code:   "internal_error",
		},
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

var _ error = RfcError{}
