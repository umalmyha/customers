package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v4"
	"github.com/umalmyha/customers/internal/model/auth"
	"github.com/umalmyha/customers/pkg/db/transactor"
)

type RefreshTokenRepository interface {
	Create(context.Context, *auth.RefreshToken) error
	FindTokensByUserId(context.Context, string) ([]*auth.RefreshToken, error)
	DeleteByUserId(context.Context, string) error
	DeleteById(context.Context, string) error
	FindById(context.Context, string) (*auth.RefreshToken, error)
}

type postgresRefreshTokenRepository struct {
	transactor.PgxWithinTransactionExecutor
}

func NewPostgresRefreshTokenRepository(e transactor.PgxWithinTransactionExecutor) RefreshTokenRepository {
	return &postgresRefreshTokenRepository{PgxWithinTransactionExecutor: e}
}

func (r *postgresRefreshTokenRepository) Create(ctx context.Context, tkn *auth.RefreshToken) error {
	q := "INSERT INTO refresh_tokens(id, user_id, fingerprint, expires_in, created_at) VALUES($1, $2, $3, $4, $5)"
	if _, err := r.Executor(ctx).Exec(ctx, q, tkn.Id, tkn.UserId, tkn.Fingerprint, tkn.ExpiresIn, tkn.CreatedAt); err != nil {
		return err
	}
	return nil
}

func (r *postgresRefreshTokenRepository) FindTokensByUserId(ctx context.Context, userId string) ([]*auth.RefreshToken, error) {
	q := "SELECT * FROM refresh_tokens WHERE user_id = $1"

	rows, err := r.Executor(ctx).Query(ctx, q, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]*auth.RefreshToken, 0)
	for rows.Next() {
		var tkn auth.RefreshToken
		if err := rows.Scan(&tkn.Id, &tkn.UserId, &tkn.Fingerprint, &tkn.ExpiresIn, &tkn.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, &tkn)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tokens, nil
}

func (r *postgresRefreshTokenRepository) DeleteByUserId(ctx context.Context, userId string) error {
	q := "DELETE FROM refresh_tokens WHERE user_id = $1"
	if _, err := r.Executor(ctx).Exec(ctx, q, userId); err != nil {
		return err
	}
	return nil
}

func (r *postgresRefreshTokenRepository) DeleteById(ctx context.Context, id string) error {
	q := "DELETE FROM refresh_tokens WHERE id = $1"
	if _, err := r.Executor(ctx).Exec(ctx, q, id); err != nil {
		return err
	}
	return nil
}

func (r *postgresRefreshTokenRepository) FindById(ctx context.Context, id string) (*auth.RefreshToken, error) {
	q := "SELECT * FROM refresh_tokens WHERE id = $1"
	row := r.Executor(ctx).QueryRow(ctx, q, id)
	return r.scanRow(row)
}

func (r *postgresRefreshTokenRepository) scanRow(row pgx.Row) (*auth.RefreshToken, error) {
	var tkn auth.RefreshToken
	if err := row.Scan(&tkn.Id, &tkn.UserId, &tkn.Fingerprint, &tkn.ExpiresIn, &tkn.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &tkn, nil
}
