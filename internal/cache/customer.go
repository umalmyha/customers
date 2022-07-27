package cache

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v9"
	"github.com/umalmyha/customers/internal/model/customer"
	"github.com/vmihailenco/msgpack/v5"
	"sync"
	"time"
)

const (
	cachedCustomerTimeToLive = 3 * time.Minute
	customerStreamMaxLen     = 1000
)

type CustomerCache interface {
	FindById(context.Context, string) (*customer.Customer, error)
	DeleteById(context.Context, string) error
	Create(context.Context, *customer.Customer) error
}

type redisCustomerCache struct {
	client *redis.Client
}

func NewRedisCustomerCache(client *redis.Client) CustomerCache {
	return &redisCustomerCache{client: client}
}

func (r *redisCustomerCache) FindById(ctx context.Context, id string) (*customer.Customer, error) {
	res, err := r.client.Get(ctx, r.key(id)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}

	var c customer.Customer
	if err := msgpack.Unmarshal([]byte(res), &c); err != nil {
		return nil, err
	}

	return &c, nil
}

func (r *redisCustomerCache) DeleteById(ctx context.Context, id string) error {
	if _, err := r.client.Del(ctx, r.key(id)).Result(); err != nil {
		return err
	}
	return nil
}

func (r *redisCustomerCache) Create(ctx context.Context, c *customer.Customer) error {
	encoded, err := msgpack.Marshal(c)
	if err != nil {
		return err
	}

	_, err = r.client.SetNX(ctx, r.key(c.Id), encoded, cachedCustomerTimeToLive).Result()
	if err != nil {
		return err
	}
	return nil
}

func (r *redisCustomerCache) key(id string) string {
	return fmt.Sprintf("customer:%s", id)
}

type inMemoryCache struct {
	customers map[string]*customer.Customer
	mu        sync.RWMutex
}

func NewInMemoryCache() CustomerCache {
	return &inMemoryCache{
		customers: make(map[string]*customer.Customer),
	}
}

func (c *inMemoryCache) FindById(_ context.Context, id string) (*customer.Customer, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	customer, ok := c.customers[id]
	if !ok {
		return nil, nil
	}

	return customer, nil
}

func (c *inMemoryCache) Create(_ context.Context, customer *customer.Customer) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.customers[customer.Id] = customer
	return nil
}

func (c *inMemoryCache) DeleteById(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.customers, id)
	return nil
}

type redisStreamCustomerCache struct {
	client *redis.Client
	CustomerCache
}

func NewRedisStreamCustomerCache(client *redis.Client, primary CustomerCache) CustomerCache {
	return &redisStreamCustomerCache{client: client, CustomerCache: primary}
}

func (r *redisStreamCustomerCache) Create(ctx context.Context, c *customer.Customer) error {
	value, err := msgpack.Marshal(c)
	if err != nil {
		return err
	}

	return r.sendMessage(ctx, "create", value)
}

func (r *redisStreamCustomerCache) DeleteById(ctx context.Context, id string) error {
	return r.sendMessage(ctx, "delete", id)
}

func (r *redisStreamCustomerCache) sendMessage(ctx context.Context, op string, value any) error {
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: "customers-stream",
		MaxLen: customerStreamMaxLen,
		Approx: true,
		ID:     "*",
		Values: map[string]any{
			"op":    op,
			"value": value,
		},
	}).Err()
}
