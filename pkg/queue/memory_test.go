package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cymonevo/go_template/pkg/logger"
)

func testLogger(t *testing.T) logger.Logger {
	t.Helper()
	l, err := logger.New("error", false)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestMemoryQueue_PublishConsume(t *testing.T) {
	q := NewMemory(testLogger(t), DefaultRetryPolicy)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	var got string
	q.Subscribe("topic", func(_ context.Context, msg Message) error {
		got = string(msg.Payload)
		wg.Done()
		return nil
	})

	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := q.Publish(ctx, "topic", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	waitOrTimeout(t, &wg, time.Second)
	if got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
}

func TestMemoryQueue_RetriesUntilSuccess(t *testing.T) {
	q := NewMemory(testLogger(t), RetryPolicy{MaxAttempts: 5, Backoff: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	var attempts int
	var mu sync.Mutex
	q.Subscribe("topic", func(_ context.Context, _ Message) error {
		mu.Lock()
		attempts++
		current := attempts
		mu.Unlock()
		if current < 3 {
			return errors.New("transient")
		}
		wg.Done()
		return nil
	})

	_ = q.Start(ctx)
	_ = q.Publish(ctx, "topic", []byte("x"))

	waitOrTimeout(t, &wg, 2*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

// TestMemoryQueue_CloseWithoutContextCancel guards against a deadlock where
// Close blocked forever when the queue was started with a context that never
// cancels (e.g. context.Background()): the consumers only stopped on ctx.Done,
// so Close's WaitGroup.Wait never returned. Close must signal them itself.
func TestMemoryQueue_CloseWithoutContextCancel(t *testing.T) {
	q := NewMemory(testLogger(t), DefaultRetryPolicy)
	q.Subscribe("topic", func(_ context.Context, _ Message) error { return nil })

	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- q.Close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close deadlocked: consumers were not signalled to stop")
	}

	// Close must be idempotent.
	if err := q.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

func waitOrTimeout(t *testing.T, wg *sync.WaitGroup, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("timed out waiting for handler")
	}
}
