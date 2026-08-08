package cache

import (
	"context"
	"sync"
	"time"
)

type memoryItem struct {
	value     []byte
	expiresAt time.Time // zero means no expiry
}

func (i memoryItem) expired() bool {
	return !i.expiresAt.IsZero() && time.Now().After(i.expiresAt)
}

// MemoryCache is an in-process Cache backed by a sharded-free map guarded by a
// mutex. It is ideal for local development, tests, and single-instance
// deployments. Expired entries are reclaimed lazily on access and by a
// background janitor.
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]memoryItem
	stop  chan struct{}
}

// NewMemory builds a MemoryCache and starts its janitor goroutine.
func NewMemory() *MemoryCache {
	c := &MemoryCache{
		items: make(map[string]memoryItem),
		stop:  make(chan struct{}),
	}
	go c.janitor(time.Minute)
	return c
}

func (c *MemoryCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || item.expired() {
		return nil, ErrMiss
	}
	return item.value, nil
}

func (c *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}
	c.mu.Lock()
	c.items[key] = memoryItem{value: value, expiresAt: expiresAt}
	c.mu.Unlock()
	return nil
}

func (c *MemoryCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	for _, k := range keys {
		delete(c.items, k)
	}
	c.mu.Unlock()
	return nil
}

func (c *MemoryCache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	return ok && !item.expired(), nil
}

func (c *MemoryCache) Close() error {
	close(c.stop)
	return nil
}

func (c *MemoryCache) janitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.mu.Lock()
			for k, item := range c.items {
				if item.expired() {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		}
	}
}
