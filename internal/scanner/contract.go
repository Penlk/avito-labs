//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package scanner

import (
	"context"
	"time"

	"github.com/Penlk/avito-labs/internal/domain/trip"
)

type PushingCandidateSelector interface {
	Select(ctx context.Context, staleAfter time.Duration) ([]trip.Trip, error)
}

type Ticker interface {
	Ticks() <-chan time.Time
	Stop()
}
