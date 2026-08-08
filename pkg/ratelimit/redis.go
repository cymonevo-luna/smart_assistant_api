package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// fixedWindowScript atomically increments a counter and sets its expiry on the
// first hit, returning the current count and remaining TTL in milliseconds.
// Running it server-side keeps the check race-free across instances.
var fixedWindowScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("PTTL", KEYS[1])
return {current, ttl}
`)

// RedisLimiter is a distributed fixed-window limiter backed by Redis.
type RedisLimiter struct {
	client *redis.Client
}

// NewRedis builds a RedisLimiter from an existing client.
func NewRedis(client *redis.Client) *RedisLimiter {
	return &RedisLimiter{client: client}
}

func (l *RedisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	res, err := fixedWindowScript.Run(ctx, l.client, []string{"ratelimit:" + key}, window.Milliseconds()).Result()
	if err != nil {
		return Result{}, err
	}

	values := res.([]interface{})
	count := values[0].(int64)
	ttlMS := values[1].(int64)

	remaining := limit - int(count)
	if remaining < 0 {
		return Result{Allowed: false, Limit: limit, Remaining: 0, RetryAfter: time.Duration(ttlMS) * time.Millisecond}, nil
	}
	return Result{Allowed: true, Limit: limit, Remaining: remaining}, nil
}

func (l *RedisLimiter) Close() error { return nil }
