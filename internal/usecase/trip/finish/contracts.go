package finish

import (
	"context"
	"errors"

	"github.com/oapi-codegen/runtime/types"

	"github.com/Penlk/avito-labs/internal/domain/trip"
)

var ErrTripNotUpdated = errors.New("trip not updated")

type StateUpdater interface {
	UpdateState(ctx context.Context, trip trip.Trip, from trip.TripStatusState) (trip.Trip, error)
}

type TripGetter interface {
	GetById(ctx context.Context, id types.UUID) (trip.Trip, error)
}

type StatusHistoryAdder interface {
	AddStatusHistory(ctx context.Context, id types.UUID, from *string, to string) error
}

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}
