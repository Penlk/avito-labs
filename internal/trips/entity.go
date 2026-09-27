package trips

import (
	"time"

	"github.com/Penlk/avito-labs/internal/trips/vo"
	"github.com/oapi-codegen/runtime/types"
)

type TripStatusCore struct {
	state TripStatusState
}

func (t *TripStatusCore) TryCompleted() bool {
	return t.state.TryCompleted(t)
}

func (t *TripStatusCore) GetState() TripStatusState {
	return t.state
}

func (t *TripStatusCore) updateState(state TripStatusState) {
	t.state = state
}

type TripStatusState interface {
	TryCompleted(core *TripStatusCore) bool
}

type ActiveState struct{}

type CompletedState struct{}

func (a ActiveState) TryCompleted(core *TripStatusCore) bool {
	core.updateState(CompletedState{})
	return true
}

func (c CompletedState) TryCompleted(core *TripStatusCore) bool {
	return false
}

type Trip struct {
	DriverId types.UUID `json:"driver_id"`

	// EndPoint Географические координаты WGS 84.
	EndPoint       vo.Coordinates
	FinishedAt     *time.Time
	Id             types.UUID
	LastPositionAt *time.Time

	// Price Стоимость поездки в целых рублях. Это учебное упрощение: копейки не используются.
	//
	// Example: 1450
	Price int64

	// StartPoint Географические координаты WGS 84.
	StartPoint vo.Coordinates
	StartedAt  time.Time
	Status     TripStatusCore
	UserId     types.UUID
} 