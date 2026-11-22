package create

import (
	"context"
	"fmt"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
)

type usecase struct {
	events       events
	tripCreator  tripCreator
	historyAdder statusHistoryAdder
	txManager    txManager
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
	u.events.Created(ctx, result.Id())
	return result, nil
}

func New(tripCreator tripCreator, historyAdder statusHistoryAdder, txManager txManager, events events) *usecase {
	return &usecase{
		events:       events,
		tripCreator:  tripCreator,
		historyAdder: historyAdder,
		txManager:    txManager,
	}
}
