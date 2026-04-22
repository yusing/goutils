# cache

Caching utilities for memoizing context-aware functions with TTL, retries, and optional bounded key eviction.

## Overview

The `cache` package provides builders for wrapping context-aware functions with:

- lock-free read paths for already-cached values
- serialized refreshes on cache misses or TTL expiry
- retry support with per-refresh backoff instances
- optional MRU/LRU-style entry trimming for keyed caches

## API Reference

### NewFunc

```go
func NewFunc[T any](fn CachedContextFunc[T]) CachedFuncBuilder[T]
```

Wraps a single context-aware function and caches the most recent computed value.

### NewKeyFunc

```go
func NewKeyFunc[T any, K comparable](fn CachedContextKeyFunc[T, K]) CachedKeyFuncBuilder[T, K]
```

Wraps a keyed context-aware function and maintains one cached value per key.

## Builder Methods

```go
func (builder CachedFuncBuilder[T]) WithTTL(ttl time.Duration) CachedFuncBuilder[T]
func (builder CachedFuncBuilder[T]) WithRetriesExponentialBackoff(retries int) CachedFuncBuilder[T]
func (builder CachedFuncBuilder[T]) WithRetriesConstantBackoff(retries int, interval time.Duration) CachedFuncBuilder[T]
func (builder CachedFuncBuilder[T]) WithRetriesZeroBackoff(retries int) CachedFuncBuilder[T]

func (builder CachedKeyFuncBuilder[T, K]) WithTTL(ttl time.Duration) CachedKeyFuncBuilder[T, K]
func (builder CachedKeyFuncBuilder[T, K]) WithRetriesExponentialBackoff(retries int) CachedKeyFuncBuilder[T, K]
func (builder CachedKeyFuncBuilder[T, K]) WithRetriesConstantBackoff(retries int, interval time.Duration) CachedKeyFuncBuilder[T, K]
func (builder CachedKeyFuncBuilder[T, K]) WithRetriesZeroBackoff(retries int) CachedKeyFuncBuilder[T, K]
func (builder CachedKeyFuncBuilder[T, K]) WithMaxEntries(maxEntries int) CachedKeyFuncBuilder[T, K]
func (builder CachedKeyFuncBuilder[T, K]) WithCleanupInterval(cleanupInterval time.Duration) CachedKeyFuncBuilder[T, K]
```

`CachedFuncBuilder[T]`

- `WithTTL(ttl time.Duration)` - Sets the time-to-live for cached values.
- `WithRetriesExponentialBackoff(retries int)` - Retries failed refreshes with a fresh exponential backoff per-refresh.
- `WithRetriesConstantBackoff(retries int, interval time.Duration)` - Retries failed refreshes with a fixed delay between attempts.
- `WithRetriesZeroBackoff(retries int)` - Retries failed refreshes immediately without sleeping between attempts.

`CachedKeyFuncBuilder[T, K]`

- `WithTTL(ttl time.Duration)` - Sets the time-to-live for each cached key/value entry.
- `WithRetriesExponentialBackoff(retries int)` - Retries failed per-key refreshes with a fresh exponential backoff per-refresh.
- `WithRetriesConstantBackoff(retries int, interval time.Duration)` - Retries failed per-key refreshes with a fixed delay between attempts.
- `WithRetriesZeroBackoff(retries int)` - Retries failed per-key refreshes immediately without sleeping between attempts.
- `WithMaxEntries(maxEntries int)` - Sets the maximum number of cached entries and enables eviction of older entries when the cache grows past that limit.
- `WithCleanupInterval(cleanupInterval time.Duration)` - Sets how often the keyed-cache janitor checks for overflow when `WithMaxEntries` is enabled.

## Concurrency Model

- Cached reads use immutable atomic snapshots, so hot-path reads do not take a mutex.
- Keyed caches track recent access with atomic access sequences plus a multi-producer/single-consumer access log instead of a global MRU lock on every hit.
- Only refreshes take a lock, ensuring one refresh per entry at a time.
- Retry backoff instances are created per-refresh, avoiding shared mutable retry state across goroutines.

## Usage

```go
fetchData := cache.NewFunc(func(ctx context.Context) (string, error) {
	return expensiveOperation()
}).WithTTL(5 * time.Second).Build()

result, _ := fetchData(ctx) // cached for 5 seconds
```

For keyed caches, the built function takes both `context.Context` and the cache key:

```go
fetchUserData := cache.NewKeyFunc(func(ctx context.Context, userID string) (string, error) {
	return expensiveUserLookup(ctx, userID)
}).WithTTL(30 * time.Second).
	WithMaxEntries(1024).
	Build()

user, _ := fetchUserData(ctx, "user-123") // caches per key; WithMaxEntries caps the number of live entries
```


## Debug logging

When built with `-tags=debug`, the cache package emits debug logs for cache hits, misses, TTL-triggered recomputes, usage, and keyed-cache overflow evictions. Logged keys and values are summarized through the same formatter: long strings and `[]byte` values are truncated, large slices and maps are capped, structs are expanded into exported fields, and nested content is summarized recursively so large payloads do not flood the logs.

Without the `debug` build tag, these helpers compile to no-op stubs.
