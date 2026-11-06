package trip

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const tripColumns = `id, user_id, driver_id,
	start_latitude, start_longitude, end_latitude, end_longitude,
	price, status, started_at, finished_at`

type Repository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}
