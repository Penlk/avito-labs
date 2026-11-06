package create

import (
	"context"

	"github.com/oapi-codegen/runtime/types"

	"github.com/Penlk/avito-labs/internal/domain/trip"
)

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type TripCreator interface {
	Create(ctx context.Context, trip trip.Trip) (trip.Trip, error)
}

type StatusHistoryAdder interface {
	AddStatusHistory(ctx context.Context, id types.UUID, from *string, to string) error
}
