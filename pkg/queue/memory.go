package queue

import (
	"context"
	"sync"
	"time"

	"github.com/cymonevo/go_template/pkg/logger"
	"github.com/google/uuid"
)

// MemoryQueue is an in-process Queue backed by buffered channels. Delivery is
// at-most-once across process restarts (messages live only in memory) but it
// retries failed handlers per the configured policy within the process.
type MemoryQueue struct {
	log       logger.Logger
	policy    RetryPolicy
	bufSize   int
	mu        sync.RWMutex
	handlers  map[string]Handler
	topics    map[string]chan Message
	wg        sync.WaitGroup
	started   bool
	done      chan struct{}
	closeOnce sync.Once
}

// NewMemory builds a MemoryQueue.
func NewMemory(log logger.Logger, policy RetryPolicy) *MemoryQueue {
	return &MemoryQueue{
		log:      log,
		policy:   policy,
		bufSize:  1024,
		handlers: make(map[string]Handler),
		topics:   make(map[string]chan Message),
		done:     make(chan struct{}),
	}
}

func (q *MemoryQueue) channel(topic string) chan Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	ch, ok := q.topics[topic]
	if !ok {
		ch = make(chan Message, q.bufSize)
		q.topics[topic] = ch
	}
	return ch
}

func (q *MemoryQueue) Publish(ctx context.Context, topic string, payload []byte, opts ...PublishOption) error {
	msg := Message{ID: uuid.NewString(), Topic: topic, Payload: payload, Attempts: 0}
	for _, opt := range opts {
		opt(&msg)
	}

	ch := q.channel(topic)
	select {
	case ch <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *MemoryQueue) Subscribe(topic string, handler Handler) {
	q.mu.Lock()
	q.handlers[topic] = handler
	q.mu.Unlock()
	q.channel(topic) // ensure the channel exists before Start.
}

func (q *MemoryQueue) Start(ctx context.Context) error {
	q.mu.Lock()
	if q.started {
		q.mu.Unlock()
		return nil
	}
	q.started = true
	handlers := make(map[string]Handler, len(q.handlers))
	for t, h := range q.handlers {
		handlers[t] = h
	}
	q.mu.Unlock()

	for topic, handler := range handlers {
		ch := q.channel(topic)
		q.wg.Add(1)
		go q.consume(ctx, topic, ch, handler)
	}
	return nil
}

func (q *MemoryQueue) consume(ctx context.Context, topic string, ch chan Message, handler Handler) {
	defer q.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.done:
			return
		case msg := <-ch:
			q.dispatch(ctx, topic, msg, handler)
		}
	}
}

func (q *MemoryQueue) dispatch(ctx context.Context, topic string, msg Message, handler Handler) {
	for {
		msg.Attempts++
		err := handler(ctx, msg)
		if err == nil {
			return
		}
		q.log.Warn("queue handler failed",
			logger.String("topic", topic),
			logger.String("message_id", msg.ID),
			logger.Int("attempt", msg.Attempts),
			logger.Err(err),
		)
		if msg.Attempts >= q.policy.MaxAttempts {
			q.log.Error("queue message dropped after max attempts",
				logger.String("topic", topic), logger.String("message_id", msg.ID))
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-q.done:
			return
		case <-time.After(q.policy.Backoff * time.Duration(msg.Attempts)):
		}
	}
}

// Close signals the consumer goroutines to stop and waits for them to drain.
// It is safe to call multiple times. Without the stop signal, Close would block
// forever whenever the queue was started with a context that never cancels
// (e.g. context.Background()).
func (q *MemoryQueue) Close() error {
	q.closeOnce.Do(func() { close(q.done) })
	q.wg.Wait()
	return nil
}
