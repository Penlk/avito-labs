package trips

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Penlk/avito-labs/internal/postgres"
	"github.com/oapi-codegen/runtime/types"
)

type Service interface {
	Create(ctx context.Context, trip Trip) (Trip, error)
	Get(ctx context.Context, id types.UUID) (Trip, error)
	UpdateState(ctx context.Context, id types.UUID, state TripStatusState) (Trip, error)
}

type serviceImpl struct {
	repository Repository
	txManager  postgres.TxManager
}

var (
	TripNotFound   = errors.New("trip not found")
	TripNotUpdated = errors.New("trip not updated")
	DriverBusy     = errors.New("driver already has an active trip")
	TripCompleted  = errors.New("trip already completed")
)

func (s *serviceImpl) Create(ctx context.Context, trip Trip) (Trip, error) {
	trip.Status = TripStatusCore{state: ActiveState{}}
	trip.StartedAt = time.Now()
	trip.FinishedAt = nil
	trip.LastPositionAt = nil

	var result Trip
	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		created, err := s.repository.Create(txCtx, trip)
		if err != nil {
			return err
		}
		err = s.repository.AddStatusHistory(txCtx, created.Id, nil, "active")
		if err != nil {
			return err
		}
		result = created
		return nil
	})
	if err != nil {
		return Trip{}, fmt.Errorf("create trip: %w", err)
	}
	return result, nil
}

func (s *serviceImpl) Get(ctx context.Context, id types.UUID) (Trip, error) {
	result, err := s.repository.GetById(ctx, id)
	if err != nil {
		return Trip{}, fmt.Errorf("get trip %s: %w", id, err)
	}
	return result, nil
}

func (s *serviceImpl) UpdateState(
	ctx context.Context,
	id types.UUID,
	state TripStatusState,
) (Trip, error) {
	var result Trip
	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		trip, err := s.Get(txCtx, id)
		if err != nil {
			return err
		}
		from := trip.Status.GetState().String()
		if !trip.TryComplete() {
			return TripCompleted
		}

		updated, err := s.repository.UpdateState(txCtx, trip)
		if errors.Is(err, TripNotUpdated) {
			current, readErr := s.Get(txCtx, id)
			if readErr != nil {
				return fmt.Errorf("check trip after update: %w", readErr)
			}
			if current.Status.GetState().String() == "completed" {
				return fmt.Errorf("%w: %w", TripCompleted, err)
			}
		}
		if err != nil {
			return err
		}

		to := updated.Status.GetState().String()
		err = s.repository.AddStatusHistory(txCtx, updated.Id, &from, to)
		if err != nil {
			return err
		}
		result = updated
		return nil
	})
	if err != nil {
		return Trip{}, fmt.Errorf("finish trip %s: %w", id, err)
	}
	return result, nil
}

func NewService(repository Repository, txManager postgres.TxManager) Service {
	return &serviceImpl{repository: repository, txManager: txManager}
}
