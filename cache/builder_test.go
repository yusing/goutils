package cache

import (
	"context"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/assert"
)

func TestNewFunc(t *testing.T) {
	fn := func(ctx context.Context) (string, error) {
		return "test", nil
	}

	builder := NewFunc(fn)
	assert.NotNil(t, builder)
	assert.NotNil(t, builder.fn)
	assert.Equal(t, 0, builder.retries)
	assert.Nil(t, builder.backoff)
	assert.Equal(t, time.Duration(0), builder.ttl)
}

func TestWithRetriesExponentialBackoff(t *testing.T) {
	fn := func(ctx context.Context) (string, error) {
		return "test", nil
	}

	builder := NewFunc(fn).WithRetriesExponentialBackoff(3)
	assert.Equal(t, 3, builder.retries)
	assert.NotNil(t, builder.backoff)
	assert.IsType(t, &backoff.ExponentialBackOff{}, builder.backoff)
}

func TestWithRetriesConstantBackoff(t *testing.T) {
	fn := func(ctx context.Context) (string, error) {
		return "test", nil
	}

	interval := 100 * time.Millisecond
	builder := NewFunc(fn).WithRetriesConstantBackoff(5, interval)
	assert.Equal(t, 5, builder.retries)
	assert.NotNil(t, builder.backoff)
	assert.IsType(t, &backoff.ConstantBackOff{}, builder.backoff)
}

func TestWithRetriesZeroBackoff(t *testing.T) {
	fn := func(ctx context.Context) (string, error) {
		return "test", nil
	}

	builder := NewFunc(fn).WithRetriesZeroBackoff(2)
	assert.Equal(t, 2, builder.retries)
	assert.NotNil(t, builder.backoff)
	assert.IsType(t, &backoff.ZeroBackOff{}, builder.backoff)
}

func TestWithTTL(t *testing.T) {
	fn := func(ctx context.Context) (string, error) {
		return "test", nil
	}

	ttl := 5 * time.Minute
	builder := NewFunc(fn).WithTTL(ttl)
	assert.Equal(t, ttl, builder.ttl)
}

func TestBuild(t *testing.T) {
	fn := func(ctx context.Context) (string, error) {
		return "test", nil
	}

	builder := NewFunc(fn)
	cachedFunc := builder.Build()
	assert.NotNil(t, cachedFunc)
	assert.IsType(t, (CachedContextFunc[string])(nil), cachedFunc)
}

func TestBuildWithAllOptions(t *testing.T) {
	fn := func(ctx context.Context) (int, error) {
		return 42, nil
	}

	cachedFunc := NewFunc(fn).
		WithRetriesExponentialBackoff(3).
		WithTTL(time.Hour).
		Build()

	assert.NotNil(t, cachedFunc)
}

func TestBuilderChaining(t *testing.T) {
	fn := func(ctx context.Context) (bool, error) {
		return true, nil
	}

	builder := NewFunc(fn).
		WithRetriesConstantBackoff(2, 50*time.Millisecond).
		WithTTL(30 * time.Second)

	assert.Equal(t, 2, builder.retries)
	assert.NotNil(t, builder.backoff)
	assert.Equal(t, 30*time.Second, builder.ttl)

	// Test that chaining doesn't modify original
	originalBuilder := NewFunc(fn)
	chainBuilder := originalBuilder.WithRetriesZeroBackoff(1)
	assert.Equal(t, 0, originalBuilder.retries) // original unchanged
	assert.Equal(t, 1, chainBuilder.retries)    // new builder has changes
}
