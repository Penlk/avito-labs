//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip"
)

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type positionAdder interface {
	Add(ctx context.Context, position position.Position) (position.Position, error)
}

type tripRepository interface {
	UpdateLastPositionAt(ctx context.Context, tripID uuid.UUID, recordedAt time.Time) error
	GetById(ctx context.Context, tripID uuid.UUID) (trip.Trip, error)
}

type pendingRequests interface {
	Cancel(tripID uuid.UUID)
}

type events interface {
	PositionSaved(ctx context.Context)
}
