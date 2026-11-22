package trip

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/oapi-codegen/runtime/types"

	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
)

func (r *Repository) AddStatusHistory(
	ctx context.Context,
	id types.UUID,
	from *string,
	to string,
) error {
	query, args, err := squirrel.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status").
		Values(id, from, to).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip history query: %w", err)
	}

	_, err = transaction.Executor(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("insert trip %s history: %w", id, err)
	}
	return nil
}
