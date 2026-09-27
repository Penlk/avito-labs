package trips

import (
	"fmt"
	"time"

	"github.com/Penlk/avito-labs/internal/trips/vo"
	"github.com/oapi-codegen/runtime/types"
)

type TripStatusCore struct {
	state TripStatusState
}

func (t *TripStatusCore) TryComplete() bool {
	return t.state.TryCompleted(t)
}

func (t *TripStatusCore) GetState() TripStatusState {
	return t.state
}

func (t *TripStatusCore) updateState(state TripStatusState) {
	t.state = state
}

type TripStatusState interface {
	fmt.Stringer
	TryCompleted(core *TripStatusCore) bool
}

type ActiveState struct{}

func (a ActiveState) String() string {
	return "active"
}

type CompletedState struct{}

func (c CompletedState) String() string {
	return "completed"
}

func (a ActiveState) TryCompleted(core *TripStatusCore) bool {
	core.updateState(CompletedState{})
	return true
}

func (c CompletedState) TryCompleted(core *TripStatusCore) bool {
	return false
}

type Trip struct {
	DriverId types.UUID

	EndPoint       vo.Coordinates
	FinishedAt     *time.Time
	Id             types.UUID
	LastPositionAt *time.Time

	Price int64

	StartPoint vo.Coordinates
	StartedAt  time.Time
	Status     TripStatusCore
	UserId     types.UUID
}

func (t *Trip) TryComplete() bool {
	success := t.Status.TryComplete()

	if success {
		timeNow := time.Now()
		t.FinishedAt = &timeNow
	}

	return success
}
