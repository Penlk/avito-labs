package domainerror

import (
	"errors"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/validation"
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

	var validationErr validation.Error
	if errors.As(err, &validationErr) {
		return InvalidRequest.WithDetail(validationErr.Error())
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
