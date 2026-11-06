package trip

import (
	"errors"

	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
)

var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrDriverBusy    = errors.New("driver already has an active trip")
	ErrTripCompleted = errors.New("trip already completed")
)

type ValidationError = vo.ValidationError
