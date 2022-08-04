package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/umalmyha/customers/internal/model"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	cachedCustomerTimeToLive = 3 * time.Minute
	customerStreamMaxLen     = 1000
)

// CustomerCacheRepository interface representing customer cache behavior
type CustomerCacheRepository interface {
	FindByID(context.Context, string) (*model.Customer, error)
	DeleteByID(context.Context, string) error
	Create(context.Context, *model.Customer) error
}

type redisCustomerCache struct {
	client *redis.Client
}

// NewRedisCustomerCache builds new redis customer cache
func NewRedisCustomerCache(client *redis.Client) CustomerCacheRepository {
	return &redisCustomerCache{client: client}
}

func (r *redisCustomerCache) FindByID(ctx context.Context, id string) (*model.Customer, error) {
	res, err := r.client.Get(ctx, r.key(id)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}

	var c model.Customer
	if err := msgpack.Unmarshal([]byte(res), &c); err != nil {
		return nil, err
	}

	return &c, nil
}

func (r *redisCustomerCache) DeleteByID(ctx context.Context, id string) error {
	if _, err := r.client.Del(ctx, r.key(id)).Result(); err != nil {
		return err
	}
	return nil
}

func (r *redisCustomerCache) Create(ctx context.Context, c *model.Customer) error {
	encoded, err := msgpack.Marshal(c)
	if err != nil {
		return err
	}

	_, err = r.client.SetNX(ctx, r.key(c.ID), encoded, cachedCustomerTimeToLive).Result()
	if err != nil {
		return err
	}
	return nil
}

func (r *redisCustomerCache) key(id string) string {
	return fmt.Sprintf("customer:%s", id)
}

type inMemoryCache struct {
	customers map[string]*model.Customer
	mu        sync.RWMutex
}

// NewInMemoryCache builds new in-memory cache
func NewInMemoryCache() CustomerCacheRepository {
	return &inMemoryCache{
		customers: make(map[string]*model.Customer),
	}
}

func (c *inMemoryCache) FindByID(_ context.Context, id string) (*model.Customer, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	customer, ok := c.customers[id]
	if !ok {
		return nil, nil
	}

	return customer, nil
}

func (c *inMemoryCache) Create(_ context.Context, customer *model.Customer) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.customers[customer.ID] = customer
	return nil
}

func (c *inMemoryCache) DeleteByID(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.customers, id)
	return nil
}

type redisStreamCustomerCache struct {
	client *redis.Client
	CustomerCacheRepository
}

// NewRedisStreamCustomerCache builds redis stream customer cache
func NewRedisStreamCustomerCache(client *redis.Client, primary CustomerCacheRepository) CustomerCacheRepository {
	return &redisStreamCustomerCache{client: client, CustomerCacheRepository: primary}
}

func (r *redisStreamCustomerCache) Create(ctx context.Context, c *model.Customer) error {
	value, err := msgpack.Marshal(c)
	if err != nil {
		return err
	}

	return r.sendMessage(ctx, "create", value)
}

func (r *redisStreamCustomerCache) DeleteByID(ctx context.Context, id string) error {
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
