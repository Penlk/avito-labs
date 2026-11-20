package get_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	"github.com/Penlk/avito-labs/internal/usecase/trip/get"
)

func TestGet(t *testing.T) {
	for _, want := range []error{nil, domain.ErrTripNotFound, errors.New("storage failed")} {
		repo := NewMocktripGetter(gomock.NewController(t))
		entity, err := domain.NewTrip(uuid.New(), uuid.New(), vo.Coordinates{}, vo.Price{}, vo.Coordinates{})
		if err != nil {
			t.Fatal(err)
		}
		repo.EXPECT().GetById(t.Context(), entity.Id()).Return(*entity, want)
		got, err := get.New(repo).Get(t.Context(), entity.Id())
		if !errors.Is(err, want) {
			t.Fatalf("err=%v want=%v", err, want)
		}
		if want == nil && got.Id() != entity.Id() {
			t.Fatal("wrong trip")
		}
		if want != nil && got.Id() != uuid.Nil {
			t.Fatal("returned trip on error")
		}
	}
}
