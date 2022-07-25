package cache

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v9"
	"github.com/umalmyha/customers/internal/model/customer"
	"github.com/vmihailenco/msgpack/v5"
	"time"
)

const cachedCustomerTimeToLive = 10 * time.Minute

type CustomerCache interface {
	FindById(context.Context, string) (*customer.Customer, error)
	EvictById(context.Context, string) error
	Cache(context.Context, *customer.Customer) error
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

func (r *redisCustomerCache) EvictById(ctx context.Context, id string) error {
	if _, err := r.client.Del(ctx, r.key(id)).Result(); err != nil {
		return err
	}
	return nil
}

func (r *redisCustomerCache) Cache(ctx context.Context, c *customer.Customer) error {
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
