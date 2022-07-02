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

type PgxQueryExecutor interface {
	pgxtype.Querier
	SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
	CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error)
}

type PgxTransactor interface {
	Transactor
	Executor(ctx context.Context) PgxQueryExecutor
	WithinTransactionWithOptions(context.Context, func(context.Context) error, pgx.TxOptions) error
}

type pgxTransactor struct {
	pool *pgxpool.Pool
}

func NewPgxTransactor(p *pgxpool.Pool) PgxTransactor {
	return &pgxTransactor{pool: p}
}

func (t *pgxTransactor) Executor(ctx context.Context) PgxQueryExecutor {
	tx := pgxTxValue(ctx)
	if tx != nil {
		return tx
	}
	return t.pool
}

func (t *pgxTransactor) WithinTransaction(ctx context.Context, txFunc func(context.Context) error) error {
	return t.WithinTransactionWithOptions(ctx, txFunc, pgx.TxOptions{})
}

func (t *pgxTransactor) WithinTransactionWithOptions(ctx context.Context, txFunc func(context.Context) error, opts pgx.TxOptions) error {
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
		tx.Rollback(ctx)
	}()

	err = txFunc(withPgTx(ctx, tx))
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}
