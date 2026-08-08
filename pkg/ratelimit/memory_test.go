package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestMemoryLimiter_AllowsThenBlocks(t *testing.T) {
	l := NewMemory()
	defer l.Close()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		res, err := l.Allow(ctx, "k", 3, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Allowed {
			t.Fatalf("request %d should be allowed", i)
		}
	}

	res, _ := l.Allow(ctx, "k", 3, time.Minute)
	if res.Allowed {
		t.Fatal("4th request should be blocked")
	}
	if res.RetryAfter <= 0 {
		t.Error("expected positive RetryAfter when blocked")
	}
}

func TestMemoryLimiter_WindowResets(t *testing.T) {
	l := NewMemory()
	defer l.Close()
	ctx := context.Background()

	l.Allow(ctx, "k", 1, 20*time.Millisecond)
	if res, _ := l.Allow(ctx, "k", 1, 20*time.Millisecond); res.Allowed {
		t.Fatal("should be blocked within window")
	}
	time.Sleep(30 * time.Millisecond)
	if res, _ := l.Allow(ctx, "k", 1, 20*time.Millisecond); !res.Allowed {
		t.Fatal("should be allowed after window reset")
	}
}

func TestMemoryLimiter_KeysIsolated(t *testing.T) {
	l := NewMemory()
	defer l.Close()
	ctx := context.Background()

	l.Allow(ctx, "a", 1, time.Minute)
	if res, _ := l.Allow(ctx, "b", 1, time.Minute); !res.Allowed {
		t.Fatal("different key should have its own budget")
	}
}

func BenchmarkMemoryLimiter_Allow(b *testing.B) {
	l := NewMemory()
	defer l.Close()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = l.Allow(ctx, "k", 1_000_000, time.Minute)
	}
}
