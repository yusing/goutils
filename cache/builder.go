package cache

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v5"
)

type (
	CachedContextFunc[T any]                  func(ctx context.Context) (T, error)
	CachedContextKeyFunc[T any, K comparable] func(ctx context.Context, key K) (T, error)
)

type CachedFuncConfig struct {
	retries        int
	backoff        backoff.BackOff
	backoffFactory func() backoff.BackOff
	ttl            time.Duration
}

type CachedFuncBuilder[T any] struct {
	CachedFuncConfig

	fn CachedContextFunc[T]
}

type CachedKeyFuncBuilder[T any, K comparable] struct {
	CachedFuncConfig

	maxEntries      int
	cleanupInterval time.Duration

	fn CachedContextKeyFunc[T, K]
}

// NewFunc creates a new CachedFuncBuilder with the given
// CachedContextFunc).
func NewFunc[T any](fn CachedContextFunc[T]) CachedFuncBuilder[T] {
	return CachedFuncBuilder[T]{fn: fn}
}

// NewKeyFunc creates a new CachedKeyFuncBuilder with the given
// CachedContextKeyFunc and default options (15 seconds cleanup interval).
func NewKeyFunc[T any, K comparable](fn CachedContextKeyFunc[T, K]) CachedKeyFuncBuilder[T, K] {
	return CachedKeyFuncBuilder[T, K]{fn: fn, cleanupInterval: 15 * time.Second}
}

func (builder CachedFuncBuilder[T]) WithRetriesExponentialBackoff(retries int) CachedFuncBuilder[T] {
	builder.retries = retries
	builder.backoffFactory = func() backoff.BackOff {
		return backoff.NewExponentialBackOff()
	}
	return builder
}

func (builder CachedKeyFuncBuilder[T, K]) WithRetriesExponentialBackoff(retries int) CachedKeyFuncBuilder[T, K] {
	builder.retries = retries
	builder.backoffFactory = func() backoff.BackOff {
		return backoff.NewExponentialBackOff()
	}
	return builder
}

func (builder CachedFuncBuilder[T]) WithRetriesConstantBackoff(retries int, interval time.Duration) CachedFuncBuilder[T] {
	builder.retries = retries
	builder.backoffFactory = func() backoff.BackOff {
		return backoff.NewConstantBackOff(interval)
	}
	builder.backoff = builder.backoffFactory()
	return builder
}

func (builder CachedKeyFuncBuilder[T, K]) WithRetriesConstantBackoff(retries int, interval time.Duration) CachedKeyFuncBuilder[T, K] {
	builder.retries = retries
	builder.backoffFactory = func() backoff.BackOff {
		return backoff.NewConstantBackOff(interval)
	}
	builder.backoff = builder.backoffFactory()
	return builder
}

func (builder CachedFuncBuilder[T]) WithRetriesZeroBackoff(retries int) CachedFuncBuilder[T] {
	builder.retries = retries
	builder.backoffFactory = func() backoff.BackOff {
		return &backoff.ZeroBackOff{}
	}
	builder.backoff = builder.backoffFactory()
	return builder
}

func (builder CachedKeyFuncBuilder[T, K]) WithRetriesZeroBackoff(retries int) CachedKeyFuncBuilder[T, K] {
	builder.retries = retries
	builder.backoffFactory = func() backoff.BackOff {
		return &backoff.ZeroBackOff{}
	}
	builder.backoff = builder.backoffFactory()
	return builder
}

func (builder CachedFuncBuilder[T]) WithTTL(ttl time.Duration) CachedFuncBuilder[T] {
	builder.ttl = ttl
	return builder
}

func (builder CachedKeyFuncBuilder[T, K]) WithTTL(ttl time.Duration) CachedKeyFuncBuilder[T, K] {
	builder.ttl = ttl
	return builder
}

// WithMaxEntries configures new CachedKeyFuncBuilder instance with
// the given maxEntries.
func (builder CachedKeyFuncBuilder[T, K]) WithMaxEntries(maxEntries int) CachedKeyFuncBuilder[T, K] {
	builder.maxEntries = maxEntries
	return builder
}

// WithCleanupInterval configures new CachedKeyFuncBuilder instance with
// the given cleanupInterval. MaxEntries must be set for this to have
// any effect.
func (builder CachedKeyFuncBuilder[T, K]) WithCleanupInterval(cleanupInterval time.Duration) CachedKeyFuncBuilder[T, K] {
	if cleanupInterval < time.Second {
		cleanupInterval = time.Second
	}
	builder.cleanupInterval = cleanupInterval
	return builder
}

func (builder CachedFuncBuilder[T]) Build() CachedContextFunc[T] {
	state := CachedFuncState[T]{
		CachedFuncBuilder: builder,
	}
	return state.callContext
}

func (builder CachedKeyFuncBuilder[T, K]) Build() CachedContextKeyFunc[T, K] {
	state := newCachedContextKeyFuncState(builder)
	return state.callContext
}
