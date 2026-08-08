// Package queue defines a backend-agnostic message queue abstraction used for
// asynchronous work and inter-service events.
//
// Producers call Publish; consumers register a Handler per topic and call
// Start to begin processing. The in-memory backend suits development and tests;
// the Redis Streams backend provides durable, at-least-once delivery with
// consumer groups for production.
package queue

import (
	"context"
	"time"
)

// Message is a unit of work flowing through the queue.
type Message struct {
	ID      string            `json:"id"`
	Topic   string            `json:"topic"`
	Payload []byte            `json:"payload"`
	Headers map[string]string `json:"headers,omitempty"`
	// Attempts counts how many times delivery has been tried (1 on first).
	Attempts int `json:"attempts"`
}

// Handler processes a message. Returning an error signals the backend to retry
// (subject to its retry policy); returning nil acknowledges the message.
type Handler func(ctx context.Context, msg Message) error

// PublishOption customises a Publish call.
type PublishOption func(*Message)

// WithHeader attaches a header to the published message.
func WithHeader(key, value string) PublishOption {
	return func(m *Message) {
		if m.Headers == nil {
			m.Headers = map[string]string{}
		}
		m.Headers[key] = value
	}
}

// Queue is the producer/consumer contract.
type Queue interface {
	// Publish enqueues a payload on a topic.
	Publish(ctx context.Context, topic string, payload []byte, opts ...PublishOption) error
	// Subscribe registers a handler for a topic. Must be called before Start.
	Subscribe(topic string, handler Handler)
	// Start begins consuming. It returns immediately; consumption runs until
	// the context is cancelled.
	Start(ctx context.Context) error
	Close() error
}

// RetryPolicy controls redelivery backoff for backends that support it.
type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

// DefaultRetryPolicy is a sensible default of 3 attempts with linear backoff.
var DefaultRetryPolicy = RetryPolicy{MaxAttempts: 3, Backoff: time.Second}
