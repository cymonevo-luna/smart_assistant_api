package worker

import (
	"context"
	"sync"
	"time"

	"github.com/cymonevo/go_template/pkg/logger"
)

// TaskFunc is a recurring task executed by the Scheduler.
type TaskFunc func(ctx context.Context) error

type task struct {
	name     string
	interval time.Duration
	run      TaskFunc
}

// Scheduler runs registered tasks on fixed intervals. It is a lightweight
// alternative to a full cron system, suitable for periodic maintenance such as
// cache warming, cleanup, or metrics emission.
//
// Shutdown is graceful: once Stop is called (or the Start context is cancelled)
// no new task runs are scheduled, but a task that is already executing is
// allowed to finish. Its context is cancelled only if the Stop grace period
// expires, so in-flight work can complete cleanly instead of being torn down
// mid-operation.
type Scheduler struct {
	log      logger.Logger
	tasks    []task
	wg       sync.WaitGroup
	quit     chan struct{}
	stopOnce sync.Once
	runCtx   context.Context
	cancel   context.CancelFunc
}

// NewScheduler builds a Scheduler.
func NewScheduler(log logger.Logger) *Scheduler {
	return &Scheduler{log: log, quit: make(chan struct{})}
}

// Register adds a recurring task. Call before Start.
func (s *Scheduler) Register(name string, interval time.Duration, run TaskFunc) {
	s.tasks = append(s.tasks, task{name: name, interval: interval, run: run})
}

// Start launches a goroutine per task. Tasks receive an internal context that
// stays live until Stop's grace period expires, so cancelling the lifecycle
// ctx only stops the scheduling of new runs (an in-flight run keeps going).
func (s *Scheduler) Start(ctx context.Context) {
	s.runCtx, s.cancel = context.WithCancel(context.Background())
	for _, t := range s.tasks {
		s.wg.Add(1)
		go s.loop(t)
	}
	// Begin a graceful stop when the lifecycle context is cancelled (e.g. on
	// SIGTERM) so no further runs are scheduled even before Stop is invoked.
	go func() {
		select {
		case <-ctx.Done():
			s.beginStop()
		case <-s.quit:
		}
	}()
}

func (s *Scheduler) loop(t task) {
	defer s.wg.Done()
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-ticker.C:
			if err := t.run(s.runCtx); err != nil {
				s.log.Error("scheduled task failed",
					logger.String("task", t.name), logger.Err(err))
			}
		}
	}
}

func (s *Scheduler) beginStop() {
	s.stopOnce.Do(func() { close(s.quit) })
}

// Stop stops scheduling new task runs and waits for any in-flight run to finish,
// bounded by ctx. If ctx expires first, in-flight runs have their context
// cancelled and Stop blocks until they return. Safe to call more than once.
func (s *Scheduler) Stop(ctx context.Context) {
	s.beginStop()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	}
}

// Wait blocks until all task goroutines have exited.
func (s *Scheduler) Wait() { s.wg.Wait() }
