package position

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const positionColumns = "id, trip_id, latitude, longitude, recorded_at"

type Repository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}
