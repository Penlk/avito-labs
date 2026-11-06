//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package finish_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/trip"
)

type stateUpdater interface {
	UpdateState(ctx context.Context, trip trip.Trip, from trip.TripStatusState) (trip.Trip, error)
}

type tripGetter interface {
	GetById(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}

type statusHistoryAdder interface {
	AddStatusHistory(ctx context.Context, id uuid.UUID, from *string, to string) error
}

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type events interface {
	Completed(ctx context.Context, id uuid.UUID)
}
