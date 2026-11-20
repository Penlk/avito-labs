package trip

import (
	"fmt"
	"time"

	"github.com/oapi-codegen/runtime/types"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
)

type TripRow struct {
	DriverId types.UUID `db:"driver_id"`

	End_latitude   float64    `db:"end_latitude"`
	End_longitude  float64    `db:"end_longitude"`
	FinishedAt     *time.Time `db:"finished_at"`
	Id             types.UUID `db:"id"`
	LastPositionAt *time.Time `db:"-"`

	Price int64 `db:"price"`

	Start_latitude  float64    `db:"start_latitude"`
	Start_longitude float64    `db:"start_longitude"`
	StartedAt       time.Time  `db:"started_at"`
	Status          string     `db:"status"`
	UserId          types.UUID `db:"user_id"`
}

func (t *TripRow) ToTrip() (domain.Trip, error) {
	var state domain.TripStatusState
	switch t.Status {
	case "active":
		state = domain.ActiveState{}
	case "completed":
		state = domain.CompletedState{}
	default:
		return domain.Trip{}, fmt.Errorf("unknown status: %s", t.Status)
	}

	startPoint, err := vo.NewCoordinates(t.Start_latitude, t.Start_longitude)
	if err != nil {
		return domain.Trip{}, err
	}

	endPoint, err := vo.NewCoordinates(t.End_latitude, t.End_longitude)
	if err != nil {
		return domain.Trip{}, err
	}

	price, err := vo.NewPrice(t.Price)
	if err != nil {
		return domain.Trip{}, err
	}

	return *domain.RestoreTrip(
		t.Id,
		t.DriverId,
		t.UserId,
		endPoint,
		t.FinishedAt,
		t.LastPositionAt,
		price,
		startPoint,
		t.StartedAt,
		state,
	), nil
}
