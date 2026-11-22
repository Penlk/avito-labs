package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
)

func (r *Repository) Select(ctx context.Context, staleAfter time.Duration) ([]domain.Trip, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := squirrel.Select(tripColumns).
		From("trips").
		Where("status = 'active'").
		Where("COALESCE(last_position_at, started_at) < now() - (? * interval '1 second')", staleAfter.Seconds()).
		OrderBy("COALESCE(last_position_at, started_at) ASC", "id ASC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select stale trips query: %w", err)
	}
	rows, err := transaction.Executor(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select stale trips: %w", err)
	}
	tripRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[TripRow])
	if err != nil {
		return nil, fmt.Errorf("read stale trips: %w", err)
	}
	result := make([]domain.Trip, len(tripRows))
	for i, row := range tripRows {
		entity, err := row.ToTrip()
		if err != nil {
			return nil, fmt.Errorf("decode stale trip: %w", err)
		}
		result[i] = entity
	}
	return result, nil
}
