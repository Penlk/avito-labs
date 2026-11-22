package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
	"github.com/Penlk/avito-labs/internal/usecase/position/create"
)

func (r *Repository) UpdateLastPositionAt(ctx context.Context, tripID uuid.UUID, recordedAt time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := squirrel.Update("trips").
		Set("last_position_at", squirrel.Expr("GREATEST(last_position_at, ?)", recordedAt)).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": tripID, statusColumn: "active"}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update last position query: %w", err)
	}

	tag, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update trip last position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update trip last position: %w", create.ErrTripNotUpdated)
	}
	return nil
}
