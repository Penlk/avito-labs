package position

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
)

func (r *Repository) GetByTripId(ctx context.Context, tripID uuid.UUID) ([]domain.Position, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := squirrel.Select(positionColumns).
		From("trip_positions").
		Where(squirrel.Eq{"trip_id": tripID}).
		OrderBy("recorded_at ASC", "id ASC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get positions query: %w", err)
	}

	executor := transaction.Executor(ctx, r.pool)
	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select positions: %w", err)
	}
	positionRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[PositionRow])
	if err != nil {
		return nil, fmt.Errorf("read selected positions: %w", err)
	}
	if len(positionRows) == 0 {
		var id uuid.UUID
		err := executor.QueryRow(ctx, "SELECT id FROM trips WHERE id = $1", tripID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("select positions: %w: %w", trip.ErrTripNotFound, err)
		}
		if err != nil {
			return nil, fmt.Errorf("check trip for empty route: %w", err)
		}
	}

	result := make([]domain.Position, len(positionRows))
	for i, row := range positionRows {
		pos, err := row.ToPosition()
		if err != nil {
			return nil, fmt.Errorf("decode selected position: %w", err)
		}
		result[i] = pos
	}
	return result, nil
}
