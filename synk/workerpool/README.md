# goutils/synk/workerpool

Runs functions concurrently with at most N at a time, under one shared context.
It is a semaphore with a `Wait`, for a batch of independent jobs where you do not
need results or errors collected for you.

It is not a set of long-lived workers: each `Go` call starts a new goroutine once
a slot is free. There is no queue, no result channel, and no panic recovery.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/synk/workerpool"
```

The package is in the root module and needs Go 1.27 or later. It is documented
with [`synk`](../README.md), which holds the buffer pools, `RefCount`, and
`Value[T]`.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/yusing/goutils/synk/workerpool"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := workerpool.New(ctx, workerpool.WithN(4)) // at most 4 at a time

	var done atomic.Int32
	for range 20 {
		pool.Go(func(ctx context.Context, idx int) {
			if ctx.Err() != nil {
				return
			}
			done.Add(1)
		})
	}

	pool.Wait()
	fmt.Println("done:", done.Load()) // done: 20
}
```

## API

```go
func New(ctx context.Context, opts ...option) Pool
func WithN(n int) option

type Pool interface {
	Go(fn func(ctx context.Context, idx int))
	Wait()
}
```

`option` is unexported. The only option is `WithN`.

| Call | Behavior |
| --- | --- |
| `New(ctx, opts...)` | Creates a pool. A nil `ctx` becomes `context.Background()`. The default limit is `runtime.GOMAXPROCS(0)` at creation time. |
| `WithN(n)` | Limits concurrency to `n`, which must be at least 1. `n < 1` is not validated: `0` makes `Go` block until the context ends and never run anything, and a negative value panics in `New`. |
| `Go(fn)` | Blocks until a slot is free, then runs `fn` on a new goroutine and returns. A nil `fn` is ignored. |
| `Wait()` | Blocks until every started function has returned. The pool can be used again afterwards. |

## Behavior to know about

- Back pressure. `Go` blocks the caller while all slots are busy, so a producer
  loop can never get ahead of the pool by more than `n` functions.
- Cancellation. `fn` receives the pool's context. If the context ends while `Go`
  is waiting for a slot, `Go` returns without running `fn`. When a slot is free
  at the same moment as the cancellation, either outcome is possible, so `fn`
  should check `ctx.Err()` itself.
- `Wait` and cancellation. `Wait` returns early if the context ends, even while
  started functions are still running. After a canceled `Wait`, do not assume
  your jobs have finished. Synchronize on your own signal if you need that.
- `idx` is the sequence number of the `Go` call, counting from 0 for the pool's
  lifetime, and it is unique per call. It is not a worker slot number and is not
  limited to `0..n-1`, so do not use it to index a per-worker array of size `n`.
- Panics. A panic in `fn` is not recovered and ends the program. Recover inside
  `fn` if jobs can panic.
- Errors. Nothing is collected. Record results and errors yourself, with a mutex
  or a channel. If you want first-error cancellation, use `errgroup` with
  `SetLimit`.
- Concurrency. `Go` may be called from several goroutines, and work submitted
  while `Wait` is waiting delays it. Call `Wait` from one goroutine at a time:
  each `Wait` holds the slots it has collected until it has all of them, so
  concurrent `Wait` calls can block each other indefinitely. Ending the context
  releases them.
