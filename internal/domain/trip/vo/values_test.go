package vo_test

import (
	"errors"
	"math"
	"testing"

	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
)

const (
	latitudeField  = "latitude"
	longitudeField = "longitude"
)

func TestCoordinates(t *testing.T) {
	for _, test := range []struct {
		name     string
		lat, lon float64
		field    string
	}{
		{"zero", 0, 0, ""},
		{"minimum", -90, -180, ""},
		{"maximum", 90, 180, ""},
		{"latitude low", -90.1, 0, latitudeField},
		{"latitude high", 90.1, 0, latitudeField},
		{"longitude low", 0, -180.1, longitudeField},
		{"longitude high", 0, 180.1, longitudeField},
		{"NaN latitude", math.NaN(), 0, latitudeField},
		{"NaN longitude", 0, math.NaN(), longitudeField},
		{"infinity", math.Inf(1), 0, latitudeField},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := vo.NewCoordinates(test.lat, test.lon)
			if test.field != "" {
				var validation vo.ValidationError
				if !errors.As(err, &validation) || validation.Field != test.field || validation.Error() == "" {
					t.Fatalf("error=%v", err)
				}
			} else if err != nil || got.Latitude() != test.lat || got.Longitude() != test.lon {
				t.Fatalf("coordinates=%+v err=%v", got, err)
			}
		})
	}
}

func TestPrice(t *testing.T) {
	for _, price := range []int64{-1, 0, 1000, math.MaxInt64} {
		got, err := vo.NewPrice(price)
		if price < 0 {
			var validation vo.ValidationError
			if !errors.As(err, &validation) || validation.Field != "price" {
				t.Fatalf("error=%v", err)
			}
		} else if err != nil || got.Rubles() != price {
			t.Fatalf("price=%v error=%v", got, err)
		}
	}
}
