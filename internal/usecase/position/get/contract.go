package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/position"
)

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type positionGet interface {
	GetByTripId(ctx context.Context, tripId uuid.UUID) ([]position.Position, error)
}
