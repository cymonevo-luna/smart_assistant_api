package ratelimit

import (
	"context"
	"sync"
	"time"
)

type window struct {
	count    int
	resetsAt time.Time
}

// MemoryLimiter is an in-process fixed-window limiter. It is appropriate for
// single-instance deployments and local development.
type MemoryLimiter struct {
	mu      sync.Mutex
	windows map[string]*window
	stop    chan struct{}
}

// NewMemory builds a MemoryLimiter and starts a janitor that evicts stale keys.
func NewMemory() *MemoryLimiter {
	l := &MemoryLimiter{windows: make(map[string]*window), stop: make(chan struct{})}
	go l.janitor()
	return l
}

func (l *MemoryLimiter) Allow(_ context.Context, key string, limit int, dur time.Duration) (Result, error) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	w, ok := l.windows[key]
	if !ok || now.After(w.resetsAt) {
		w = &window{count: 0, resetsAt: now.Add(dur)}
		l.windows[key] = w
	}

	w.count++
	remaining := limit - w.count
	if remaining < 0 {
		return Result{Allowed: false, Limit: limit, Remaining: 0, RetryAfter: time.Until(w.resetsAt)}, nil
	}
	return Result{Allowed: true, Limit: limit, Remaining: remaining, RetryAfter: 0}, nil
}

func (l *MemoryLimiter) Close() error {
	close(l.stop)
	return nil
}

func (l *MemoryLimiter) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case now := <-ticker.C:
			l.mu.Lock()
			for k, w := range l.windows {
				if now.After(w.resetsAt) {
					delete(l.windows, k)
				}
			}
			l.mu.Unlock()
		}
	}
}
