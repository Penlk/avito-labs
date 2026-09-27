package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type txManager struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

type txContextKey struct{}

type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func Executor(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txContextKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}

func (m *txManager) Do(ctx context.Context, fn func(context.Context) error) (err error) {
	if _, ok := ctx.Value(txContextKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		rollbackCtx := context.WithoutCancel(ctx)
		rollbackCtx, cancel := context.WithTimeout(rollbackCtx, m.timeout)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil &&
			!errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	txCtx := context.WithValue(ctx, txContextKey{}, tx)
	if err = fn(txCtx); err != nil {
		return err
	}
	if err = tx.Commit(txCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func NewTxManager(pool *pgxpool.Pool, timeout time.Duration) TxManager {
	return &txManager{pool: pool, timeout: timeout}
}
