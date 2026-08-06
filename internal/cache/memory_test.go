package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryGetSet(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	if _, ok, err := c.Get(ctx, "k"); err != nil || ok {
		t.Fatalf("expected miss, got ok=%v err=%v", ok, err)
	}

	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, ok, err := c.Get(ctx, "k")
	if err != nil || !ok || got != "v" {
		t.Fatalf("expected hit v, got %q ok=%v err=%v", got, ok, err)
	}
}

func TestMemoryExpiry(t *testing.T) {
	c := NewMemory()
	now := time.Now()
	c.now = func() time.Time { return now }
	ctx := context.Background()

	if err := c.Set(ctx, "k", "v", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}

	now = now.Add(2 * time.Minute)
	if _, ok, err := c.Get(ctx, "k"); err != nil || ok {
		t.Fatalf("expected expired miss, got ok=%v err=%v", ok, err)
	}
}

func TestMemoryNoTTL(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	if err := c.Set(ctx, "k", "v", 0); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, ok, err := c.Get(ctx, "k")
	if err != nil || !ok || got != "v" {
		t.Fatalf("expected hit v, got %q ok=%v err=%v", got, ok, err)
	}

	if err := c.Del(ctx, "k"); err != nil {
		t.Fatalf("del: %v", err)
	}
	if _, ok, err := c.Get(ctx, "k"); err != nil || ok {
		t.Fatalf("expected miss after del, got ok=%v err=%v", ok, err)
	}
}

func TestMemoryConcurrent(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				_ = c.Set(ctx, "k", "v", time.Minute)
				_, _, _ = c.Get(ctx, "k")
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
