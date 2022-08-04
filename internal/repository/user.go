package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/umalmyha/customers/internal/model"
	"github.com/umalmyha/customers/pkg/db/transactor"
)

// UserRepository represents user repository behavior
type UserRepository interface {
	Create(context.Context, *model.User) error
	FindByEmail(context.Context, string) (*model.User, error)
	FindByID(context.Context, string) (*model.User, error)
}

type postgresUserRepository struct {
	transactor.PgxWithinTransactionExecutor
}

// NewPostgresUserRepository builds new postgresUserRepository
func NewPostgresUserRepository(e transactor.PgxWithinTransactionExecutor) UserRepository {
	return &postgresUserRepository{PgxWithinTransactionExecutor: e}
}

func (r *postgresUserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	q := "SELECT id, email, password_hash FROM users WHERE email = $1"
	row := r.Executor(ctx).QueryRow(ctx, q, email)
	return r.scanRow(row)
}

func (r *postgresUserRepository) Create(ctx context.Context, u *model.User) error {
	q := "INSERT INTO users(id, email, password_hash) VALUES($1, $2, $3)"
	if _, err := r.Executor(ctx).Exec(ctx, q, u.ID, u.Email, u.PasswordHash); err != nil {
		return fmt.Errorf("postgres: failed to create user %s - %w", u.ID, err)
	}
	return nil
}

func (r *postgresUserRepository) FindByID(ctx context.Context, id string) (*model.User, error) {
	q := "SELECT id, email, password_hash FROM users WHERE id = $1"
	row := r.Executor(ctx).QueryRow(ctx, q, id)
	return r.scanRow(row)
}

func (r *postgresUserRepository) scanRow(row pgx.Row) (*model.User, error) {
	var u model.User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: failed to scan user - %w", err)
	}
	return &u, nil
}
