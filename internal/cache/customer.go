package cache

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/umalmyha/customers/internal/model/customer"
	"github.com/vmihailenco/msgpack/v5"
	"time"
)

const (
	cachedCustomerTimeToLive = 3 * time.Minute
	customerStreamName       = "customers-stream"
	customerStreamMaxLen     = 1000
	streamCacheWriteTimeout  = 5 * time.Second
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
	value, err := r.client.Get(ctx, streamedCustomerKey(id)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}

	var c customer.Customer
	if err := msgpack.Unmarshal([]byte(value), &c); err != nil {
		return nil, err
	}

	return &c, nil
}

func (r *redisStreamCustomerCache) Create(ctx context.Context, c *customer.Customer) error {
	value, err := msgpack.Marshal(c)
	if err != nil {
		return err
	}

	return r.sendMessage(ctx, map[string]any{
		"op":    "create",
		"id":    c.Id,
		"value": value,
	})
}

func (r *redisStreamCustomerCache) DeleteById(ctx context.Context, id string) error {
	return r.sendMessage(ctx, map[string]any{
		"op": "delete",
		"id": id,
	})
}

func (r *redisStreamCustomerCache) sendMessage(ctx context.Context, values any) error {
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: customerStreamName,
		MaxLen: customerStreamMaxLen,
		Approx: true,
		ID:     "*",
		Values: values,
	}).Err()
}

type redisCustomerCacheUpdater struct {
	logger logrus.FieldLogger
	client *redis.Client
	stop   context.CancelFunc
}

func NewRedisCustomerCacheUpdater(logger logrus.FieldLogger, client *redis.Client) CacheUpdater {
	return &redisCustomerCacheUpdater{logger: logger, client: client}
}

func (r *redisCustomerCacheUpdater) Listen() error {
	r.logger.Info("starting to listen for cache updates...")

	ctx, cancel := context.WithCancel(context.Background())
	r.stop = cancel
	streamKey := "$"

Listen:
	for {
		select {
		case <-ctx.Done():
			break Listen
		default:
			r.logger.Infof("awaiting messages starting from %s", streamKey)
			nextKey, err := r.readStream(ctx, streamKey)
			if err != nil {
				r.logger.Errorf("error occurred on message processing - %v", err)
				if errors.Is(err, redis.ErrClosed) {
					return err
				}
			}
			r.logger.Info("messages processing is finished")
			streamKey = nextKey
		}
	}

	r.logger.Info("listen loop is stopped")

	return nil
}

func (r *redisCustomerCacheUpdater) Stop() {
	if r.stop == nil {
		r.logger.Info("listen loop hasn't been started yet")
		return
	}

	r.logger.Info("stopping the listen loop...")
	r.stop()
	r.stop = nil
}

func (r *redisCustomerCacheUpdater) readStream(ctx context.Context, streamKey string) (string, error) {
	streams, err := r.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{customerStreamName, streamKey},
		Count:   10,
		Block:   0,
	}).Result()
	if err != nil {
		return "", err
	}

	r.logger.Info("messages have been received, processing...")

	nextKey := ""
	for _, stream := range streams {
		r.logger.Infof("number of messages received %d", len(stream.Messages))
		for _, msg := range stream.Messages {
			nextKey = msg.ID
			if err := r.processMessage(msg); err != nil {
				return nextKey, fmt.Errorf("failed to process message %s - %w", msg.ID, err)
			}
		}
	}

	return nextKey, nil
}

func (r *redisCustomerCacheUpdater) processMessage(msg redis.XMessage) error {
	op, ok := msg.Values["op"].(string)
	if !ok || op == "" {
		return errors.New("incorrect message format received - op is missing")
	}

	id, ok := msg.Values["id"].(string)
	if !ok || id == "" {
		return errors.New("incorrect message format received - id is missing")
	}
	customerKey := streamedCustomerKey(id)

	r.logger.Infof("process %s operation for customer %s", op, id)

	ctx, cancel := context.WithTimeout(context.Background(), streamCacheWriteTimeout)
	defer cancel()

	switch op {
	case "create":
		value, ok := msg.Values["value"].(string)
		if !ok {
			return errors.New("incorrect message format received - value is missing for create operation")
		}
		return r.client.SetNX(ctx, customerKey, value, cachedCustomerTimeToLive).Err()
	case "delete":
		return r.client.Del(ctx, customerKey).Err()
	}

	return nil
}

func streamedCustomerKey(id string) string {
	return fmt.Sprintf("customer::%s", id)
}
