# goutils/cache

Memoizes context-aware functions: wrap a `func(ctx) (T, error)` or a
`func(ctx, key) (T, error)` once, and callers get cached results with a time-to-live,
single-flight refreshes, retries with backoff, and (for keyed caches) a bounded number
of entries. Package name: `cache`.

## Install

```sh
go get github.com/yusing/goutils/cache@v0.8.0
```

```go
import "github.com/yusing/goutils/cache"
```

- Go 1.27. The module is self-contained: it does not depend on the root
  `github.com/yusing/goutils` module. It requires `github.com/cenkalti/backoff/v5` and
  `github.com/puzpuzpuz/xsync/v4`; zerolog is linked only with the `debug` build tag.
- No linker flags, environment variables, or logging unless built with `-tags=debug`.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yusing/goutils/cache"
)

func main() {
	calls := 0
	rate := cache.NewFunc(func(ctx context.Context) (float64, error) {
		calls++
		return 1.2345, nil // stands in for an expensive lookup
	}).WithTTL(time.Minute).Build()

	ctx := context.Background()
	a, _ := rate(ctx)
	b, _ := rate(ctx)
	fmt.Println(a, b, calls) // 1.2345 1.2345 1
}
```

A keyed cache keeps one value per key:

```go
user := cache.NewKeyFunc(func(ctx context.Context, id string) (string, error) {
	return lookupUser(ctx, id)
}).
	WithTTL(30 * time.Second).
	WithRetriesConstantBackoff(2, 100*time.Millisecond).
	WithMaxEntries(1024).
	Build()

name, err := user(ctx, "user-123")
```

`NewFunc` returns a `CachedFuncBuilder[T]`, `NewKeyFunc` a `CachedKeyFuncBuilder[T, K]`,
and `Build()` returns the cached function (`CachedContextFunc[T]` or
`CachedContextKeyFunc[T, K]`). Builder methods return modified copies, so chain them and
finish with `Build()`. Build a cache once, at start-up or as a long-lived field, and
reuse the function: a cache built per request caches nothing across requests, and each
bounded keyed cache permanently registers with the process-wide janitor (see Limits).

## Behavior

### TTL

`WithTTL(d)` sets how long a result stays valid, counted from the moment the refresh
finishes.

- Zero (the default): results never expire. The function runs once per cache, or once
  per key.
- Positive: the next call after expiry recomputes. There is no background refresh and
  no early refresh.
- Negative: results are always considered expired, so every call recomputes.

### Errors are cached too

A call that returns an error stores that error as the result, for the whole TTL
(forever when no TTL is set, so a transient first-call failure sticks). Use a TTL, or
retries, when `fn` can fail temporarily. The exception is cancellation: when the
context given to the refreshing call is done and the error matches
`context.Cause(ctx)`, the result is discarded and the next call runs `fn` again. An
error that merely looks like a timeout but is unrelated to that context is cached.

### Single-flight refresh

Only one refresh runs at a time per cache (per key for keyed caches). Callers that
arrive during it wait and share its result; callers that find a valid value return
without locking.

- The refresh runs with the context of the caller that triggered it.
- A waiting caller is not released by its own context: it stays blocked until the
  refresh finishes, even if its context has expired.
- All callers receive the same value. Do not mutate returned maps, slices, or
  pointed-to data.

### Retries

`WithRetriesExponentialBackoff(n)`, `WithRetriesConstantBackoff(n, interval)`, and
`WithRetriesZeroBackoff(n)` retry a failed refresh up to `n` more times (`n = 2` means
at most three calls). Without one of them there are no retries.

- Exponential backoff uses the `backoff/v5` defaults: 500 ms initial delay with 50
  percent jitter, growing by 1.5 each time up to 60 s, with no overall time limit. Bound
  it with a context deadline.
- Retries stop as soon as the context is done and return `context.Cause(ctx)`.
- The delays happen while the refresh holds its lock, so waiting callers wait for all
  attempts. Only the final result, success or error, is cached.

### Keyed caches and limits

`WithMaxEntries(n)` bounds the number of keys; `WithCleanupInterval(d)` sets how often
the janitor may trim (default 15 s, minimum 1 s, ignored without `WithMaxEntries`).

- Without `WithMaxEntries` nothing is ever deleted. TTL only forces a recompute when a
  key is requested again, so memory grows with the number of distinct keys. Set a limit
  when keys come from user input or are otherwise unbounded.
- When a new key pushes the size above the limit, a shared background janitor evicts the
  least recently used entries. The limit is soft: the janitor runs at most once per
  cleanup interval, so the size can exceed `n` until the next pass. A key evicted while
  still wanted is simply recomputed on its next call.
- The janitor is process-wide with no fixed registration limit. Registrations are
  never released, so build bounded caches once and reuse them rather than creating
  them per request. `cache.Janitor` and the `State` interface are exported for custom
  cleanup states, which have the same process-lifetime ownership.

## Debug logging

Build with `-tags=debug` to log hits, misses, expiries, usage, and evictions through
zerolog's global logger at debug level. Keys and values are summarized: strings are cut
at 100 characters, slices and maps at 10 entries, and nesting at depth 32, but cached
values still appear in the log, so do not use the tag where they are sensitive. Without
the tag these calls compile to no-ops and zerolog is not linked.
