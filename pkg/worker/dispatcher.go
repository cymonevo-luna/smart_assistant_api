// Package worker provides background job processing on top of the queue
// abstraction, plus a periodic scheduler for recurring tasks.
package worker

import (
	"context"
	"encoding/json"

	"github.com/cymonevo/go_template/pkg/queue"
)

// Enqueue marshals a typed job payload and publishes it to a topic. This is the
// producer side used by services to offload work asynchronously.
func Enqueue[T any](ctx context.Context, q queue.Queue, topic string, payload T, opts ...queue.PublishOption) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return q.Publish(ctx, topic, data, opts...)
}

// JobFunc handles a typed job payload.
type JobFunc[T any] func(ctx context.Context, payload T) error

// Register wires a typed handler for a topic onto the queue, decoding the JSON
// payload into T before invoking fn. This keeps job handlers strongly typed
// while the queue itself deals only in bytes.
func Register[T any](q queue.Queue, topic string, fn JobFunc[T]) {
	q.Subscribe(topic, func(ctx context.Context, msg queue.Message) error {
		var payload T
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return err
		}
		return fn(ctx, payload)
	})
}
