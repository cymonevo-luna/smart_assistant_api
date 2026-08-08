package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cymonevo/go_template/pkg/logger"
)

func newTestLogger(t *testing.T) logger.Logger {
	t.Helper()
	log, err := logger.New("error", false)
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}
	return log
}

// TestSchedulerRunsTasks verifies a registered task fires on its interval.
func TestSchedulerRunsTasks(t *testing.T) {
	s := NewScheduler(newTestLogger(t))

	var runs atomic.Int32
	s.Register("tick", 10*time.Millisecond, func(context.Context) error {
		runs.Add(1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)

	time.Sleep(55 * time.Millisecond)
	cancel()
	s.Stop(testCtx(t))

	if runs.Load() == 0 {
		t.Fatal("expected the task to run at least once")
	}
}

// TestSchedulerStopWaitsForInFlightTask ensures a task already executing when
// Stop is called is allowed to finish rather than being cut off — this is what
// keeps a deploy/restart from interrupting an in-progress orchestration tick.
func TestSchedulerStopWaitsForInFlightTask(t *testing.T) {
	s := NewScheduler(newTestLogger(t))

	started := make(chan struct{})
	var completed atomic.Bool
	var ctxCancelled atomic.Bool

	s.Register("slow", 5*time.Millisecond, func(ctx context.Context) error {
		select {
		case <-started:
			// already signalled; subsequent runs are no-ops
		default:
			close(started)
		}
		// Simulate in-flight work; observe whether our context was cancelled.
		select {
		case <-time.After(80 * time.Millisecond):
		case <-ctx.Done():
			ctxCancelled.Store(true)
		}
		completed.Store(true)
		return nil
	})

	s.Start(context.Background())
	<-started // a run is now in flight

	// Generous grace: Stop should wait for the in-flight run to finish cleanly.
	s.Stop(testCtx(t))

	if !completed.Load() {
		t.Fatal("Stop returned before the in-flight task completed")
	}
	if ctxCancelled.Load() {
		t.Fatal("in-flight task context was cancelled despite ample grace period")
	}
}

// TestSchedulerStopCancelsAfterGrace ensures Stop cancels a stuck task once the
// grace period expires, so shutdown cannot hang forever.
func TestSchedulerStopCancelsAfterGrace(t *testing.T) {
	s := NewScheduler(newTestLogger(t))

	started := make(chan struct{})
	var ctxCancelled atomic.Bool

	s.Register("stuck", 5*time.Millisecond, func(ctx context.Context) error {
		select {
		case <-started:
		default:
			close(started)
		}
		<-ctx.Done() // never returns until cancelled
		ctxCancelled.Store(true)
		return nil
	})

	s.Start(context.Background())
	<-started

	graceCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	s.Stop(graceCtx)

	if !ctxCancelled.Load() {
		t.Fatal("expected the stuck task to be cancelled once the grace period expired")
	}
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}
