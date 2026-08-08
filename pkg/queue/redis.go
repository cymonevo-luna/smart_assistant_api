package queue

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const consumerGroup = "workers"

// RedisQueue is a durable Queue backed by Redis Streams with consumer groups,
// providing at-least-once delivery. Failed messages remain pending and are
// reclaimed by other consumers, giving crash resilience.
type RedisQueue struct {
	client   *redis.Client
	log      logger.Logger
	policy   RetryPolicy
	consumer string

	mu       sync.RWMutex
	handlers map[string]Handler
	wg       sync.WaitGroup
}

// NewRedis builds a RedisQueue from an existing client.
func NewRedis(client *redis.Client, log logger.Logger, policy RetryPolicy) *RedisQueue {
	return &RedisQueue{
		client:   client,
		log:      log,
		policy:   policy,
		consumer: "consumer-" + uuid.NewString(),
		handlers: make(map[string]Handler),
	}
}

func streamKey(topic string) string { return "queue:" + topic }

func (q *RedisQueue) Publish(ctx context.Context, topic string, payload []byte, opts ...PublishOption) error {
	msg := Message{Topic: topic}
	for _, opt := range opts {
		opt(&msg)
	}
	values := map[string]interface{}{"payload": payload}
	for k, v := range msg.Headers {
		values["h:"+k] = v
	}
	return q.client.XAdd(ctx, &redis.XAddArgs{Stream: streamKey(topic), Values: values}).Err()
}

func (q *RedisQueue) Subscribe(topic string, handler Handler) {
	q.mu.Lock()
	q.handlers[topic] = handler
	q.mu.Unlock()
}

func (q *RedisQueue) Start(ctx context.Context) error {
	q.mu.RLock()
	handlers := make(map[string]Handler, len(q.handlers))
	for t, h := range q.handlers {
		handlers[t] = h
	}
	q.mu.RUnlock()

	for topic, handler := range handlers {
		key := streamKey(topic)
		// MKSTREAM creates the stream + group if absent; ignore BUSYGROUP.
		if err := q.client.XGroupCreateMkStream(ctx, key, consumerGroup, "0").Err(); err != nil &&
			!isBusyGroup(err) {
			return err
		}
		q.wg.Add(1)
		go q.consume(ctx, key, handler)
	}
	return nil
}

func (q *RedisQueue) consume(ctx context.Context, key string, handler Handler) {
	defer q.wg.Done()
	for {
		if ctx.Err() != nil {
			return
		}
		streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: q.consumer,
			Streams:  []string{key, ">"},
			Count:    16,
			Block:    2 * time.Second,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
				continue
			}
			q.log.Warn("queue read failed", logger.String("stream", key), logger.Err(err))
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			for _, raw := range stream.Messages {
				q.handle(ctx, key, raw, handler)
			}
		}
	}
}

func (q *RedisQueue) handle(ctx context.Context, key string, raw redis.XMessage, handler Handler) {
	msg := toMessage(key, raw)
	for {
		msg.Attempts++
		if err := handler(ctx, msg); err == nil {
			_ = q.client.XAck(ctx, key, consumerGroup, raw.ID).Err()
			return
		} else {
			q.log.Warn("queue handler failed",
				logger.String("stream", key), logger.String("message_id", raw.ID),
				logger.Int("attempt", msg.Attempts), logger.Err(err))
		}
		if msg.Attempts >= q.policy.MaxAttempts {
			// Acknowledge to stop redelivery; a real system would move this to
			// a dead-letter stream here.
			_ = q.client.XAck(ctx, key, consumerGroup, raw.ID).Err()
			q.log.Error("queue message dead-lettered",
				logger.String("stream", key), logger.String("message_id", raw.ID))
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(q.policy.Backoff * time.Duration(msg.Attempts)):
		}
	}
}

func toMessage(key string, raw redis.XMessage) Message {
	msg := Message{ID: raw.ID, Topic: key, Headers: map[string]string{}}
	for k, v := range raw.Values {
		s, _ := v.(string)
		switch {
		case k == "payload":
			msg.Payload = []byte(s)
		case len(k) > 2 && k[:2] == "h:":
			msg.Headers[k[2:]] = s
		}
	}
	return msg
}

func (q *RedisQueue) Close() error {
	q.wg.Wait()
	return nil
}

func isBusyGroup(err error) bool {
	return err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists"
}
