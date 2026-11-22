//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package get_test
package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/trip"
)

type tripGetter interface {
	GetById(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}
