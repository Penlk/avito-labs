//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/Penlk/avito-labs/internal/domain/position"
)

type PositionCreator interface {
	Create(ctx context.Context, pos position.Position) (position.Position, error)
}
