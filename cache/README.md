# cache

Caching utilities for memoizing context-based functions with TTL, retries, and backoff support.

## Overview

The `cache` package provides builders for creating cached versions of functions that take a context.

## API Reference

### NewFunc

```go
func NewFunc[T any](fn CachedContextFunc[T]) CachedFuncBuilder[T]
```

### NewKeyFunc

```go
func NewKeyFunc[T any, K comparable](fn CachedContextKeyFunc[T, K]) CachedKeyFuncBuilder[T, K]
```

### Builder Methods

```go
func (builder CachedFuncBuilder[T]) WithTTL(ttl time.Duration) CachedFuncBuilder[T]
func (builder CachedFuncBuilder[T]) WithRetriesExponentialBackoff(retries int) CachedFuncBuilder[T]
func (builder CachedKeyFuncBuilder[T, K]) WithMaxEntries(maxEntries int) CachedKeyFuncBuilder[T, K]
func (builder CachedKeyFuncBuilder[T, K]) WithCleanupInterval(cleanupInterval time.Duration) CachedKeyFuncBuilder[T, K]
```

## Usage

```go
fetchData := cache.NewFunc(func(ctx context.Context) (string, error) {
    return expensiveOperation()
}).WithTTL(5 * time.Second).Build()

result, _ := fetchData(ctx) // Cached for 5 seconds
```
