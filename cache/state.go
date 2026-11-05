package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v7"
)

type cachedValue[T any] struct {
	result     T
	err        error
	expireAt   time.Time
	generation uint64
}

const singleValueCacheKey = "<func>"

type CachedFuncState[T any] struct {
	CachedFuncBuilder[T]

	mu sync.Mutex

	cached atomic.Pointer[cachedValue[T]]
}

// Peek returns the last stored result without waiting or checking its TTL.
// ok is false until a result has been stored. Returned values must not be mutated.
func (state *CachedFuncState[T]) Peek() (result T, ok bool) {
	if cached := state.cached.Load(); cached != nil {
		return cached.result, true
	}
	return result, false
}

// Refresh runs the function regardless of TTL, serialized with other refreshes.
// Only a successful result replaces the cache. It returns the previous and current
// values so callers can observe changes after publication; on error they are equal.
// Unlike a TTL-driven call, each explicit Refresh runs the function, even when
// another refresh just finished. Peek remains nonblocking during the refresh.
func (state *CachedFuncState[T]) Refresh(ctx context.Context) (previous, current T, err error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	previous, _ = state.Peek()
	current, err = state.execute(ctx)
	if err != nil {
		return previous, previous, err
	}
	state.setResult(current, nil)
	return previous, current, nil
}

func (state *CachedFuncState[T]) cachedExpired(cached *cachedValue[T]) bool {
	if cached == nil {
		return true
	}
	if state.ttl == 0 {
		return false
	}
	return time.Now().After(cached.expireAt)
}

func (state *CachedFuncState[T]) checkExpired() bool {
	return state.cachedExpired(state.cached.Load())
}

func (state *CachedFuncState[T]) setResult(result T, err error) {
	cached := &cachedValue[T]{
		result: result,
		err:    err,
	}
	if state.ttl > 0 {
		cached.expireAt = time.Now().Add(state.ttl)
	}
	state.cached.Store(cached)
}

func (state *CachedFuncState[T]) newBackoff() backoff.BackOff {
	if state.backoffFactory == nil {
		return &backoff.ZeroBackOff{}
	}
	return state.backoffFactory()
}

func (state *CachedFuncState[T]) execute(ctx context.Context) (T, error) {
	result, err := state.fn(ctx)
	if err == nil || state.retries == 0 {
		return result, err
	}

	retries := state.retries
	retryBackoff := state.newBackoff()

	for retries > 0 {
		select {
		case <-ctx.Done():
			return result, context.Cause(ctx)
		default:
			retries--
			delay := retryBackoff.NextBackOff()
			if delay == backoff.Stop {
				return result, err
			}
			if err := waitForBackoff(ctx, delay); err != nil {
				return result, err
			}

			result, err = state.fn(ctx)
			if err == nil {
				return result, nil
			}
		}
	}

	return result, err
}

func (state *CachedFuncState[T]) callContext(ctx context.Context) (T, error) {
	if cached := state.cached.Load(); !state.cachedExpired(cached) {
		logCacheHit(singleValueCacheKey, cached.result, cached.err)
		return cached.result, cached.err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	cached := state.cached.Load()
	if !state.cachedExpired(cached) {
		logCacheHit(singleValueCacheKey, cached.result, cached.err)
		return cached.result, cached.err
	}
	if cached != nil {
		logCacheExpiredEntry(singleValueCacheKey, cached.result, cached.err)
	} else {
		logCacheMiss(singleValueCacheKey)
		logCacheUsage(1, 1)
	}

	result, err := state.execute(ctx)
	if shouldCacheResult(ctx, err) {
		state.setResult(result, err)
	}

	return result, err
}
