-- +goose NO TRANSACTION

-- +goose Up
CREATE UNIQUE INDEX CONCURRENTLY trips_one_active_driver_idx
    ON trips (driver_id) WHERE status = 'active';

-- +goose Down
DROP INDEX CONCURRENTLY trips_one_active_driver_idx;