package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
)

type usecase struct {
	tripGetter tripGetter
}

func New(tripGetter tripGetter) *usecase {
	return &usecase{tripGetter: tripGetter}
}

func (u *usecase) Get(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	result, err := u.tripGetter.GetById(ctx, id)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("get trip %s: %w", id, err)
	}
	return result, nil
}
