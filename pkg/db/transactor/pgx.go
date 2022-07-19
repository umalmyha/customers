package transactor

import (
	"context"
	"github.com/jackc/pgtype/pgxtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type pgxTxKey struct{}

func withPgTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, pgxTxKey{}, tx)
}

func pgxTxValue(ctx context.Context) pgx.Tx {
	if tx, ok := ctx.Value(pgxTxKey{}).(pgx.Tx); ok {
		return tx
	}
	return nil
}

type PgxTransactor interface {
	Transactor
	WithinTransactionWithOptions(context.Context, func(context.Context) error, pgx.TxOptions) error
}

type pgxTransactor struct {
	pool *pgxpool.Pool
}

func NewPgxTransactor(p *pgxpool.Pool) PgxTransactor {
	return &pgxTransactor{pool: p}
}

func (t *pgxTransactor) WithinTransaction(ctx context.Context, txFunc func(context.Context) error) error {
	return t.WithinTransactionWithOptions(ctx, txFunc, pgx.TxOptions{})
}

func (t *pgxTransactor) WithinTransactionWithOptions(ctx context.Context, txFunc func(context.Context) error, opts pgx.TxOptions) (err error) {
	conn, err := t.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	tx, err := conn.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer func() {
		var txErr error
		if err != nil {
			txErr = tx.Rollback(ctx)
		} else {
			txErr = tx.Commit(ctx)
		}

		if txErr != nil {
			err = txErr
		}
	}()

	err = txFunc(withPgTx(ctx, tx))
	return err
}

type PgxWithinTransactionExecutor interface {
	Executor(ctx context.Context) PgxQueryExecutor
}

type PgxQueryExecutor interface {
	pgxtype.Querier
	Begin(context.Context) (pgx.Tx, error)
	SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
	CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error)
}

type pgxWithinTransactionExecutor struct {
	pool *pgxpool.Pool
}

func NewPgxWithinTransactionExecutor(p *pgxpool.Pool) PgxWithinTransactionExecutor {
	return &pgxWithinTransactionExecutor{pool: p}
}

func (e *pgxWithinTransactionExecutor) Executor(ctx context.Context) PgxQueryExecutor {
	tx := pgxTxValue(ctx)
	if tx != nil {
		return tx
	}
	return e.pool
}
