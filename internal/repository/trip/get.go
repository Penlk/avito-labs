package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/runtime/types"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
)

func (r *Repository) GetById(ctx context.Context, id types.UUID) (domain.Trip, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	executor := transaction.Executor(ctx, r.pool)

	query, args, err := squirrel.Select(tripColumns).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("select trip: %w", err)
	}

	tripRow, err := pgx.CollectExactlyOneRow(
		rows, pgx.RowToStructByName[TripRow],
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, fmt.Errorf("select trip: %w: %w", domain.ErrTripNotFound, err)
	}
	if err != nil {
		return domain.Trip{}, fmt.Errorf("read selected trip: %w", err)
	}

	trip, err := tripRow.ToTrip()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("decode selected trip: %w", err)
	}
	return trip, nil
}
