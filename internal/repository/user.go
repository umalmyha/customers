package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v4"
	"github.com/umalmyha/customers/internal/model/auth"
	"github.com/umalmyha/customers/pkg/db/transactor"
)

type UserRepository interface {
	Create(context.Context, *auth.User) error
	FindByEmail(context.Context, string) (*auth.User, error)
	FindById(context.Context, string) (*auth.User, error)
}

type postgresUserRepository struct {
	transactor.PgxWithinTransactionExecutor
}

func NewPostgresUserRepository(e transactor.PgxWithinTransactionExecutor) UserRepository {
	return &postgresUserRepository{PgxWithinTransactionExecutor: e}
}

func (r *postgresUserRepository) FindByEmail(ctx context.Context, email string) (*auth.User, error) {
	q := "SELECT * FROM users WHERE email = $1"
	row := r.Executor(ctx).QueryRow(ctx, q, email)
	return r.scanRow(row)
}

func (r *postgresUserRepository) Create(ctx context.Context, u *auth.User) error {
	q := "INSERT INTO users(id, email, password_hash) VALUES($1, $2, $3)"
	if _, err := r.Executor(ctx).Exec(ctx, q, u.Id, u.Email, u.PasswordHash); err != nil {
		return err
	}
	return nil
}

func (r *postgresUserRepository) FindById(ctx context.Context, id string) (*auth.User, error) {
	q := "SELECT * FROM users WHERE id = $1"
	row := r.Executor(ctx).QueryRow(ctx, q, id)
	return r.scanRow(row)
}

func (r *postgresUserRepository) scanRow(row pgx.Row) (*auth.User, error) {
	var u auth.User
	if err := row.Scan(&u.Id, &u.Email, &u.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}
