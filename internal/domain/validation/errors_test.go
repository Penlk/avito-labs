package validation_test

import (
	"testing"

	"github.com/Penlk/avito-labs/internal/domain/validation"
)

func TestErrorIdentifiesInvalidField(t *testing.T) {
	err := validation.Error{Field: "trip_id", Reason: "must not be zero"}
	if err.Error() != "trip_id: must not be zero" {
		t.Fatalf("unexpected validation message: %s", err)
	}
}
