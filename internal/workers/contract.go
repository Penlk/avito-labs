//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package workers

import (
	"context"

	"github.com/Penlk/avito-labs/internal/gateway/push"
)

type Pusher interface {
	Push(ctx context.Context, pushRequest push.Request) (push.Response, error)
}
