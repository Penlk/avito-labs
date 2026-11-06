package finish

import (
	"context"
	"errors"
	"fmt"

	"github.com/oapi-codegen/runtime/types"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
)

type usecase struct {
	stateUpdater StateUpdater
	tripGetter   TripGetter
	txManager    TxManager
	historyAdder StatusHistoryAdder
}

func New(
	stateUpdater StateUpdater, tripGetter TripGetter,
	historyAdder StatusHistoryAdder, txManager TxManager,
) *usecase {
	return &usecase{
		stateUpdater: stateUpdater,
		tripGetter:   tripGetter,
		historyAdder: historyAdder,
		txManager:    txManager,
	}
}

func (u *usecase) Finish(
	ctx context.Context,
	id types.UUID,
) (domain.Trip, error) {
	var result domain.Trip
	err := u.txManager.Do(ctx, func(txCtx context.Context) error {
		trip, err := u.tripGetter.GetById(txCtx, id)
		if err != nil {
			return err
		}
		from := trip.State()
		if !trip.TryComplete() {
			return domain.ErrTripCompleted
		}

		updated, err := u.stateUpdater.UpdateState(txCtx, trip, from)
		if errors.Is(err, ErrTripNotUpdated) {
			current, readErr := u.tripGetter.GetById(txCtx, id)
			if readErr != nil {
				return fmt.Errorf("check trip after update: %w", readErr)
			}
			if current.State() == (domain.CompletedState{}) {
				return fmt.Errorf("%w: %w", domain.ErrTripCompleted, err)
			}
		}
		if err != nil {
			return err
		}

		to := updated.State().String()
		fromString := from.String()
		err = u.historyAdder.AddStatusHistory(txCtx, updated.Id(), &fromString, to)
		if err != nil {
			return err
		}
		result = updated
		return nil
	})
	if err != nil {
		return domain.Trip{}, fmt.Errorf("finish trip %s: %w", id, err)
	}
	return result, nil
}
