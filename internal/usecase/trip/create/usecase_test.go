package create_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	"github.com/Penlk/avito-labs/internal/usecase/trip/create"
)

func TestCreate(t *testing.T) {
	failed := errors.New("storage failed")
	for _, test := range []struct {
		name                       string
		repoErr, historyErr, txErr error
	}{
		{name: "success"},
		{name: "busy", repoErr: domain.ErrDriverBusy},
		{name: "repository", repoErr: failed},
		{name: "history", historyErr: failed},
		{name: "commit", txErr: failed},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo, history := NewMocktripCreator(ctrl), NewMockstatusHistoryAdder(ctrl)
			tx, events := NewMocktxManager(ctrl), NewMockevents(ctrl)
			entity, err := domain.NewTrip(uuid.New(), uuid.New(), vo.Coordinates{}, vo.Price{}, vo.Coordinates{})
			if err != nil {
				t.Fatal(err)
			}
			ctx := t.Context()
			txCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			tx.EXPECT().Do(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, fn func(context.Context) error) error {
				if err := fn(txCtx); err != nil {
					return err
				}
				return test.txErr
			})
			insert := repo.EXPECT().Create(txCtx, *entity).Return(*entity, test.repoErr)
			if test.repoErr == nil {
				history.EXPECT().AddStatusHistory(txCtx, entity.Id(), nil, "active").Return(test.historyErr).After(insert)
			}
			want := errors.Join(test.repoErr, test.historyErr, test.txErr)
			if want == nil {
				events.EXPECT().Created(ctx, entity.Id())
			}
			got, err := create.New(repo, history, tx, events).Create(ctx, *entity)
			if want == nil {
				if err != nil || got.Id() != entity.Id() {
					t.Fatalf("got=%v err=%v", got, err)
				}
			} else {
				cause := test.repoErr
				if cause == nil {
					cause = test.historyErr
				}
				if cause == nil {
					cause = test.txErr
				}
				if !errors.Is(err, cause) || got.Id() != uuid.Nil {
					t.Fatalf("got=%v err=%v", got, err)
				}
			}
		})
	}
}
