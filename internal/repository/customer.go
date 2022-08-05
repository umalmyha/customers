package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/umalmyha/customers/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// CustomerRepository represents behavior for customer repository
type CustomerRepository interface {
	FindByID(context.Context, string) (*model.Customer, error)
	FindAll(context.Context) ([]*model.Customer, error)
	Create(context.Context, *model.Customer) error
	Update(context.Context, *model.Customer) error
	DeleteByID(context.Context, string) error
}

type postgresCustomerRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresCustomerRepository builds postgresCustomerRepository
func NewPostgresCustomerRepository(p *pgxpool.Pool) CustomerRepository {
	return &postgresCustomerRepository{pool: p}
}

func (r *postgresCustomerRepository) FindByID(ctx context.Context, id string) (*model.Customer, error) {
	var c model.Customer
	q := "SELECT id, first_name, last_name, middle_name, email, importance, inactive FROM customers WHERE id = $1"

	row := r.pool.QueryRow(ctx, q, id)
	err := row.Scan(&c.ID, &c.FirstName, &c.LastName, &c.MiddleName, &c.Email, &c.Importance, &c.Inactive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: failed to scan customer %s while reading by id - %w", id, err)
	}
	return &c, nil
}

func (r *postgresCustomerRepository) FindAll(ctx context.Context) ([]*model.Customer, error) {
	customers := make([]*model.Customer, 0)
	q := "SELECT id, first_name, last_name, middle_name, email, importance, inactive FROM customers"

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to read all customers - %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c model.Customer
		if err := rows.Scan(&c.ID, &c.FirstName, &c.LastName, &c.MiddleName, &c.Email, &c.Importance, &c.Inactive); err != nil {
			return nil, fmt.Errorf("postgres: failed to scan customer while reading all customers - %w", err)
		}
		customers = append(customers, &c)
	}

	return customers, nil
}

func (r *postgresCustomerRepository) Create(ctx context.Context, c *model.Customer) error {
	q := `INSERT INTO customers(id, first_name, last_name, middle_name, email, importance, inactive)
					  VALUES($1, $2, $3, $4, $5, $6, $7)`

	_, err := r.pool.Exec(ctx, q, c.ID, c.FirstName, c.LastName, c.MiddleName, c.Email, c.Importance, c.Inactive)
	if err != nil {
		return fmt.Errorf("postgres: failed to insert customer %s while reading by id - %w", c.ID, err)
	}
	return nil
}

func (r *postgresCustomerRepository) Update(ctx context.Context, c *model.Customer) error {
	q := `UPDATE customers SET first_name = $1, last_name = $2, middle_name = $3, email = $4, importance = $5, inactive = $6
          WHERE id = $7`
	_, err := r.pool.Exec(ctx, q, c.FirstName, c.LastName, c.MiddleName, c.Email, c.Importance, c.Inactive, c.ID)
	if err != nil {
		return fmt.Errorf("postgres: failed to update customer %s - %w", c.ID, err)
	}
	return nil
}

func (r *postgresCustomerRepository) DeleteByID(ctx context.Context, id string) error {
	q := "DELETE FROM customers WHERE id = $1"
	_, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("postgres: failed to delete customer %s - %w", id, err)
	}
	return nil
}

type mongoCustomerRepository struct {
	client *mongo.Client
}

// NewMongoCustomerRepository builds new mongoCustomerRepository
func NewMongoCustomerRepository(client *mongo.Client) CustomerRepository {
	return &mongoCustomerRepository{client: client}
}

func (r *mongoCustomerRepository) FindByID(ctx context.Context, id string) (*model.Customer, error) {
	var c model.Customer
	err := r.client.Database("customers").Collection("customers").FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("mongo: failed to read customer %s by id - %w", id, err)
	}
	return &c, nil
}

func (r *mongoCustomerRepository) FindAll(ctx context.Context) ([]*model.Customer, error) {
	cur, err := r.client.Database("customers").Collection("customers").Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("mongo: failed to read all customers - %w", err)
	}

	customers := make([]*model.Customer, 0)
	if err := cur.All(ctx, &customers); err != nil {
		return nil, fmt.Errorf("mongo: failed to scan customers while reading all - %w", err)
	}
	return customers, nil
}

func (r *mongoCustomerRepository) Create(ctx context.Context, c *model.Customer) error {
	_, err := r.client.Database("customers").Collection("customers").InsertOne(ctx, c)
	if err != nil {
		return fmt.Errorf("mongo: failed to create customer %s - %w", c.ID, err)
	}
	return nil
}

func (r *mongoCustomerRepository) Update(ctx context.Context, c *model.Customer) error {
	_, err := r.client.Database("customers").Collection("customers").UpdateByID(ctx, c.ID, bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "firstName", Value: c.FirstName},
			{Key: "lastName", Value: c.LastName},
			{Key: "middleName", Value: c.MiddleName},
			{Key: "email", Value: c.Email},
			{Key: "importance", Value: c.Importance},
			{Key: "inactive", Value: c.Inactive},
		}},
	})
	if err != nil {
		return fmt.Errorf("mongo: failed to update customer %s - %w", c.ID, err)
	}
	return nil
}

func (r *mongoCustomerRepository) DeleteByID(ctx context.Context, id string) error {
	_, err := r.client.Database("customers").Collection("customers").DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("mongo: failed to delete customer %s - %w", id, err)
	}
	return nil
}
