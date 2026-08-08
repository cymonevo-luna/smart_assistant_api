package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryCache_SetGetDelete(t *testing.T) {
	c := NewMemory()
	defer c.Close()
	ctx := context.Background()

	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get(ctx, "k")
	if err != nil || string(got) != "v" {
		t.Fatalf("got %q err %v", got, err)
	}

	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrMiss) {
		t.Fatalf("expected miss, got %v", err)
	}
}

func TestMemoryCache_Expiry(t *testing.T) {
	c := NewMemory()
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "k", []byte("v"), 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if _, err := c.Get(ctx, "k"); !errors.Is(err, ErrMiss) {
		t.Fatalf("expected expired miss, got %v", err)
	}
}

func TestTyped_GetOrSet(t *testing.T) {
	c := NewMemory()
	defer c.Close()
	typed := NewTyped[string](c, time.Minute)
	ctx := context.Background()

	calls := 0
	loader := func(context.Context) (*string, error) {
		calls++
		v := "loaded"
		return &v, nil
	}

	for i := 0; i < 3; i++ {
		v, err := typed.GetOrSet(ctx, "k", loader)
		if err != nil || *v != "loaded" {
			t.Fatalf("got %v err %v", v, err)
		}
	}
	if calls != 1 {
		t.Errorf("loader should run once, ran %d times", calls)
	}
}

func BenchmarkMemoryCache_SetGet(b *testing.B) {
	c := NewMemory()
	defer c.Close()
	ctx := context.Background()
	val := []byte("value")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Set(ctx, "k", val, time.Minute)
		_, _ = c.Get(ctx, "k")
	}
}
