package trips

import (
	"errors"
	"fmt"
	"time"

	"github.com/Penlk/avito-labs/internal/trips/vo"
	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"
)

type TripStatusCore struct {
	state TripStatusState
}

func (t *TripStatusCore) TryComplete() bool {
	return t.state.TryComplete(t)
}

func (t *TripStatusCore) State() TripStatusState {
	return t.state
}

func (t *TripStatusCore) updateState(state TripStatusState) {
	t.state = state
}

type TripStatusState interface {
	fmt.Stringer
	TryComplete(core *TripStatusCore) bool
}

type ActiveState struct{}

func (a ActiveState) String() string {
	return "active"
}

type CompletedState struct{}

func (c CompletedState) String() string {
	return "completed"
}

func (a ActiveState) TryComplete(core *TripStatusCore) bool {
	core.updateState(CompletedState{})
	return true
}

func (c CompletedState) TryComplete(core *TripStatusCore) bool {
	return false
}

type Trip struct {
	id types.UUID

	driverId types.UUID
	userId   types.UUID

	endPoint       vo.Coordinates
	finishedAt     *time.Time
	lastPositionAt *time.Time

	price vo.Price

	startPoint vo.Coordinates
	startedAt  time.Time
	status     TripStatusCore
}

func NewTrip(driverId types.UUID, userId types.UUID, endPoint vo.Coordinates, price vo.Price, startPoint vo.Coordinates) (*Trip, error) {
	if driverId == uuid.Nil || userId == uuid.Nil {
		return nil, errors.New("uuid mustn't be zero")
	}

	return &Trip{
		id: uuid.New(),

		driverId: driverId,
		userId:   userId,

		endPoint:       endPoint,
		finishedAt:     nil,
		lastPositionAt: nil,

		price: price,

		startPoint: startPoint,
		startedAt:  time.Now(),
		status:     TripStatusCore{state: ActiveState{}},
	}, nil
}

func RestoreTrip(id types.UUID, driverId types.UUID, userId types.UUID, endPoint vo.Coordinates, finishedAt *time.Time, lastPositionAt *time.Time, price vo.Price, startPoint vo.Coordinates, startedAt time.Time, status TripStatusCore) *Trip {
	return &Trip{
		id: id,

		driverId: driverId,
		userId:   userId,

		endPoint:       endPoint,
		finishedAt:     finishedAt,
		lastPositionAt: lastPositionAt,

		price: price,

		startPoint: startPoint,
		startedAt:  startedAt,
		status:     status,
	}
}

func (t *Trip) TryComplete() bool {
	success := t.status.TryComplete()

	if success {
		timeNow := time.Now()
		t.finishedAt = &timeNow
	}

	return success
}

func (t *Trip) State() TripStatusState {
	return t.status.State()
}

func (t *Trip) Id() types.UUID {
	return t.id
}

func (t *Trip) DriverId() types.UUID {
	return t.driverId
}

func (t *Trip) UserId() types.UUID {
	return t.userId
}

func (t *Trip) EndPoint() vo.Coordinates {
	return t.endPoint
}

func (t *Trip) FinishedAt() *time.Time {
	return t.finishedAt
}

func (t *Trip) LastPositionAt() *time.Time {
	return t.lastPositionAt
}

func (t *Trip) Price() vo.Price {
	return t.price
}

func (t *Trip) StartPoint() vo.Coordinates {
	return t.startPoint
}

func (t *Trip) StartedAt() time.Time {
	return t.startedAt
}
