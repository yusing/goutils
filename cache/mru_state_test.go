package cache

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/assert"
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
		mru:     list.New(),
	}

	entry := &CacheEntry[string]{result: "test", err: nil}

	// Test checkExpired when never set
	assert.True(t, state.checkExpired(entry))

	// Test checkExpired after setting expireAt
	entry.expireAt = time.Now().Add(200 * time.Millisecond)
	assert.False(t, state.checkExpired(entry))

	// Test checkExpired after TTL expires
	entry.expireAt = time.Now().Add(-10 * time.Millisecond)
	assert.True(t, state.checkExpired(entry))

	// Test with zero TTL (should never expire)
	stateZeroTTL := &CachedContextKeyFuncState[string, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[string, int]{
			CachedFuncConfig: CachedFuncConfig{
				ttl: 0,
			},
			fn: fn,
		},
		entries: xsync.NewMap[int, *CacheEntry[string]](),
		mru:     list.New(),
	}

	entryZeroTTL := &CacheEntry[string]{result: "test", err: nil}
	entryZeroTTL.expireAt = time.Now().Add(-10 * time.Millisecond)
	assert.False(t, stateZeroTTL.checkExpired(entryZeroTTL)) // Should be false with TTL=0
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
