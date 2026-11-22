package create

import (
	"context"
	"errors"
	"fmt"

	"github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip"
)

type usecase struct {
	txManager       txManager
	positionAdder   positionAdder
	tripRepository  tripRepository
	pendingRequests pendingRequests
	events          events
}

func (u *usecase) Create(ctx context.Context, pos position.Position) (position.Position, error) {
	var result position.Position

	err := u.txManager.Do(ctx, func(txCtx context.Context) error {
		err := u.tripRepository.UpdateLastPositionAt(txCtx, pos.TripId(), pos.RecordedAt())
		if errors.Is(err, ErrTripNotUpdated) {
			current, readErr := u.tripRepository.GetById(txCtx, pos.TripId())
			if readErr != nil {
				return fmt.Errorf("check trip after last position update: %w", readErr)
			}
			if current.State() == (trip.CompletedState{}) {
				return fmt.Errorf("%w: %w", trip.ErrTripCompleted, err)
			}
		}
		if err != nil {
			return err
		}
		result, err = u.positionAdder.Add(txCtx, pos)
		return err
	})
	if err != nil {
		return position.Position{}, fmt.Errorf("create position for trip %s: %w", pos.TripId(), err)
	}

	u.pendingRequests.Cancel(pos.TripId())
	u.events.PositionSaved(ctx)
	return result, nil
}

func New(
	txManager txManager, positionAdder positionAdder, tripRepository tripRepository, pendingRequests pendingRequests, events events,
) *usecase {
	return &usecase{
		txManager:       txManager,
		positionAdder:   positionAdder,
		tripRepository:  tripRepository,
		pendingRequests: pendingRequests,
		events:          events,
	}
}
