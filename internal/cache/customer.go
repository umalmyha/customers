package cache

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v9"
	"github.com/umalmyha/customers/internal/model/customer"
	"github.com/vmihailenco/msgpack/v5"
	"strconv"
	"time"
)

const (
	cachedCustomerTimeToLive = 3 * time.Minute
	cachedCustomersStream    = "customers"
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

type redisStreamCustomerCache struct {
	client *redis.Client
}

func NewRedisStreamCustomerCache(client *redis.Client) CustomerCache {
	return &redisStreamCustomerCache{client: client}
}

func (r *redisStreamCustomerCache) FindById(ctx context.Context, id string) (*customer.Customer, error) {
	msg, err := r.findStreamMessageById(ctx, id)
	if err != nil {
		return nil, err
	}

	if msg == nil {
		return nil, nil
	}

	encValue, ok := msg.Values["value"].(string)
	if !ok {
		return nil, errors.New("failed to parse customer encoded in message")
	}

	var c customer.Customer
	if err := msgpack.Unmarshal([]byte(encValue), &c); err != nil {
		return nil, err
	}

	return &c, nil
}

func (r *redisStreamCustomerCache) Create(ctx context.Context, c *customer.Customer) error {
	encValue, err := msgpack.Marshal(c)
	if err != nil {
		return err
	}

	now := r.now()
	minId := r.minId(now)
	id := fmt.Sprintf("%s-*", strconv.FormatInt(now.Unix(), 10))

	_, err = r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: cachedCustomersStream,
		MinID:  minId,
		ID:     id,
		Values: map[string]any{
			"id":    c.Id,
			"value": encValue,
		},
	}).Result()
	if err != nil {
		return err
	}

	return nil
}

func (r *redisStreamCustomerCache) DeleteById(ctx context.Context, id string) error {
	msg, err := r.findStreamMessageById(ctx, id)
	if err != nil {
		return err
	}

	if msg == nil {
		return nil
	}

	if _, err := r.client.XDel(ctx, cachedCustomersStream, msg.ID).Result(); err != nil {
		return err
	}

	return nil
}

func (r *redisStreamCustomerCache) findStreamMessageById(ctx context.Context, id string) (*redis.XMessage, error) {
	minId := r.minId(r.now())

	messages, err := r.client.XRevRange(ctx, cachedCustomersStream, "+", minId).Result()
	if err != nil {
		return nil, err
	}

	for _, m := range messages {
		if v, ok := m.Values["id"]; ok && v == id {
			return &m, nil
		}
	}
	return nil, nil
}

func (r *redisStreamCustomerCache) now() time.Time {
	return time.Now().UTC()
}

func (r *redisStreamCustomerCache) minId(now time.Time) string {
	id := now.Add(-cachedCustomerTimeToLive).Unix()
	return strconv.FormatInt(id, 10)
}
