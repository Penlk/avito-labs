//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"

	"github.com/oapi-codegen/runtime/types"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
)

type tripFinisher interface {
	Finish(ctx context.Context, id types.UUID) (domain.Trip, error)
}
