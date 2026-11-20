package finish_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	"github.com/Penlk/avito-labs/internal/usecase/trip/finish"
)

const completedStatus = "completed"

func TestFinish(t *testing.T) {
	failed := errors.New("storage failed")
	for _, test := range []struct {
		name                                            string
		completed                                       bool
		readErr, updateErr, historyErr, commitErr, want error
	}{
		{name: "success"},
		{name: completedStatus, completed: true, want: domain.ErrTripCompleted},
		{name: "missing", readErr: domain.ErrTripNotFound, want: domain.ErrTripNotFound},
		{name: "update", updateErr: failed, want: failed},
		{name: "history", historyErr: failed, want: failed},
		{name: "commit", commitErr: failed, want: failed},
		{name: "concurrent finish", updateErr: finish.ErrTripNotUpdated, want: domain.ErrTripCompleted},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			updater, getter := NewMockstateUpdater(ctrl), NewMocktripGetter(ctrl)
			history, tx, events := NewMockstatusHistoryAdder(ctrl), NewMocktxManager(ctrl), NewMockevents(ctrl)
			entity, err := domain.NewTrip(uuid.New(), uuid.New(), vo.Coordinates{}, vo.Price{}, vo.Coordinates{})
			if err != nil {
				t.Fatal(err)
			}
			if test.completed {
				if err := entity.Finish(time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			ctx := t.Context()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			tx.EXPECT().Do(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				if err := fn(txCtx); err != nil {
					return err
				}
				return test.commitErr
			})
			read := getter.EXPECT().GetById(txCtx, entity.Id()).Return(*entity, test.readErr)
			if test.readErr == nil && !test.completed {
				update := updater.EXPECT().UpdateState(txCtx, gomock.Any(), domain.ActiveState{}).DoAndReturn(
					func(_ context.Context, value domain.Trip, _ domain.TripStatusState) (domain.Trip, error) {
						if value.State().String() != completedStatus || value.FinishedAt() == nil {
							t.Fatal("trip not completed before update")
						}
						return value, test.updateErr
					}).After(read)
				if test.updateErr == nil {
					from := "active"
					history.EXPECT().AddStatusHistory(txCtx, entity.Id(), &from, completedStatus).Return(test.historyErr).After(update)
				} else if errors.Is(test.updateErr, finish.ErrTripNotUpdated) {
					completed := *entity
					if err := completed.Finish(time.Now()); err != nil {
						t.Fatal(err)
					}
					getter.EXPECT().GetById(txCtx, entity.Id()).Return(completed, nil).After(update)
				}
			}
			if test.want == nil {
				events.EXPECT().Completed(ctx, entity.Id())
			}
			got, err := finish.New(updater, getter, history, tx, events).Finish(ctx, entity.Id())
			if !errors.Is(err, test.want) {
				t.Fatalf("err=%v want=%v", err, test.want)
			}
			if test.want == nil && (got.Id() != entity.Id() || got.State().String() != "completed") {
				t.Fatalf("trip=%+v", got)
			}
			if test.want != nil && got.Id() != uuid.Nil {
				t.Fatal("returned trip on error")
			}
		})
	}
}
