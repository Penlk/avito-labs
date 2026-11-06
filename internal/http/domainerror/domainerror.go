package domainerror

import (
	"errors"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	api "github.com/Penlk/avito-labs/internal/generated"
)

func Problem(err error) (status int, body api.Problem) {
	problem := mapError(err).Problem
	return int(problem.Status), problem
}

func mapError(err error) RfcError {
	var problem RfcError
	if errors.As(err, &problem) {
		return problem
	}

	switch {
	case errors.Is(err, domain.ErrTripNotFound):
		return TripNotFound
	case errors.Is(err, domain.ErrTripCompleted):
		return TripCompleted
	case errors.Is(err, domain.ErrDriverBusy):
		return DriverBusy
	default:
		return InternalError
	}
}
