package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/domain/position"
)

type usecase struct {
	txManager   txManager
	positionGet positionGet
}

func (u *usecase) GetByTripId(ctx context.Context, tripId uuid.UUID) ([]position.Position, error) {
	var result []position.Position

	err := u.txManager.Do(ctx, func(ctx context.Context) error {
		var err error
		result, err = u.positionGet.GetByTripId(ctx, tripId)
		return err
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func New(txManager txManager, positionGet positionGet) *usecase {
	return &usecase{
		txManager:   txManager,
		positionGet: positionGet,
	}
}
