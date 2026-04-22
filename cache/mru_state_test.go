package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedContextKeyFuncState_BasicCaching(t *testing.T) {
	callCounts := make(map[int]int)
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		return "result-" + string(rune(key+'0')), nil
	}

	cachedFunc := NewKeyFunc(fn).Build()
	ctx := t.Context()

	// First call for key 1 should execute the function
	result, err := cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "result-1", result)
	assert.Equal(t, 1, callCounts[1])

	// Second call for same key should use cached result
	result, err = cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "result-1", result)
	assert.Equal(t, 1, callCounts[1]) // Still 1, function not called again

	// Call for different key should execute function again
	result, err = cachedFunc(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, "result-2", result)
	assert.Equal(t, 1, callCounts[2])
}

func TestCachedContextKeyFuncState_WithError(t *testing.T) {
	callCounts := make(map[int]int)
	testErr := errors.New("test error")
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		return "", testErr
	}

	cachedFunc := NewKeyFunc(fn).Build()
	ctx := t.Context()

	// First call should execute and return error
	result, err := cachedFunc(ctx, 1)
	assert.Error(t, err)
	assert.ErrorIs(t, err, testErr)
	assert.Empty(t, result)
	assert.Equal(t, 1, callCounts[1])

	// Second call should use cached error
	result, err = cachedFunc(ctx, 1)
	assert.Error(t, err)
	assert.ErrorIs(t, err, testErr)
	assert.Empty(t, result)
	assert.Equal(t, 1, callCounts[1]) // Still 1, function not called again
}

func TestCachedContextKeyFuncState_WithTTL(t *testing.T) {
	callCounts := make(map[int]int)
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		return "result-" + string(rune(key+'0')), nil
	}

	ttl := 100 * time.Millisecond
	cachedFunc := NewKeyFunc(fn).WithTTL(ttl).Build()
	ctx := t.Context()

	// First call
	result, err := cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "result-1", result)
	assert.Equal(t, 1, callCounts[1])

	// Second call within TTL should use cache
	result, err = cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "result-1", result)
	assert.Equal(t, 1, callCounts[1])

	// Wait for TTL to expire
	time.Sleep(ttl + 10*time.Millisecond)

	// Third call after TTL should execute function again
	result, err = cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "result-1", result)
	assert.Equal(t, 2, callCounts[1])
}

func TestCachedContextKeyFuncState_WithZeroTTL(t *testing.T) {
	callCounts := make(map[int]int)
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		return "result-" + string(rune(key+'0')), nil
	}

	cachedFunc := NewKeyFunc(fn).WithTTL(0).Build()
	ctx := t.Context()

	// With TTL=0, cache should never expire
	result, err := cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "result-1", result)
	assert.Equal(t, 1, callCounts[1])

	// Should always use cache
	result, err = cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "result-1", result)
	assert.Equal(t, 1, callCounts[1])
}

func TestCachedContextKeyFuncState_WithRetries(t *testing.T) {
	callCounts := make(map[int]int)
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		if callCounts[key] < 3 {
			return "", errors.New("temporary error")
		}
		return "success", nil
	}

	cachedFunc := NewKeyFunc(fn).WithRetriesZeroBackoff(3).Build()

	result, err := cachedFunc(t.Context(), 1)
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 3, callCounts[1]) // Called 3 times (1 initial + 2 retries)
}

func TestCachedContextKeyFuncState_RetryExhausted(t *testing.T) {
	callCounts := make(map[int]int)
	testErr := errors.New("persistent error")
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		return "", testErr
	}

	cachedFunc := NewKeyFunc(fn).WithRetriesZeroBackoff(2).Build()

	result, err := cachedFunc(t.Context(), 1)
	assert.Error(t, err)
	assert.ErrorIs(t, err, testErr)
	assert.Empty(t, result)
	assert.Equal(t, 3, callCounts[1]) // Called 3 times (1 initial + 2 retries)
}

func TestCachedContextKeyFuncState_ContextCancellation(t *testing.T) {
	callCounts := make(map[int]int)
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			// Always return error to force retries
			return "", errors.New("persistent error")
		}
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	cachedFunc := NewKeyFunc(fn).WithRetriesConstantBackoff(10, 10*time.Millisecond).Build()

	result, err := cachedFunc(ctx, 1)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, result)
	// Function should have been called multiple times before context cancellation
	assert.GreaterOrEqual(t, callCounts[1], 1)
}

func TestCachedContextKeyFuncState_ContextCancellationDuringBackoffSleep(t *testing.T) {
	fnStarted := make(chan struct{})
	fn := func(ctx context.Context, key int) (string, error) {
		select {
		case <-fnStarted:
		default:
			close(fnStarted)
		}
		return "", errors.New("persistent error")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cachedFunc := NewKeyFunc(fn).WithRetriesConstantBackoff(10, 200*time.Millisecond).Build()

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := cachedFunc(ctx, 1)
		done <- err
	}()

	<-fnStarted
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
		assert.Less(t, time.Since(start), 150*time.Millisecond)
	case <-time.After(time.Second):
		t.Fatal("cached key function did not return promptly after cancellation")
	}
}

func TestCachedContextKeyFuncState_CanceledRetryDoesNotPoisonCache(t *testing.T) {
	var callCount atomic.Int32
	fnStarted := make(chan struct{})
	fn := func(ctx context.Context, key int) (string, error) {
		if callCount.Add(1) == 1 {
			close(fnStarted)
			return "", errors.New("transient error")
		}
		return "success", nil
	}

	cachedFunc := NewKeyFunc(fn).WithRetriesConstantBackoff(1, 200*time.Millisecond).Build()
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)
	go func() {
		_, err := cachedFunc(ctx, 1)
		done <- err
	}()

	<-fnStarted
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cached key function did not return promptly after cancellation")
	}

	result, err := cachedFunc(t.Context(), 1)
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, int32(2), callCount.Load())
}

func TestCachedContextKeyFuncState_BackoffStopStopsRetries(t *testing.T) {
	testErr := errors.New("persistent error")
	var callCount atomic.Int32
	state := &CachedContextKeyFuncState[string, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[string, int]{
			CachedFuncConfig: CachedFuncConfig{
				retries: 3,
				backoffFactory: func() backoff.BackOff {
					return &stopBackOff{}
				},
			},
			fn: func(ctx context.Context, key int) (string, error) {
				callCount.Add(1)
				return "", testErr
			},
		},
	}

	result, err := state.execute(t.Context(), 1)
	assert.ErrorIs(t, err, testErr)
	assert.Empty(t, result)
	assert.Equal(t, int32(1), callCount.Load())
}

func TestCachedContextKeyFuncState_TTLWaitsForPublishedInFlightEntry(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	state := &CachedContextKeyFuncState[int, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[int, int]{
			CachedFuncConfig: CachedFuncConfig{ttl: time.Second},
			fn: func(ctx context.Context, key int) (int, error) {
				calls.Add(1)
				return 99, nil
			},
		},
		entries: xsync.NewMap[int, *CacheEntry[int]](),
	}

	entry := &CacheEntry[int]{}
	entry.refreshMu.Lock()
	state.entries.Store(1, entry)

	resultCh := make(chan int, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := state.callContext(t.Context(), 1)
		resultCh <- result
		errCh <- err
	}()

	select {
	case result := <-resultCh:
		t.Fatalf("call returned before published entry became ready: got %d", result)
	case err := <-errCh:
		t.Fatalf("call returned before published entry became ready: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	entry.cached.Store(&cachedValue[int]{
		result:   42,
		expireAt: time.Now().Add(time.Second),
	})
	entry.refreshMu.Unlock()

	assert.NoError(t, <-errCh)
	assert.Equal(t, 42, <-resultCh)
	assert.Zero(t, calls.Load(), "waiter should not recompute while the first result is still in flight")
}

func TestCachedContextKeyFuncState_ColdKeyConcurrentNeverReturnsIntZero(t *testing.T) {
	t.Parallel()

	const (
		rounds = 40
		n      = 64
		key    = 3
		want   = key * 7
	)

	for round := range rounds {
		var calls atomic.Int32
		fn := func(ctx context.Context, k int) (int, error) {
			calls.Add(1)
			time.Sleep(2 * time.Millisecond)
			return k * 7, nil
		}

		cached := NewKeyFunc(fn).Build()
		results := make([]int, n)
		errs := make([]error, n)

		var wg sync.WaitGroup
		for i := range n {
			i := i
			wg.Go(func() {
				results[i], errs[i] = cached(t.Context(), key)
			})
		}
		wg.Wait()

		for i := range n {
			assert.NoError(t, errs[i])
			assert.Equalf(t, want, results[i], "round %d goroutine %d returned the zero value or a stale result", round, i)
		}
		assert.Equalf(t, int32(1), calls.Load(), "round %d should compute the cold key once", round)
	}
}

func TestCachedContextKeyFuncState_ConcurrentAccess(t *testing.T) {
	callCounts := make(map[int]int)
	callCountMutex := sync.Mutex{}
	fn := func(ctx context.Context, key int) (int, error) {
		callCountMutex.Lock()
		callCounts[key]++
		callCountMutex.Unlock()
		time.Sleep(50 * time.Millisecond) // Simulate work
		return key * 42, nil
	}

	cachedFunc := NewKeyFunc(fn).Build()
	ctx := t.Context()

	var wg sync.WaitGroup
	results := make([]int, 20)
	errs := make([]error, 20)

	// Launch multiple goroutines concurrently for the same key
	for i := range 20 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = cachedFunc(ctx, 5)
		}(i)
	}

	wg.Wait()

	// All calls should have the same result
	for i := range 20 {
		assert.NoError(t, errs[i])
		assert.Equal(t, 210, results[i]) // 5 * 42
	}

	// Function should only be called once due to proper locking
	assert.Equal(t, 1, callCounts[5])
}

func TestCachedContextKeyFuncState_ConcurrentAccessAfterTTLExpiry(t *testing.T) {
	var callCount atomic.Int32
	recomputeStarted := make(chan struct{})
	releaseRecompute := make(chan struct{})

	fn := func(ctx context.Context, key int) (int, error) {
		switch callCount.Add(1) {
		case 1:
			return key, nil
		default:
			select {
			case <-recomputeStarted:
			default:
				close(recomputeStarted)
			}
			<-releaseRecompute
			return key * 10, nil
		}
	}

	ttl := 20 * time.Millisecond
	cachedFunc := NewKeyFunc(fn).WithTTL(ttl).Build()
	ctx := t.Context()

	result, err := cachedFunc(ctx, 7)
	assert.NoError(t, err)
	assert.Equal(t, 7, result)

	time.Sleep(ttl + 10*time.Millisecond)

	var wg sync.WaitGroup
	results := make([]int, 12)
	errs := make([]error, 12)
	for i := range len(results) {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = cachedFunc(ctx, 7)
		}(i)
	}

	<-recomputeStarted
	assert.Equal(t, int32(2), callCount.Load())
	close(releaseRecompute)
	wg.Wait()

	for i := range len(results) {
		assert.NoError(t, errs[i])
		assert.Equal(t, 70, results[i])
	}
	assert.Equal(t, int32(2), callCount.Load())
}

func TestCachedContextKeyFuncState_ErrorExpiresAndRefreshes(t *testing.T) {
	var callCount atomic.Int32
	testErr := errors.New("temporary error")
	fn := func(ctx context.Context, key int) (string, error) {
		if callCount.Add(1) == 1 {
			return "", testErr
		}
		return "success", nil
	}

	ttl := 20 * time.Millisecond
	cachedFunc := NewKeyFunc(fn).WithTTL(ttl).Build()
	ctx := t.Context()

	result, err := cachedFunc(ctx, 1)
	assert.ErrorIs(t, err, testErr)
	assert.Empty(t, result)

	result, err = cachedFunc(ctx, 1)
	assert.ErrorIs(t, err, testErr)
	assert.Empty(t, result)
	assert.Equal(t, int32(1), callCount.Load())

	time.Sleep(ttl + 10*time.Millisecond)

	result, err = cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, int32(2), callCount.Load())
}

func TestCachedContextKeyFuncState_ConcurrentAccessMultipleKeys(t *testing.T) {
	callCounts := make(map[int]int)
	callCountMutex := sync.Mutex{}
	fn := func(ctx context.Context, key int) (int, error) {
		callCountMutex.Lock()
		callCounts[key]++
		callCountMutex.Unlock()
		time.Sleep(30 * time.Millisecond) // Simulate work
		return key * 10, nil
	}

	cachedFunc := NewKeyFunc(fn).Build()
	ctx := t.Context()

	var wg sync.WaitGroup
	results := make([]int, 15)
	errs := make([]error, 15)
	keys := []int{1, 2, 3}

	// Launch multiple goroutines concurrently for different keys
	for i := range 15 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			key := keys[index%3]
			results[index], errs[index] = cachedFunc(ctx, key)
		}(i)
	}

	wg.Wait()

	// All calls should have correct results based on their keys
	for i := range 15 {
		assert.NoError(t, errs[i])
		expected := keys[i%3] * 10
		assert.Equal(t, expected, results[i])
	}

	// Each unique key should only be called once
	assert.Equal(t, 1, callCounts[1])
	assert.Equal(t, 1, callCounts[2])
	assert.Equal(t, 1, callCounts[3])
}

func TestCachedContextKeyFuncState_MaxEntries(t *testing.T) {
	callCounts := make(map[int]int)
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		return "result-" + string(rune(key+'0')), nil
	}

	maxEntries := 3
	cachedFunc := NewKeyFunc(fn).WithMaxEntries(maxEntries).Build()
	ctx := t.Context()

	// Add entries up to maxEntries
	for i := 1; i <= maxEntries; i++ {
		result, err := cachedFunc(ctx, i)
		assert.NoError(t, err)
		assert.Equal(t, "result-"+string(rune(i+'0')), result)
		assert.Equal(t, 1, callCounts[i])
	}

	// Access all entries to refresh them
	for i := 1; i <= maxEntries; i++ {
		result, err := cachedFunc(ctx, i)
		assert.NoError(t, err)
		assert.Equal(t, "result-"+string(rune(i+'0')), result)
		assert.Equal(t, 1, callCounts[i]) // Should still be cached
	}

	// Add one more entry (should trigger eviction)
	result, err := cachedFunc(ctx, 4)
	assert.NoError(t, err)
	assert.Equal(t, "result-4", result)
	assert.Equal(t, 1, callCounts[4])

	// Allow janitor to run
	time.Sleep(50 * time.Millisecond)

	// The least recently used entry (key 1) should have been evicted
	// We can't directly test eviction here without accessing internal state,
	// but we can test that the cache continues to work properly
	result, err = cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "result-1", result)
	assert.Equal(t, 2, callCounts[1]) // Should be called again due to eviction
}

func TestCachedContextKeyFuncState_DifferentTypes(t *testing.T) {
	// Test with string keys and int values
	intFn := func(ctx context.Context, key string) (int, error) {
		switch key {
		case "one":
			return 1, nil
		case "two":
			return 2, nil
		default:
			return 0, errors.New("unknown key")
		}
	}
	ctx := t.Context()

	intCached := NewKeyFunc(intFn).Build()
	result, err := intCached(ctx, "one")
	assert.NoError(t, err)
	assert.Equal(t, 1, result)

	// Test with struct keys and values
	type PersonKey struct {
		ID   int
		Name string
	}
	type Person struct {
		Age  int
		City string
	}
	personFn := func(ctx context.Context, key PersonKey) (Person, error) {
		return Person{Age: 30 + key.ID, City: key.Name}, nil
	}
	personCached := NewKeyFunc(personFn).Build()
	person, err := personCached(ctx, PersonKey{ID: 5, Name: "NYC"})
	assert.NoError(t, err)
	assert.Equal(t, Person{Age: 35, City: "NYC"}, person)
}

func TestCachedContextKeyFuncState_BackoffReset(t *testing.T) {
	callCounts := make(map[int]int)
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		if callCounts[key] == 1 {
			return "", errors.New("first error")
		}
		return "success", nil
	}

	cachedFunc := NewKeyFunc(fn).WithRetriesExponentialBackoff(2).Build()
	ctx := t.Context()

	// First call should fail once then succeed
	result, err := cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 2, callCounts[1])

	// Reset call count for second test
	callCounts[1] = 0

	// Second call should use cached result
	result, err = cachedFunc(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 0, callCounts[1]) // Function not called again
}

func TestCachedContextKeyFuncState_ExpirationLogic(t *testing.T) {
	fn := func(ctx context.Context, key int) (string, error) {
		return "result", nil
	}

	state := &CachedContextKeyFuncState[string, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[string, int]{
			CachedFuncConfig: CachedFuncConfig{
				ttl: 100 * time.Millisecond,
			},
			fn: fn,
		},
		entries: xsync.NewMap[int, *CacheEntry[string]](),
	}

	// Test checkExpired when never set
	assert.True(t, state.checkExpired(nil))

	entry := &CacheEntry[string]{}
	entry.cached.Store(&cachedValue[string]{result: "test"})

	// Test checkExpired after setting expireAt
	entry.cached.Store(&cachedValue[string]{result: "test", expireAt: time.Now().Add(200 * time.Millisecond)})
	assert.False(t, state.checkExpired(entry.cached.Load()))

	// Test checkExpired after TTL expires
	entry.cached.Store(&cachedValue[string]{result: "test", expireAt: time.Now().Add(-10 * time.Millisecond)})
	assert.True(t, state.checkExpired(entry.cached.Load()))

	// Test with zero TTL (should never expire)
	stateZeroTTL := &CachedContextKeyFuncState[string, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[string, int]{
			CachedFuncConfig: CachedFuncConfig{
				ttl: 0,
			},
			fn: fn,
		},
		entries: xsync.NewMap[int, *CacheEntry[string]](),
	}

	entryZeroTTL := &CacheEntry[string]{}
	entryZeroTTL.cached.Store(&cachedValue[string]{result: "test", expireAt: time.Now().Add(-10 * time.Millisecond)})
	assert.False(t, stateZeroTTL.checkExpired(entryZeroTTL.cached.Load())) // Should be false with TTL=0
}

func TestCachedContextKeyFuncState_Cleanup(t *testing.T) {
	callCounts := make(map[int]int)
	fn := func(ctx context.Context, key int) (string, error) {
		callCounts[key]++
		return "result-" + string(rune(key+'0')), nil
	}
	ctx := t.Context()

	maxEntries := 2
	iters := 10
	cachedFunc := NewKeyFunc(fn).WithMaxEntries(maxEntries).Build()

	// Add more entries than maxEntries to trigger cleanup
	for range maxEntries + 1 {
		for i := range iters {
			result, err := cachedFunc(ctx, i)
			assert.NoError(t, err)
			assert.Equal(t, "result-"+string(rune(i+'0')), result)
		}
		// wait for janitor to run
		time.Sleep(10 * time.Millisecond)
	}

	// Some entries should have been recomputed due to eviction
	totalCalls := 0
	for _, count := range callCounts {
		totalCalls += count
	}
	assert.Greater(t, totalCalls, iters) // Should be more than 5 due to evictions
}

func TestCachedContextKeyFuncState_CleanupTrimsOverflowToMostRecentEntries(t *testing.T) {
	state := &CachedContextKeyFuncState[string, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[string, int]{
			maxEntries: 2,
		},
		entries:   xsync.NewMap[int, *CacheEntry[string]](),
		accessLog: make([]cleanupCandidate[int], 0, 8),
	}

	for _, key := range []int{1, 2, 3, 2} {
		entry, _ := state.entries.LoadOrCompute(key, newCacheEntry[string])
		entry.cached.Store(&cachedValue[string]{result: "value"})
		state.touchEntry(key, entry)
	}

	state.Cleanup()

	assert.Equal(t, 2, state.entries.Size())

	_, has1 := state.entries.Load(1)
	_, has2 := state.entries.Load(2)
	_, has3 := state.entries.Load(3)
	assert.False(t, has1)
	assert.True(t, has2)
	assert.True(t, has3)
}

func TestCachedContextKeyFuncState_SwapAndRestoreAccessLogPreservesCandidates(t *testing.T) {
	state := &CachedContextKeyFuncState[string, int]{
		accessLog: []cleanupCandidate[int]{
			{key: 1, seq: 1},
			{key: 2, seq: 2},
		},
		cleanupLog: make([]cleanupCandidate[int], 0, 1),
	}

	swapped := state.swapAccessLogs()
	require.Equal(t, []cleanupCandidate[int]{
		{key: 1, seq: 1},
		{key: 2, seq: 2},
	}, swapped)
	require.Empty(t, state.accessLog)

	state.accessLog = append(state.accessLog, cleanupCandidate[int]{key: 9, seq: 9})
	state.restoreAccessLog(swapped[1:])

	require.Equal(t, []cleanupCandidate[int]{
		{key: 9, seq: 9},
		{key: 2, seq: 2},
	}, state.accessLog)
}

func TestCachedContextKeyFuncState_CleanupConcurrentContentionStress(t *testing.T) {
	const (
		maxEntries = 32
		totalKeys  = 96
		iterations = 400
	)

	state := &CachedContextKeyFuncState[int, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[int, int]{
			maxEntries: maxEntries,
		},
		entries:    xsync.NewMap[int, *CacheEntry[int]](),
		accessLog:  make([]cleanupCandidate[int], 0, initialAccessLogCap(maxEntries)),
		cleanupLog: make([]cleanupCandidate[int], 0, initialAccessLogCap(maxEntries)),
	}

	for key := range totalKeys {
		entry := &CacheEntry[int]{}
		entry.cached.Store(&cachedValue[int]{result: key})
		state.entries.Store(key, entry)
		state.touchEntry(key, entry)
	}

	var wg sync.WaitGroup
	for worker := range 8 {
		worker := worker
		wg.Go(func() {
			for i := range iterations {
				key := (worker*iterations + i) % totalKeys
				switch worker % 4 {
				case 0:
					entry, _ := state.entries.LoadOrCompute(key, newCacheEntry[int])
					entry.cached.Store(&cachedValue[int]{result: key})
					state.touchEntry(key, entry)
				case 1:
					if entry, ok := state.entries.Load(key); ok {
						_ = entry.cached.Load()
						state.touchEntry(key, entry)
					}
				case 2:
					state.entries.Delete(key)
					entry := &CacheEntry[int]{}
					entry.cached.Store(&cachedValue[int]{result: key})
					state.entries.Store(key, entry)
					state.touchEntry(key, entry)
				default:
					state.Cleanup()
				}
			}
		})
	}
	wg.Wait()

	for range 4 {
		state.Cleanup()
	}

	assert.LessOrEqual(t, state.entries.Size(), maxEntries)

	log := state.swapAccessLogs()
	defer state.restoreAccessLog(log)

	for key := range totalKeys {
		entry, ok := state.entries.Load(key)
		if !ok {
			continue
		}
		accessSeq := entry.accessSeq.Load()
		queuedSeq := entry.queuedSeq.Load()
		assert.NotZero(t, accessSeq)
		assert.GreaterOrEqual(t, accessSeq, queuedSeq)
	}

	for _, candidate := range log {
		entry, ok := state.entries.Load(candidate.key)
		if !ok {
			continue
		}
		assert.GreaterOrEqual(t, entry.accessSeq.Load(), candidate.seq)
	}
}

func TestCachedContextKeyFuncState_CleanupConcurrentTouchStress(t *testing.T) {
	const attempts = 200

	for attempt := range attempts {
		state := &CachedContextKeyFuncState[int, int]{
			CachedKeyFuncBuilder: CachedKeyFuncBuilder[int, int]{
				maxEntries: 1,
			},
			entries:    xsync.NewMap[int, *CacheEntry[int]](),
			accessLog:  make([]cleanupCandidate[int], 0, 32),
			cleanupLog: make([]cleanupCandidate[int], 0, 32),
		}

		hotEntry := &CacheEntry[int]{}
		hotEntry.cached.Store(&cachedValue[int]{result: 1})
		coldEntry := &CacheEntry[int]{}
		coldEntry.cached.Store(&cachedValue[int]{result: 2})

		state.entries.Store(1, hotEntry)
		state.entries.Store(2, coldEntry)
		state.touchEntry(1, hotEntry)
		state.touchEntry(2, coldEntry)

		var wg sync.WaitGroup
		wg.Go(func() {
			for range 256 {
				state.touchEntry(1, hotEntry)
			}
		})
		wg.Go(func() {
			state.Cleanup()
		})
		wg.Wait()

		state.Cleanup()

		accessSeq := hotEntry.accessSeq.Load()
		queuedSeq := hotEntry.queuedSeq.Load()
		require.NotZerof(t, accessSeq, "missing hot access sequence on attempt %d", attempt)
		require.GreaterOrEqualf(t, accessSeq, queuedSeq, "invalid hot entry sequencing on attempt %d", attempt)
		require.LessOrEqualf(t, state.entries.Size(), 1, "cleanup left overflow behind on attempt %d", attempt)

		log := state.swapAccessLogs()
		for _, candidate := range log {
			if candidate.key != 1 {
				continue
			}
			require.GreaterOrEqualf(t, accessSeq, candidate.seq, "stale hot candidate overtook latest access on attempt %d", attempt)
		}
		state.restoreAccessLog(log)
	}
}

func BenchmarkCachedKeyFunc_NoCache(b *testing.B) {
	fn := func(ctx context.Context, key int) (string, error) {
		time.Sleep(time.Nanosecond)
		return "result-" + string(rune(key+'0')), nil
	}
	ctx := b.Context()

	b.ResetTimer()
	for b.Loop() {
		cachedFunc := NewKeyFunc(fn).Build()
		_, _ = cachedFunc(ctx, 1)
	}
}

func BenchmarkCachedKeyFunc_WithCache(b *testing.B) {
	callCount := 0
	fn := func(ctx context.Context, key int) (string, error) {
		callCount++
		time.Sleep(time.Nanosecond)
		return "result-" + string(rune(key+'0')), nil
	}
	ctx := b.Context()

	cachedFunc := NewKeyFunc(fn).Build()

	b.ResetTimer()
	for b.Loop() {
		_, _ = cachedFunc(ctx, 1)
	}

	// Function should only be called once
	assert.Equal(b, 1, callCount)
}
