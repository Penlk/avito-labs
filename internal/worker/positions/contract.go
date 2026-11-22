//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package positions

import (
	"context"
	"time"

	"github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/gateway/push"
)

type pushingCandidateSelector interface {
	Select(ctx context.Context, staleAfter time.Duration) ([]trip.Trip, error)
}

type ticker interface {
	Ticks() <-chan time.Time
	Stop()
}

type pusher interface {
	Push(ctx context.Context, request push.Request) (push.Response, error)
}
