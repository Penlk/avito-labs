package trips

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oapi-codegen/runtime/types"
)

type Service interface {
	Create(ctx context.Context, trip Trip) (Trip, error)
	Get(ctx context.Context, id types.UUID) (Trip, error)
	UpdateState(ctx context.Context, id types.UUID, state TripStatusState) (Trip, error)
}

type serviceImpl struct {
	repository Repository
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

	result, err := s.repository.Create(ctx, trip)
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

func (s *serviceImpl) UpdateState(ctx context.Context, id types.UUID, state TripStatusState) (Trip, error) {
	trip, err := s.Get(ctx, id)
	if err != nil {
		return Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	if !trip.TryComplete() {
		return Trip{}, fmt.Errorf("finish trip %s: %w", id, TripCompleted)
	}

	result, err := s.repository.Update(ctx, trip)
	if errors.Is(err, TripNotUpdated) {
		current, readErr := s.Get(ctx, id)
		if readErr != nil {
			return Trip{}, fmt.Errorf("check trip after update: %w", readErr)
		}
		if current.Status.GetState().String() == "completed" {
			return Trip{}, fmt.Errorf("finish trip %s: %w: %w", id, TripCompleted, err)
		}
	}
	if err != nil {
		return Trip{}, fmt.Errorf("finish trip %s: %w", id, err)
	}
	return result, nil
}

func NewService(repository Repository) Service {
	return &serviceImpl{repository: repository}
}
