package cache

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v5"
)

type CachedContextFunc[T any] func(ctx context.Context) (T, error)

type CachedFuncBuilder[T any] struct {
	fn      CachedContextFunc[T]
	retries int
	backoff backoff.BackOff
	ttl     time.Duration
}

func NewFunc[T any](fn CachedContextFunc[T]) CachedFuncBuilder[T] {
	return CachedFuncBuilder[T]{fn: fn}
}

func (builder CachedFuncBuilder[T]) WithRetriesExponentialBackoff(retries int) CachedFuncBuilder[T] {
	builder.retries = retries
	builder.backoff = backoff.NewExponentialBackOff()
	return builder
}

func (builder CachedFuncBuilder[T]) WithRetriesConstantBackoff(retries int, interval time.Duration) CachedFuncBuilder[T] {
	builder.retries = retries
	builder.backoff = backoff.NewConstantBackOff(interval)
	return builder
}

func (builder CachedFuncBuilder[T]) WithRetriesZeroBackoff(retries int) CachedFuncBuilder[T] {
	builder.retries = retries
	builder.backoff = &backoff.ZeroBackOff{}
	return builder
}

func (builder CachedFuncBuilder[T]) WithTTL(ttl time.Duration) CachedFuncBuilder[T] {
	builder.ttl = ttl
	return builder
}

func (builder CachedFuncBuilder[T]) Build() CachedContextFunc[T] {
	state := CachedFuncState[T]{
		CachedFuncBuilder: builder,
	}
	return state.callContext
}
