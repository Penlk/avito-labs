package get

import (
	"context"

	"github.com/oapi-codegen/runtime/types"

	"github.com/Penlk/avito-labs/internal/domain/trip"
)

type TripGetter interface {
	GetById(ctx context.Context, id types.UUID) (trip.Trip, error)
}
