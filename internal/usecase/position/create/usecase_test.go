package create_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	"github.com/Penlk/avito-labs/internal/usecase/position/create"
)

func TestCreateCancelsPendingRequestAfterCommit(t *testing.T) {
	failed := errors.New("storage failed")
	for _, test := range []struct {
		name                         string
		updateErr, addErr, commitErr error
	}{
		{name: "success"},
		{name: "update failed", updateErr: failed},
		{name: "insert failed", addErr: failed},
		{name: "commit failed", commitErr: failed},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			tx, adder := NewMocktxManager(ctrl), NewMockpositionAdder(ctrl)
			trips, pending := NewMocktripRepository(ctrl), NewMockpendingRequests(ctrl)
			events := NewMockevents(ctrl)
			pos := *position.RestorePosition(0, uuid.New(), vo.Coordinates{}, time.Now())
			stored := *position.RestorePosition(42, pos.TripId(), pos.Coordinates(), pos.RecordedAt())
			txCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			committed := false
			tx.EXPECT().Do(t.Context(), gomock.Any()).DoAndReturn(
				func(_ context.Context, fn func(context.Context) error) error {
					if err := fn(txCtx); err != nil {
						return err
					}
					committed = test.commitErr == nil
					return test.commitErr
				})
			update := trips.EXPECT().UpdateLastPositionAt(txCtx, pos.TripId(), pos.RecordedAt()).Return(test.updateErr)
			if test.updateErr == nil {
				adder.EXPECT().Add(txCtx, pos).Return(stored, test.addErr).After(update)
			}
			want := errors.Join(test.updateErr, test.addErr, test.commitErr)
			if want == nil {
				events.EXPECT().PositionSaved(t.Context()).Do(func(context.Context) {
					if !committed {
						t.Error("position counted before transaction commit")
					}
				})
				pending.EXPECT().Cancel(pos.TripId()).Do(func(uuid.UUID) {
					if !committed {
						t.Error("pending request canceled before transaction commit")
					}
				})
			}
			result, err := create.New(tx, adder, trips, pending, events).Create(t.Context(), pos)
			if want == nil {
				if err != nil || result.Id() != stored.Id() {
					t.Fatalf("position=%+v error=%v", result, err)
				}
			} else if !errors.Is(err, failed) || result != (position.Position{}) {
				t.Fatalf("position=%+v error=%v; want empty result and storage error", result, err)
			}
		})
	}
}

func TestCreateClassifiesTripAfterConditionalUpdate(t *testing.T) {
	failed := errors.New("read failed")
	for _, test := range []struct {
		name      string
		completed bool
		readErr   error
		want      error
	}{
		{name: "missing", readErr: trip.ErrTripNotFound, want: trip.ErrTripNotFound},
		{name: "completed", completed: true, want: trip.ErrTripCompleted},
		{name: "read failed", readErr: failed, want: failed},
		{name: "active but not updated", want: create.ErrTripNotUpdated},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			tx, adder := NewMocktxManager(ctrl), NewMockpositionAdder(ctrl)
			trips, pending, events := NewMocktripRepository(ctrl), NewMockpendingRequests(ctrl), NewMockevents(ctrl)
			entity, err := trip.NewTrip(uuid.New(), uuid.New(), vo.Coordinates{}, vo.Price{}, vo.Coordinates{})
			if err != nil {
				t.Fatal(err)
			}
			if test.completed {
				if err := entity.Finish(time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			pos := *position.RestorePosition(0, entity.Id(), vo.Coordinates{}, time.Now())
			tx.EXPECT().Do(t.Context(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
			gomock.InOrder(
				trips.EXPECT().UpdateLastPositionAt(t.Context(), entity.Id(), pos.RecordedAt()).Return(create.ErrTripNotUpdated),
				trips.EXPECT().GetById(t.Context(), entity.Id()).Return(*entity, test.readErr),
			)
			result, err := create.New(tx, adder, trips, pending, events).Create(t.Context(), pos)
			if !errors.Is(err, test.want) || result != (position.Position{}) {
				t.Fatalf("position=%+v error=%v want=%v", result, err, test.want)
			}
		})
	}
}
