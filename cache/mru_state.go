package cache

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
)

type CacheEntry[T any] struct {
	sync.Mutex

	result T
	err    error

	expireAt time.Time
	element  *list.Element // linked list element for this entry
}

type CachedContextKeyFuncState[T any, K comparable] struct {
	CachedKeyFuncBuilder[T, K]

	entries *xsync.Map[K, *CacheEntry[T]]

	mru     *list.List // keys ordered by access time, from most recent to least recent
	mruLock sync.Mutex

	janitorIdx int
}

func newCachedContextKeyFuncState[T any, K comparable](builder CachedKeyFuncBuilder[T, K]) *CachedContextKeyFuncState[T, K] {
	state := &CachedContextKeyFuncState[T, K]{
		CachedKeyFuncBuilder: builder,
		entries:              xsync.NewMap[K, *CacheEntry[T]](),
		mru:                  list.New(),
	}
	// cleanup is only needed if maxEntries is set
	if state.maxEntries > 0 {
		state.janitorIdx = Janitor.Add(state, state.cleanupInterval)
	}
	return state
}

func (state *CachedContextKeyFuncState[T, K]) checkExpired(entry *CacheEntry[T]) bool {
	if state.ttl == 0 {
		return false
	}

	// should not happen
	//
	// If expireAt is zero time (uninitialized), consider it expired
	// if entry.expireAt.IsZero() {
	// 	return true
	// }
	return time.Now().After(entry.expireAt)
}

func (state *CachedContextKeyFuncState[T, K]) Cleanup() {
	if state.maxEntries == 0 { // should not happen, but just in case
		return
	}
	if state.entries.Size() <= state.maxEntries {
		return
	}

	state.mruLock.Lock()
	defer state.mruLock.Unlock()
	for state.mru.Len() > state.maxEntries {
		// remove the oldest entry
		key := state.mru.Remove(state.mru.Back()).(K)
		state.entries.Delete(key)
	}
}

func newCacheEntry[T any]() (entry *CacheEntry[T], cancel bool) {
	return &CacheEntry[T]{}, false
}

func (state *CachedContextKeyFuncState[T, K]) callContext(ctx context.Context, key K) (T, error) {
	entry, loaded := state.entries.LoadOrCompute(key, newCacheEntry[T])
	entry.Lock()

	if loaded {
		// After acquiring the lock, check if the entry is valid.
		if !state.checkExpired(entry) {
			entry.Unlock()
			return entry.result, entry.err
		}
	} else { // This is a new entry we just created.
		if state.maxEntries > 0 {
			// move once now to front of time list (most recently used)
			entry.element = state.moveToFront(key, entry.element)
			// it might be a long running tasks, do it again at the end
			defer state.moveToFront(key, entry.element)
		}
		if state.maxEntries > 0 && state.entries.Size() > state.maxEntries {
			defer Janitor.TriggerCleanup(state.janitorIdx)
		}
	}

	// We are the one to compute (or re-compute) the value.
	defer entry.Unlock()

	// Compute the result
	entry.result, entry.err = state.fn(ctx, key)
	if entry.err != nil {
		retries := state.retries

	retriesLoop:
		for retries > 0 {
			select {
			case <-ctx.Done():
				return entry.result, context.Cause(ctx)
			default:
				retries--
				time.Sleep(state.backoff.NextBackOff())

				entry.result, entry.err = state.fn(ctx, key)
				if entry.err == nil {
					state.backoff.Reset()
					break retriesLoop
				}
			}
		}
	}

	// Update expiration time
	if state.ttl > 0 {
		entry.expireAt = time.Now().Add(state.ttl)
	}
	return entry.result, entry.err
}

// moveToFront moves the entry to the front of the time list (most recently used)
func (state *CachedContextKeyFuncState[T, K]) moveToFront(k K, elem *list.Element) *list.Element {
	state.mruLock.Lock()
	defer state.mruLock.Unlock()
	if elem != nil {
		state.mru.MoveToFront(elem)
		return elem
	} else {
		return state.mru.PushFront(k)
	}
}
