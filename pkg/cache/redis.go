package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache is a Cache backed by Redis, suitable for distributed deployments.
type RedisCache struct {
	client *redis.Client
}

// NewRedis connects to Redis and verifies the connection with a ping.
func NewRedis(ctx context.Context, addr, password string, db int) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &RedisCache{client: client}, nil
}

// NewRedisWithClient wraps an existing client so a single connection can be
// shared across the cache, rate limiter, and message queue.
func NewRedisWithClient(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, ErrMiss
	}
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}

func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	return n > 0, err
}

func (r *RedisCache) Close() error { return r.client.Close() }
