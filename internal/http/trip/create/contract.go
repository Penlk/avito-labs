//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
)

type tripCreator interface {
	Create(ctx context.Context, trip domain.Trip) (domain.Trip, error)
}
