package places

import (
	"context"
	"strings"
	"sync"
	"time"
)

type cacheEntry struct {
	result    Result
	expiresAt time.Time
}

// CachingProvider wraps a Provider and caches geocode results in memory.
type CachingProvider struct {
	inner Provider
	ttl   time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewCachingProvider wraps inner and caches Geocode results for ttl.
func NewCachingProvider(inner Provider, ttl time.Duration) *CachingProvider {
	return &CachingProvider{
		inner: inner,
		ttl:   ttl,
		cache: make(map[string]cacheEntry),
	}
}

// Geocode returns a cached result when available.
func (c *CachingProvider) Geocode(ctx context.Context, address string) (Result, error) {
	key := strings.ToLower(strings.TrimSpace(address))
	if key == "" {
		return c.inner.Geocode(ctx, address)
	}

	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.cache[key]; ok && now.Before(entry.expiresAt) {
		result := entry.result
		c.mu.Unlock()
		return result, nil
	}
	c.mu.Unlock()

	result, err := c.inner.Geocode(ctx, address)
	if err != nil {
		return Result{}, err
	}

	c.mu.Lock()
	c.cache[key] = cacheEntry{result: result, expiresAt: now.Add(c.ttl)}
	c.mu.Unlock()
	return result, nil
}

// SearchNearby delegates to the inner provider without caching.
func (c *CachingProvider) SearchNearby(ctx context.Context, keyword string, lat, lng float64, radiusMeters int) ([]Place, error) {
	return c.inner.SearchNearby(ctx, keyword, lat, lng, radiusMeters)
}
