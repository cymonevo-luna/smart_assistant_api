// Package cache provides a database-agnostic caching abstraction.
//
// Like the store package, callers depend only on the Cache interface (or the
// generic Typed[T] helper). Switching between Redis and the in-memory backend
// is a wiring-time decision and requires no changes to business code.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrMiss is returned when a key is absent or expired.
var ErrMiss = errors.New("cache: miss")

// Cache is the low-level byte oriented contract implemented by each backend.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	Close() error
}

// Typed is a generic, JSON-serialising wrapper around a Cache that lets callers
// work with domain types directly instead of raw bytes.
type Typed[T any] struct {
	backend    Cache
	defaultTTL time.Duration
}

// NewTyped builds a typed cache view over the given backend.
func NewTyped[T any](backend Cache, defaultTTL time.Duration) *Typed[T] {
	return &Typed[T]{backend: backend, defaultTTL: defaultTTL}
}

// Get fetches and decodes a value. It returns ErrMiss when absent.
func (t *Typed[T]) Get(ctx context.Context, key string) (*T, error) {
	raw, err := t.backend.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

// Set encodes and stores a value using the default TTL.
func (t *Typed[T]) Set(ctx context.Context, key string, value *T) error {
	return t.SetTTL(ctx, key, value, t.defaultTTL)
}

// SetTTL encodes and stores a value with an explicit TTL.
func (t *Typed[T]) SetTTL(ctx context.Context, key string, value *T, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return t.backend.Set(ctx, key, raw, ttl)
}

// Delete removes keys.
func (t *Typed[T]) Delete(ctx context.Context, keys ...string) error {
	return t.backend.Delete(ctx, keys...)
}

// GetOrSet returns the cached value, or computes it via loader, caches it and
// returns it on a miss. This is the common read-through pattern.
func (t *Typed[T]) GetOrSet(ctx context.Context, key string, loader func(context.Context) (*T, error)) (*T, error) {
	value, err := t.Get(ctx, key)
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, ErrMiss) {
		return nil, err
	}

	loaded, err := loader(ctx)
	if err != nil {
		return nil, err
	}
	if err := t.Set(ctx, key, loaded); err != nil {
		return nil, err
	}
	return loaded, nil
}
