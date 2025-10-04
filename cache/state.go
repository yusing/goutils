package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type CachedFuncState[T any] struct {
	CachedFuncBuilder[T]
	sync.Mutex

	result T
	err    error

	cached uintptr
	last   time.Time
}

func (state *CachedFuncState[T]) checkExpired() bool {
	if state.ttl == 0 {
		return false
	}
	if state.last.IsZero() {
		return true
	}
	return time.Since(state.last) > state.ttl
}

func (state *CachedFuncState[T]) setResult(result T, err error) {
	state.cached = 1
	state.result = result
	state.err = err
	if state.ttl > 0 {
		state.last = time.Now()
	}
}

func (state *CachedFuncState[T]) callContext(ctx context.Context) (T, error) {
	// atomic check
	if atomic.LoadUintptr(&state.cached) == 1 {
		// return early if the result is cached and not expired
		if !state.checkExpired() {
			return state.result, state.err
		}
	}

	// lock and check again, no need atomic
	state.Lock()
	defer state.Unlock()
	if state.cached == 1 {
		// return early if the result is cached and not expired
		if !state.checkExpired() {
			return state.result, state.err
		}
	}

	state.result, state.err = state.fn(ctx)
	retries := state.retries

retriesLoop:
	for retries > 0 {
		select {
		case <-ctx.Done():
			return state.result, context.Cause(ctx)
		default:
			retries--
			time.Sleep(state.backoff.NextBackOff())

			state.result, state.err = state.fn(ctx)
			if state.err == nil {
				state.backoff.Reset()
				break retriesLoop
			}
		}
	}

	state.setResult(state.result, state.err)
	return state.result, state.err
}
