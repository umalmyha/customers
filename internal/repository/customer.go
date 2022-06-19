package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/umalmyha/customers/internal/customer"
)

type CustomerRepository interface {
	FindById(context.Context, string) (customer.Customer, error)
	FindAll(context.Context) ([]customer.Customer, error)
	Create(context.Context, customer.Customer) (bool, error)
	Update(context.Context, customer.Customer) (bool, error)
	DeleteById(context.Context, string) (bool, error)
}

type postgresCustomerRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresCustomerRepository(p *pgxpool.Pool) CustomerRepository {
	return &postgresCustomerRepository{pool: p}
}

func (repo *postgresCustomerRepository) FindById(ctx context.Context, id string) (customer.Customer, error) {
	var c customer.Customer
	q := "SELECT * FROM customers WHERE id = $1"

	row := repo.pool.QueryRow(ctx, q, id)
	if err := row.Scan(&c.Id, &c.FirstName, &c.LastName, &c.MiddleName, &c.Email, &c.Importance, &c.Inactive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c, nil
		}
		return c, err
	}
	return c, nil
}

func (repo *postgresCustomerRepository) FindAll(ctx context.Context) ([]customer.Customer, error) {
	customers := make([]customer.Customer, 0)
	q := "SELECT * FROM customers"

	rows, err := repo.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var c customer.Customer
		if err := rows.Scan(&c.Id, &c.FirstName, &c.LastName, &c.MiddleName, &c.Email, &c.Importance, &c.Inactive); err != nil {
			return nil, err
		}
		customers = append(customers, c)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return customers, nil
}

func (repo *postgresCustomerRepository) Create(ctx context.Context, c customer.Customer) (bool, error) {
	q := `INSERT INTO customers(id, first_name, last_name, middle_name, email, importance, inactive)
					  VALUES($1, $2, $3, $4, $5, $6, $7)`
	comm, err := repo.pool.Exec(ctx, q, &c.Id, &c.FirstName, &c.LastName, &c.MiddleName, &c.Email, &c.Importance, &c.Inactive)
	if err != nil {
		return false, err
	}
	return comm.RowsAffected() > 0, nil
}

func (repo *postgresCustomerRepository) Update(ctx context.Context, c customer.Customer) (bool, error) {
	q := `UPDATE customers SET first_name = $1, last_name = $2, middle_name = $3, email = $4, importance = $5, inactive = $6
          WHERE id = $7`
	comm, err := repo.pool.Exec(ctx, q, &c.FirstName, &c.LastName, &c.MiddleName, &c.Email, &c.Importance, &c.Inactive, &c.Id)
	if err != nil {
		return false, err
	}
	return comm.RowsAffected() > 0, nil
}

func (repo *postgresCustomerRepository) DeleteById(ctx context.Context, id string) (bool, error) {
	q := "DELETE FROM customers WHERE id = $1"
	comm, err := repo.pool.Exec(ctx, q, id)
	if err != nil {
		return false, err
	}
	return comm.RowsAffected() > 0, nil
}
