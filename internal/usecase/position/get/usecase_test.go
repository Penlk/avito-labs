package get_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	"github.com/Penlk/avito-labs/internal/usecase/position/get"
)

func TestGetPositions(t *testing.T) {
	failed := errors.New("storage failed")
	for _, test := range []struct {
		name               string
		empty              bool
		readErr, commitErr error
	}{
		{name: "route"},
		{name: "empty route", empty: true},
		{name: "missing trip", readErr: trip.ErrTripNotFound},
		{name: "read failed", readErr: failed},
		{name: "commit failed", commitErr: failed},
		{name: "canceled", readErr: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			tx, getter := NewMocktxManager(ctrl), NewMockpositionGet(ctrl)
			tripID := uuid.New()
			route := []position.Position{}
			if !test.empty {
				route = append(route, *position.RestorePosition(1, tripID, vo.Coordinates{}, time.Time{}))
			}
			txCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			tx.EXPECT().Do(t.Context(), gomock.Any()).DoAndReturn(
				func(_ context.Context, fn func(context.Context) error) error {
					if err := fn(txCtx); err != nil {
						return err
					}
					return test.commitErr
				})
			getter.EXPECT().GetByTripId(txCtx, tripID).Return(route, test.readErr)
			result, err := get.New(tx, getter).GetByTripId(t.Context(), tripID)
			want := test.readErr
			if want == nil {
				want = test.commitErr
			}
			if !errors.Is(err, want) {
				t.Fatalf("error=%v want=%v", err, want)
			}
			if want != nil && result != nil {
				t.Fatal("returned route after a failed transaction")
			}
			if want == nil && !reflect.DeepEqual(result, route) {
				t.Fatalf("route=%+v want=%+v", result, route)
			}
		})
	}
}
