package cache

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/puzpuzpuz/xsync/v4"
)

type CacheEntry[T any] struct {
	refreshMu sync.Mutex
	cached    atomic.Pointer[cachedValue[T]]
	accessSeq atomic.Uint64
	queuedSeq atomic.Uint64
}

type CachedContextKeyFuncState[T any, K comparable] struct {
	CachedKeyFuncBuilder[T, K]

	entries *xsync.Map[K, *CacheEntry[T]]

	accessSeq atomic.Uint64

	accessLogMu sync.Mutex
	accessLog   []cleanupCandidate[K]
	cleanupLog  []cleanupCandidate[K]
	cleanupMu   sync.Mutex

	janitorIdx int
}

type cleanupCandidate[K comparable] struct {
	key K
	seq uint64
}

func newCachedContextKeyFuncState[T any, K comparable](builder CachedKeyFuncBuilder[T, K]) *CachedContextKeyFuncState[T, K] {
	accessLogCap := initialAccessLogCap(builder.maxEntries)
	state := &CachedContextKeyFuncState[T, K]{
		CachedKeyFuncBuilder: builder,
		entries:              xsync.NewMap[K, *CacheEntry[T]](),
		accessLog:            make([]cleanupCandidate[K], 0, accessLogCap),
	}
	// cleanup is only needed if maxEntries is set
	if state.maxEntries > 0 {
		state.janitorIdx = Janitor.Add(state, state.cleanupInterval)
	}
	return state
}

func (state *CachedContextKeyFuncState[T, K]) checkExpired(cached *cachedValue[T]) bool {
	if cached == nil {
		return true
	}
	if state.ttl == 0 {
		return false
	}
	return time.Now().After(cached.expireAt)
}

func (state *CachedContextKeyFuncState[T, K]) Cleanup() {
	// Cleanup is the single consumer for the access log scratch buffers. Multiple callers may
	// race to request cleanup, but only one pass may swap/sort/restore logs at a time.
	state.cleanupMu.Lock()
	defer state.cleanupMu.Unlock()

	if state.maxEntries == 0 { // should not happen, but just in case
		return
	}

	overflow := state.entries.Size() - state.maxEntries
	if overflow <= 0 {
		return
	}

	log := state.swapAccessLogs()

	current := log[:0]
	for _, candidate := range log {
		entry, ok := state.entries.Load(candidate.key)
		if !ok {
			continue
		}
		// accessSeq is the source of truth for the most recent touch recorded for this entry.
		latestSeq := entry.accessSeq.Load()
		if latestSeq == 0 {
			continue
		}
		if latestSeq == candidate.seq {
			// queuedSeq == candidate.seq means this candidate is still the active cleanup schedule.
			if entry.queuedSeq.Load() == candidate.seq {
				current = append(current, candidate)
			}
			continue
		}
		// Rewrite the queued sequence in a single CAS so touchEntry never observes a transient
		// queuedSeq==0 state for an entry that is already scheduled for cleanup.
		if !entry.queuedSeq.CompareAndSwap(candidate.seq, latestSeq) {
			continue
		}
		current = append(current, cleanupCandidate[K]{key: candidate.key, seq: latestSeq})
	}
	if len(current) == 0 {
		return
	}

	slices.SortFunc(current, func(a, b cleanupCandidate[K]) int {
		return cmp.Compare(a.seq, b.seq)
	})

	survivors := current[:0]
	for _, candidate := range current {
		if overflow <= 0 {
			survivors = append(survivors, candidate)
			continue
		}

		entry, ok := state.entries.Load(candidate.key)
		// If accessSeq moved, a newer touch won the race and this entry must be reconsidered later.
		if !ok || entry.accessSeq.Load() != candidate.seq {
			survivors = append(survivors, candidate)
			continue
		}
		// queuedSeq == candidate.seq means this cleanup pass still owns the scheduled candidate.
		if !entry.queuedSeq.CompareAndSwap(candidate.seq, 0) {
			continue
		}

		// Once the entry is unscheduled, a concurrent touch may enqueue a fresher candidate.
		// Deleting here only races with that enqueue path; a later load either sees no entry or
		// a replacement entry, and stale candidates are ignored by the Load/accessSeq checks above.
		state.entries.Delete(candidate.key)
		overflow--
	}

	state.restoreAccessLog(survivors)
}

func newCacheEntry[T any]() (entry *CacheEntry[T], cancel bool) {
	return &CacheEntry[T]{}, false
}

func (state *CachedContextKeyFuncState[T, K]) callContext(ctx context.Context, key K) (T, error) {
	entry, loaded := state.entries.LoadOrCompute(key, newCacheEntry[T])
	if loaded {
		if cached := entry.cached.Load(); !state.checkExpired(cached) {
			state.touchEntry(key, entry)
			return cached.result, cached.err
		}
	}

	entry.refreshMu.Lock()
	defer entry.refreshMu.Unlock()

	if cached := entry.cached.Load(); !state.checkExpired(cached) {
		state.touchEntry(key, entry)
		return cached.result, cached.err
	}

	result, err := state.execute(ctx, key)
	if shouldCacheResult(ctx, err) {
		cached := &cachedValue[T]{
			result: result,
			err:    err,
		}
		if state.ttl > 0 {
			cached.expireAt = time.Now().Add(state.ttl)
		}
		entry.cached.Store(cached)
	}

	state.touchEntry(key, entry)
	if state.maxEntries > 0 && !loaded && state.entries.Size() > state.maxEntries {
		Janitor.TriggerCleanup(state.janitorIdx)
	}

	return result, err
}

func (state *CachedContextKeyFuncState[T, K]) newBackoff() backoff.BackOff {
	if state.backoffFactory == nil {
		return &backoff.ZeroBackOff{}
	}
	return state.backoffFactory()
}

func (state *CachedContextKeyFuncState[T, K]) execute(ctx context.Context, key K) (T, error) {
	result, err := state.fn(ctx, key)
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

			result, err = state.fn(ctx, key)
			if err == nil {
				return result, nil
			}
		}
	}

	return result, err
}

func (state *CachedContextKeyFuncState[T, K]) nextAccessSeq() uint64 {
	return state.accessSeq.Add(1)
}

// touchEntry records that an entry was recently accessed without taking a global lock.
func (state *CachedContextKeyFuncState[T, K]) touchEntry(key K, entry *CacheEntry[T]) {
	if state.maxEntries == 0 {
		return
	}
	seq := state.nextAccessSeq()
	// Once accessSeq is stored, Cleanup can safely observe this touch before deciding whether to
	// keep, re-queue, or evict the entry.
	entry.accessSeq.Store(seq)
	// queuedSeq==0 means "not currently scheduled". queuedSeq!=0 means "already scheduled at that
	// sequence", and Cleanup will observe accessSeq >= queuedSeq and either keep the candidate or
	// rewrite queuedSeq to the newer accessSeq before reconsidering eviction.
	if entry.queuedSeq.Load() != 0 {
		// Returning early is safe because an already-queued entry will still be examined by Cleanup,
		// which observes the newer accessSeq before eviction or re-queues the candidate if needed.
		return
	}
	// Cleanup may concurrently delete the entry from the map, but that only races with enqueueing:
	// the cleanup pass decides using the queuedSeq/accessSeq pair, and this path only publishes a
	// new candidate when the entry is currently unscheduled.
	state.enqueueCleanupCandidate(key, entry, seq)
}

func (state *CachedContextKeyFuncState[T, K]) enqueueCleanupCandidate(key K, entry *CacheEntry[T], seq uint64) {
	if seq == 0 || !entry.queuedSeq.CompareAndSwap(0, seq) {
		return
	}
	state.accessLogMu.Lock()
	state.accessLog = append(state.accessLog, cleanupCandidate[K]{key: key, seq: seq})
	state.accessLogMu.Unlock()
}

func (state *CachedContextKeyFuncState[T, K]) swapAccessLogs() []cleanupCandidate[K] {
	state.accessLogMu.Lock()
	defer state.accessLogMu.Unlock()

	if cap(state.cleanupLog) < cap(state.accessLog) {
		state.cleanupLog = make([]cleanupCandidate[K], 0, cap(state.accessLog))
	}

	log := state.accessLog
	spare := state.cleanupLog
	state.accessLog = spare[:0]
	state.cleanupLog = log[:0]
	return log
}

func (state *CachedContextKeyFuncState[T, K]) restoreAccessLog(log []cleanupCandidate[K]) {
	if len(log) == 0 {
		return
	}

	state.accessLogMu.Lock()
	state.accessLog = append(state.accessLog, log...)
	state.accessLogMu.Unlock()
}

func initialAccessLogCap(maxEntries int) int {
	if maxEntries <= 0 {
		return 64
	}
	return max(64, maxEntries+min(1024, maxEntries/4))
}
