//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/position"
)

type PositionLister interface {
	GetByTripId(ctx context.Context, tripId uuid.UUID) ([]position.Position, error)
}
