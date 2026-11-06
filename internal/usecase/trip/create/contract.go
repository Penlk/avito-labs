//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package create_test
package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/trip"
)

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type tripCreator interface {
	Create(ctx context.Context, trip trip.Trip) (trip.Trip, error)
}

type statusHistoryAdder interface {
	AddStatusHistory(ctx context.Context, id uuid.UUID, from *string, to string) error
}

type events interface {
	Created(ctx context.Context, id uuid.UUID)
}
