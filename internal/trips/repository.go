package trips

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Create(ctx context.Context, trip Trip) error
	GetById(ctx context.Context, id uint) (Trip, error)
	Update(ctx context.Context, trip Trip) error
}

type repositoryImpl struct {
	pool *pgxpool.Pool
}

func (r *repositoryImpl) Create(ctx context.Context, trip Trip) {
	poll := r.pool
	tx, err := ctx.Value("tx").(pgx.Tx)
	if err == nil {
		poll = tx
	}
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repositoryImpl{pool: pool}
}