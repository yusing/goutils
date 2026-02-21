package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCachedFuncState_BasicCaching(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		return "result", nil
	}

	cachedFunc := NewFunc(fn).Build()

	// First call should execute the function
	result, err := cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.Equal(t, 1, callCount)

	// Second call should use cached result
	result, err = cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.Equal(t, 1, callCount) // Still 1, function not called again
}

func TestCachedFuncState_WithError(t *testing.T) {
	callCount := 0
	testErr := errors.New("test error")
	fn := func(ctx context.Context) (string, error) {
		callCount++
		return "", testErr
	}

	cachedFunc := NewFunc(fn).Build()

	// First call should execute and return error
	result, err := cachedFunc(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, testErr)
	assert.Empty(t, result)
	assert.Equal(t, 1, callCount)

	// Second call should use cached error
	result, err = cachedFunc(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, testErr)
	assert.Empty(t, result)
	assert.Equal(t, 1, callCount) // Still 1, function not called again
}

func TestCachedFuncState_WithTTL(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		return "result", nil
	}

	ttl := 100 * time.Millisecond
	cachedFunc := NewFunc(fn).WithTTL(ttl).Build()

	// First call
	result, err := cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.Equal(t, 1, callCount)

	// Second call within TTL should use cache
	result, err = cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.Equal(t, 1, callCount)

	// Wait for TTL to expire
	time.Sleep(ttl + 10*time.Millisecond)

	// Third call after TTL should execute function again
	result, err = cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.Equal(t, 2, callCount)
}

func TestCachedFuncState_WithZeroTTL(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		return "result", nil
	}

	cachedFunc := NewFunc(fn).WithTTL(0).Build()

	// With TTL=0, cache should never expire
	result, err := cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.Equal(t, 1, callCount)

	// Should always use cache
	result, err = cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.Equal(t, 1, callCount)
}

func TestCachedFuncState_WithRetries(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		if callCount < 3 {
			return "", errors.New("temporary error")
		}
		return "success", nil
	}

	cachedFunc := NewFunc(fn).WithRetriesZeroBackoff(3).Build()

	result, err := cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 3, callCount) // Called 3 times (1 initial + 2 retries)
}

func TestCachedFuncState_RetryExhausted(t *testing.T) {
	callCount := 0
	testErr := errors.New("persistent error")
	fn := func(ctx context.Context) (string, error) {
		callCount++
		return "", testErr
	}

	cachedFunc := NewFunc(fn).WithRetriesZeroBackoff(2).Build()

	result, err := cachedFunc(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, testErr)
	assert.Empty(t, result)
	assert.Equal(t, 3, callCount) // Called 3 times (1 initial + 2 retries)
}

func TestCachedFuncState_ContextCancellation(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			// Always return error to force retries
			return "", errors.New("persistent error")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cachedFunc := NewFunc(fn).WithRetriesConstantBackoff(10, 10*time.Millisecond).Build()

	result, err := cachedFunc(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, result)
	// Function should have been called multiple times before context cancellation
	assert.GreaterOrEqual(t, callCount, 1)
}

func TestCachedFuncState_ConcurrentAccess(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context) (int, error) {
		callCount++
		time.Sleep(50 * time.Millisecond) // Simulate work
		return 42, nil
	}

	cachedFunc := NewFunc(fn).Build()

	var wg sync.WaitGroup
	results := make([]int, 10)
	errs := make([]error, 10)

	// Launch multiple goroutines concurrently
	for i := range 10 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = cachedFunc(context.Background())
		}(i)
	}

	wg.Wait()

	// All calls should have the same result
	for i := range 10 {
		assert.NoError(t, errs[i])
		assert.Equal(t, 42, results[i])
	}

	// Function should only be called once due to proper locking
	assert.Equal(t, 1, callCount)
}

func TestCachedFuncState_DifferentTypes(t *testing.T) {
	// Test with int
	intFn := func(ctx context.Context) (int, error) {
		return 123, nil
	}
	intCached := NewFunc(intFn).Build()
	result, err := intCached(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 123, result)

	// Test with struct
	type Person struct {
		Name string
		Age  int
	}
	personFn := func(ctx context.Context) (Person, error) {
		return Person{Name: "John", Age: 30}, nil
	}
	personCached := NewFunc(personFn).Build()
	person, err := personCached(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, Person{Name: "John", Age: 30}, person)

	// Test with slice
	sliceFn := func(ctx context.Context) ([]string, error) {
		return []string{"a", "b", "c"}, nil
	}
	sliceCached := NewFunc(sliceFn).Build()
	slice, err := sliceCached(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, slice)
}

func TestCachedFuncState_BackoffReset(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		if callCount == 1 {
			return "", errors.New("first error")
		}
		return "success", nil
	}

	cachedFunc := NewFunc(fn).WithRetriesExponentialBackoff(2).Build()

	// First call should fail once then succeed
	result, err := cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 2, callCount)

	// Reset call count for second test
	callCount = 0

	// Second call should use cached result
	result, err = cachedFunc(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "success", result)
	assert.Equal(t, 0, callCount) // Function not called again
}

func TestCachedFuncState_ExpirationLogic(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		return "result", nil
	}

	state := &CachedFuncState[string]{
		CachedFuncBuilder: CachedFuncBuilder[string]{
			CachedFuncConfig: CachedFuncConfig{
				ttl: 100 * time.Millisecond,
			},
			fn: fn,
		},
	}

	// Test checkExpired when never set
	assert.True(t, state.checkExpired())

	// Test checkExpired after setting result
	state.setResult("test", nil)
	assert.False(t, state.checkExpired())

	// Test checkExpired after TTL expires
	time.Sleep(150 * time.Millisecond)
	assert.True(t, state.checkExpired())

	// Test with zero TTL (should never expire)
	stateZeroTTL := &CachedFuncState[string]{
		CachedFuncBuilder: CachedFuncBuilder[string]{
			fn: fn,
		},
	}
	stateZeroTTL.setResult("test", nil)
	assert.False(t, stateZeroTTL.checkExpired())
	time.Sleep(50 * time.Millisecond)
	assert.False(t, stateZeroTTL.checkExpired()) // Should still be false with TTL=0
}

func BenchmarkCachedFunc_NoCache(b *testing.B) {
	fn := func(ctx context.Context) (string, error) {
		return "result", nil
	}

	// Create new cache each time to simulate no caching
	b.ResetTimer()
	for b.Loop() {
		cachedFunc := NewFunc(fn).Build()
		_, _ = cachedFunc(context.Background())
	}
}

func BenchmarkCachedFunc_WithCache(b *testing.B) {
	callCount := 0
	fn := func(ctx context.Context) (string, error) {
		callCount++
		return "result", nil
	}

	cachedFunc := NewFunc(fn).Build()

	b.ResetTimer()
	for b.Loop() {
		_, _ = cachedFunc(context.Background())
	}

	// Function should only be called once
	assert.Equal(b, 1, callCount)
}
