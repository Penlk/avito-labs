package position

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Penlk/avito-labs/internal/domain/trip"
)

func classifyPositionWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" && pgErr.ConstraintName == "trip_positions_trip_id_fkey" {
		return fmt.Errorf("%w: %w", trip.ErrTripNotFound, err)
	}
	return err
}
