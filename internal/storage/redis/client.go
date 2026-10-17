package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func NewClient(addr, password string, db int) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

func (c *Client) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, "lock:"+key, "1", ttl).Result()
}

func (c *Client) ReleaseLock(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, "lock:"+key).Err()
}

func (c *Client) AllowRequest(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	fullKey := "ratelimit:" + key

	count, err := c.rdb.Incr(ctx, fullKey).Result()
	if err != nil {
		return false, err
	}

	if count == 1 {
		c.rdb.Expire(ctx, fullKey, window)
	}

	if count > limit {
		return false, nil
	}

	return true, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}
