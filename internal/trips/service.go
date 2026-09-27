package trips

import (
	"context"
	"errors"

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
	DatabaseError = errors.New("internal error with database")
	TripNotFound  = errors.New("trip not found")
	DriverBusy    = errors.New("driver already has an active trip")
	TripCompleted = errors.New("trip already completed")
)

func (s *serviceImpl) Create(ctx context.Context, trip Trip) (Trip, error) {
	result, err := s.repository.Create(ctx, trip)
	if err != nil {
		return Trip{}, DatabaseError
	}
	return result, nil
}

func (s *serviceImpl) Get(ctx context.Context, id types.UUID) (Trip, error) {
	result, err := s.repository.GetById(ctx, id)
	if err != nil {
		return Trip{}, DatabaseError
	}
	return result, nil
}

func (s *serviceImpl) UpdateState(ctx context.Context, id types.UUID, state TripStatusState) (Trip, error) {
	trip, err := s.Get(ctx, id)
	if err != nil {
		return Trip{}, err
	}
	
	if !trip.TryComplete() {
		return Trip{}, TripCompleted
	}

	result, err := s.repository.Update(ctx, trip)
	if err != nil {
		return Trip{}, DatabaseError
	}
	return result, nil
}

func NewService(repository Repository) Service {
	return &serviceImpl{repository: repository}
}
