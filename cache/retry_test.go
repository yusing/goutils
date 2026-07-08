package cache

import (
	"context"
	"errors"
	"testing"
)

func TestShouldCacheResult(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	testErr := errors.New("test error")

	if !shouldCacheResult(ctx, nil) {
		t.Fatal("active context success should be cached")
	}
	if !shouldCacheResult(ctx, testErr) {
		t.Fatal("active context error should be cached")
	}

	canceledCtx, cancel := context.WithCancelCause(t.Context())
	cancel(testErr)

	if !shouldCacheResult(canceledCtx, nil) {
		t.Fatal("canceled context success should be cached")
	}
	if shouldCacheResult(canceledCtx, testErr) {
		t.Fatal("context cancellation cause should not be cached")
	}
	if !shouldCacheResult(canceledCtx, errors.New("other error")) {
		t.Fatal("non-cause error should be cached")
	}
}

func BenchmarkShouldCacheResult(b *testing.B) {
	activeCtx := b.Context()
	testErr := errors.New("test error")
	canceledCtx, cancel := context.WithCancelCause(context.Background())
	cancel(testErr)

	b.Run("active_success", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = shouldCacheResult(activeCtx, nil)
		}
	})

	b.Run("active_error", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = shouldCacheResult(activeCtx, testErr)
		}
	})

	b.Run("canceled_success", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = shouldCacheResult(canceledCtx, nil)
		}
	})

	b.Run("canceled_cause", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = shouldCacheResult(canceledCtx, testErr)
		}
	})
}
