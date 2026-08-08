// Package ratelimit provides a backend-agnostic request rate limiter.
//
// The Limiter interface is implemented by an in-memory token bucket (single
// instance) and a Redis fixed-window counter (distributed). Callers — including
// the HTTP middleware — depend only on the interface, so switching backends is
// a wiring-time decision.
package ratelimit

import (
	"context"
	"time"
)

// Result describes the outcome of a rate-limit check.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

// Limiter decides whether an action identified by key may proceed.
type Limiter interface {
	// Allow reports whether the request for key is permitted given a budget of
	// limit events per window.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error)
	Close() error
}
