package get

import (
	"context"
	"fmt"

	"github.com/oapi-codegen/runtime/types"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
)

type usecase struct {
	tripGetter TripGetter
}

func New(tripGetter TripGetter) *usecase {
	return &usecase{tripGetter: tripGetter}
}

func (u *usecase) Get(ctx context.Context, id types.UUID) (domain.Trip, error) {
	result, err := u.tripGetter.GetById(ctx, id)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("get trip %s: %w", id, err)
	}
	return result, nil
}
