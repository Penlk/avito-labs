package trip

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
)

func classifyTripWriteError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch pgErr.ConstraintName {
	case "trips_one_active_driver_idx", "trips_driver_id_key":
		return fmt.Errorf("%w: %w", domain.ErrDriverBusy, err)
	default:
		return err
	}
}
