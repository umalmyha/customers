package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/sirupsen/logrus"
	"github.com/umalmyha/customers/internal/cache"
	"github.com/umalmyha/customers/internal/model/customer"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type CustomerRepository interface {
	FindById(context.Context, string) (*customer.Customer, error)
	FindAll(context.Context) ([]*customer.Customer, error)
	Create(context.Context, *customer.Customer) error
	Update(context.Context, *customer.Customer) error
	DeleteById(context.Context, string) error
}

type postgresCustomerRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresCustomerRepository(p *pgxpool.Pool) CustomerRepository {
	return &postgresCustomerRepository{pool: p}
}

func (r *postgresCustomerRepository) FindById(ctx context.Context, id string) (*customer.Customer, error) {
	var c customer.Customer
	q := "SELECT * FROM customers WHERE id = $1"

	row := r.pool.QueryRow(ctx, q, id)
	if err := row.Scan(&c.Id, &c.FirstName, &c.LastName, &c.MiddleName, &c.Email, &c.Importance, &c.Inactive); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *postgresCustomerRepository) FindAll(ctx context.Context) ([]*customer.Customer, error) {
	customers := make([]*customer.Customer, 0)
	q := "SELECT * FROM customers"

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var c customer.Customer
		if err := rows.Scan(&c.Id, &c.FirstName, &c.LastName, &c.MiddleName, &c.Email, &c.Importance, &c.Inactive); err != nil {
			return nil, err
		}
		customers = append(customers, &c)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return customers, nil
}

func (r *postgresCustomerRepository) Create(ctx context.Context, c *customer.Customer) error {
	q := `INSERT INTO customers(id, first_name, last_name, middle_name, email, importance, inactive)
					  VALUES($1, $2, $3, $4, $5, $6, $7)`

	_, err := r.pool.Exec(ctx, q, c.Id, c.FirstName, c.LastName, c.MiddleName, c.Email, c.Importance, c.Inactive)
	if err != nil {
		return err
	}
	return nil
}

func (r *postgresCustomerRepository) Update(ctx context.Context, c *customer.Customer) error {
	q := `UPDATE customers SET first_name = $1, last_name = $2, middle_name = $3, email = $4, importance = $5, inactive = $6
          WHERE id = $7`
	_, err := r.pool.Exec(ctx, q, c.FirstName, c.LastName, c.MiddleName, c.Email, c.Importance, c.Inactive, c.Id)
	if err != nil {
		return err
	}
	return nil
}

func (r *postgresCustomerRepository) DeleteById(ctx context.Context, id string) error {
	q := "DELETE FROM customers WHERE id = $1"
	_, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return err
	}
	return nil
}

type mongoCustomerRepository struct {
	client *mongo.Client
}

func NewMongoCustomerRepository(client *mongo.Client) CustomerRepository {
	return &mongoCustomerRepository{client: client}
}

func (r *mongoCustomerRepository) FindById(ctx context.Context, id string) (*customer.Customer, error) {
	var c customer.Customer
	if err := r.client.Database("customers").Collection("customers").FindOne(ctx, bson.M{"_id": id}).Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *mongoCustomerRepository) FindAll(ctx context.Context) ([]*customer.Customer, error) {
	cur, err := r.client.Database("customers").Collection("customers").Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	customers := make([]*customer.Customer, 0)
	if err := cur.All(ctx, &customers); err != nil {
		return nil, err
	}
	return customers, nil
}

func (r *mongoCustomerRepository) Create(ctx context.Context, c *customer.Customer) error {
	_, err := r.client.Database("customers").Collection("customers").InsertOne(ctx, c)
	if err != nil {
		return err
	}
	return nil
}

func (r *mongoCustomerRepository) Update(ctx context.Context, c *customer.Customer) error {
	_, err := r.client.Database("customers").Collection("customers").UpdateByID(ctx, c.Id, bson.D{
		{"$set", bson.D{
			{"firstName", c.FirstName},
			{"lastName", c.LastName},
			{"middleName", c.MiddleName},
			{"email", c.Email},
			{"importance", c.Importance},
			{"inactive", c.Inactive},
		}},
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *mongoCustomerRepository) DeleteById(ctx context.Context, id string) error {
	if _, err := r.client.Database("customers").Collection("customers").DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return err
	}
	return nil
}

type redisCachedCustomerRepository struct {
	logger logrus.FieldLogger
	cache  cache.CustomerCache
	CustomerRepository
}

func NewRedisCachedCustomerRepository(logger logrus.FieldLogger, cache cache.CustomerCache, primaryRps CustomerRepository) CustomerRepository {
	return &redisCachedCustomerRepository{
		logger:             logger,
		cache:              cache,
		CustomerRepository: primaryRps,
	}
}

func (r *redisCachedCustomerRepository) FindById(ctx context.Context, id string) (*customer.Customer, error) {
	c, err := r.cache.FindById(ctx, id)
	if err != nil {
		r.logger.Errorf("bypassing cache: failed to access cache for reading customer %s - %v", id, err)
	}

	if c != nil {
		return c, nil
	}

	c, err = r.CustomerRepository.FindById(ctx, id)
	if err != nil {
		return nil, err
	}

	if c == nil {
		return nil, nil
	}

	if err := r.cache.Cache(ctx, c); err != nil {
		r.logger.Errorf("failed to cache customer %s - %v", id, err)
	}
	return c, nil
}

func (r *redisCachedCustomerRepository) Update(ctx context.Context, c *customer.Customer) error {
	if err := r.cache.EvictById(ctx, c.Id); err != nil {
		r.logger.Errorf("failed to access cache for customer %s eviction - %v", c.Id, err)
		return err
	}
	return r.CustomerRepository.Update(ctx, c)
}

func (r *redisCachedCustomerRepository) DeleteById(ctx context.Context, id string) error {
	if err := r.cache.EvictById(ctx, id); err != nil {
		r.logger.Errorf("failed to access cache for customer %s eviction - %v", id, err)
		return err
	}
	return r.CustomerRepository.DeleteById(ctx, id)
}
