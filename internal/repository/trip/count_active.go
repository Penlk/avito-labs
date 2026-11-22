package trip

import (
	"context"
	"fmt"

	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
)

func (r *Repository) CountActive(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	var count int64
	err := transaction.Executor(ctx, r.pool).QueryRow(ctx,
		"SELECT count(*) FROM trips WHERE status = $1", "active").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return count, nil
}
