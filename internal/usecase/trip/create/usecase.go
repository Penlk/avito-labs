package create

import (
	"context"
	"fmt"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
)

type usecase struct {
	tripCreator  TripCreator
	historyAdder StatusHistoryAdder
	txManager    TxManager
}

func (u *usecase) Create(ctx context.Context, trip domain.Trip) (domain.Trip, error) {
	var result domain.Trip
	err := u.txManager.Do(ctx, func(txCtx context.Context) error {
		created, err := u.tripCreator.Create(txCtx, trip)
		if err != nil {
			return err
		}
		err = u.historyAdder.AddStatusHistory(txCtx, created.Id(), nil, trip.State().String())
		if err != nil {
			return err
		}
		result = created
		return nil
	})
	if err != nil {
		return domain.Trip{}, fmt.Errorf("create trip: %w", err)
	}
	return result, nil
}

func NewTripGetter(tripCreator TripCreator, historyAdder StatusHistoryAdder, txManager TxManager) *usecase {
	return &usecase{
		tripCreator:  tripCreator,
		historyAdder: historyAdder,
		txManager:    txManager,
	}
}
