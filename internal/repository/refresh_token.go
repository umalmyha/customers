package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/pkg/db/transactor"
)

// RefreshTokenRepository represents behavior of refresh token repository
type RefreshTokenRepository interface {
	Create(context.Context, *model.RefreshToken) error
	FindTokensByUserID(context.Context, string) ([]*model.RefreshToken, error)
	DeleteByUserID(context.Context, string) error
	DeleteByID(context.Context, string) error
	FindByID(context.Context, string) (*model.RefreshToken, error)
}

type postgresRefreshTokenRepository struct {
	transactor.PgxWithinTransactionExecutor
}

// NewPostgresRefreshTokenRepository builds postgresRefreshTokenRepository
func NewPostgresRefreshTokenRepository(e transactor.PgxWithinTransactionExecutor) RefreshTokenRepository {
	return &postgresRefreshTokenRepository{PgxWithinTransactionExecutor: e}
}

func (r *postgresRefreshTokenRepository) Create(ctx context.Context, tkn *model.RefreshToken) error {
	q := "INSERT INTO refresh_tokens(id, user_id, fingerprint, expires_in, created_at) VALUES($1, $2, $3, $4, $5)"
	if _, err := r.Executor(ctx).Exec(ctx, q, tkn.ID, tkn.UserID, tkn.Fingerprint, tkn.ExpiresIn, tkn.CreatedAt); err != nil {
		return fmt.Errorf("postgres: failed to create refresh token %s - %w", tkn.ID, err)
	}
	return nil
}

func (r *postgresRefreshTokenRepository) FindTokensByUserID(ctx context.Context, userID string) ([]*model.RefreshToken, error) {
	q := "SELECT id, user_id, fingerprint, expires_in, created_at FROM refresh_tokens WHERE user_id = $1"

	rows, err := r.Executor(ctx).Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to read refresh tokens for user id %s - %w", userID, err)
	}
	defer rows.Close()

	tokens := make([]*model.RefreshToken, 0)
	for rows.Next() {
		var tkn model.RefreshToken
		if err := rows.Scan(&tkn.ID, &tkn.UserID, &tkn.Fingerprint, &tkn.ExpiresIn, &tkn.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: failed to scan refresh token while reading for user id %s - %w", userID, err)
		}
		tokens = append(tokens, &tkn)
	}

	return tokens, nil
}

func (r *postgresRefreshTokenRepository) DeleteByUserID(ctx context.Context, userID string) error {
	q := "DELETE FROM refresh_tokens WHERE user_id = $1"
	if _, err := r.Executor(ctx).Exec(ctx, q, userID); err != nil {
		return fmt.Errorf("postgres: failed to delete all tokens for user id %s - %w", userID, err)
	}
	return nil
}

func (r *postgresRefreshTokenRepository) DeleteByID(ctx context.Context, id string) error {
	q := "DELETE FROM refresh_tokens WHERE id = $1"
	if _, err := r.Executor(ctx).Exec(ctx, q, id); err != nil {
		return fmt.Errorf("postgres: failed to delete token by id %s - %w", id, err)
	}
	return nil
}

func (r *postgresRefreshTokenRepository) FindByID(ctx context.Context, id string) (*model.RefreshToken, error) {
	q := "SELECT id, user_id, fingerprint, expires_in, created_at FROM refresh_tokens WHERE id = $1"
	row := r.Executor(ctx).QueryRow(ctx, q, id)
	return r.scanRow(row)
}

func (r *postgresRefreshTokenRepository) scanRow(row pgx.Row) (*model.RefreshToken, error) {
	var tkn model.RefreshToken
	if err := row.Scan(&tkn.ID, &tkn.UserID, &tkn.Fingerprint, &tkn.ExpiresIn, &tkn.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: failed to scan token - %w", err)
	}
	return &tkn, nil
}
