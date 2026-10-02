# goutils/synk

Concurrency and memory helpers:

- reusable byte buffers in two process-wide pools, one for known sizes and one
  for data of unknown size, so hot paths allocate less;
- `RefCount`, a reference counter that announces when the last reference is gone;
- `Value[T]`, a type-safe `atomic.Value`;
- [`synk/workerpool`](workerpool/README.md), a bounded group of goroutines
  sharing a context.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/synk"
```

The package is in the root module and needs Go 1.27 or later. The workerpool
subpackage is `github.com/yusing/goutils/synk/workerpool`.

## Byte buffer pools

### Quick start

```go
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/yusing/goutils/synk"
)

func main() {
	// Known length: borrow a slice of exactly 1 KiB.
	sized := synk.GetSizedBytesPool()
	buf := sized.GetSized(1024)
	defer sized.Put(buf)

	n, err := io.ReadFull(strings.NewReader(strings.Repeat("x", 4096)), buf)
	fmt.Println(n, err) // 1024 <nil>

	// Unknown length: borrow a *bytes.Buffer.
	unsized := synk.GetUnsizedBytesPool()
	out := unsized.GetBuffer()
	defer unsized.PutBuffer(out)

	if _, err := io.Copy(out, strings.NewReader("hello")); err != nil {
		panic(err)
	}
	fmt.Println(out.String()) // hello
}
```

### Choosing a pool

| You have | Use | Returns |
| --- | --- | --- |
| A known length, for example a fixed read block | `GetSizedBytesPool().GetSized(n)` | `[]byte` with `len == n` |
| A known size and a `*bytes.Buffer` is convenient | `GetSizedBytesPool().GetBuffer(n)` | empty buffer with capacity of at least `n` |
| Data of unknown size, such as `io.Copy` or building output | `GetUnsizedBytesPool().GetBuffer()` or `.Get()` | empty buffer or `[]byte` with `len == 0` |
| Unknown size with a likely minimum | `GetUnsizedBytesPool().GetBufferAtLeast(n)` or `.GetAtLeast(n)` | the same, with capacity of at least `n` |

Each `Get` has a matching `Put` for slices and `PutBuffer` for buffers. The pools
are process-wide: `GetSizedBytesPool()` and `GetUnsizedBytesPool()` return the
same shared pool every time, and they are safe for concurrent use.

```go
// Slices from the unsized pool.
b := unsized.Get() // len 0; capacity is 4 KiB for buffers the pool allocated
b = append(b, data...)
unsized.Put(b)

// A minimum capacity.
b = unsized.GetAtLeast(64 << 10)
unsized.Put(b)
```

### Ownership rules

- You own a buffer from `Get` until `Put`. After `Put` or `PutBuffer`, do not
  read, write, append to, return, or `Put` it again. A second `Put` hands one
  array to two later callers.
- Put back the buffer you currently hold. `defer pool.Put(buf)` captures `buf` at
  the `defer` statement. If you later reassign `buf`, for example after an
  `append` that reallocated, the deferred call still returns the old slice.
- Memory is not zeroed on reuse. Overwrite the part you read from, and clear
  secrets before `Put`.
- `GetSized` returns the requested length. A prefix split off a larger recycled
  tier has `cap == len`, so `append` allocates rather than writing into the
  separately pooled tail. Treat the slice as fixed length and use
  `GetBuffer`/`GetAtLeast` when you need room to grow.
- Use `PutBuffer` for buffers from `GetBuffer`, which resets the buffer first.
  Buffers that grew beyond their initial capacity return their larger backing
  array.

### What the pools guarantee

- Sizes. Sized requests are served from 11 tiers whose capacities double from
  2 KiB to 2 MiB. `GetSized(n)` returns a slice of length `n` whose capacity is
  `n` rounded up to a tier, or exactly `n` when it was split from a larger
  recycled buffer. `GetSized(0)` returns an empty slice with 2 KiB capacity. A
  request above 2 MiB is an exact allocation that `Put` drops.
  The unsized pool allocates 4 KiB buffers (`MinAllocSize`), but `Get` may return
  any recycled buffer that was `Put`, including one that did not come from the
  pool and has less capacity. Use `GetAtLeast` or `GetBufferAtLeast` when a
  minimum matters.
- Reuse is best effort. Pooled buffers are held through weak pointers, so the
  pool never keeps memory alive. After a garbage collection, buffers that nothing
  else references are gone and the next `Get` allocates. The pools save
  allocations between collections, and they cannot cause a leak.
- Limits. A full pool drops what you `Put` instead of growing. The sized pool
  also drops buffers smaller than 2 KiB or larger than 2 MiB. No call reports
  this.
- Constants. `MinAllocSize` (4 KiB), `UnsizedPoolLimit` (16 MiB),
  `UnsizedPoolSize` (4096 entries), and `SizedPools` (11) are exported
  constants. They describe the pools and cannot be configured.

## RefCount

Tells you when the last user of something has let go.

```go
package main

import (
	"fmt"

	"github.com/yusing/goutils/synk"
)

func main() {
	rc := synk.NewRefCounter() // starts at 1: the owner's reference

	for i := range 3 {
		rc.Add() // one reference per user
		go func() {
			defer rc.Sub()
			fmt.Println("user", i)
		}()
	}

	rc.Sub() // the owner releases its own reference
	<-rc.Zero()
	fmt.Println("all references released")
}
```

- `NewRefCounter` starts at 1. `Add` increments, `Sub` decrements, and `Zero()`
  returns a channel that is closed when the count reaches zero.
- `Add` after the count reached zero panics with
  `RefCount.Add() called after count reached zero`. Take every reference while you
  still hold one.
- Extra `Sub` calls at zero do nothing.
- A `RefCount` must not be copied. Its no-op `Lock` and `Unlock` methods exist so
  `go vet` flags copies.

## Value[T]

A generic wrapper over `sync/atomic.Value`.

```go
package main

import (
	"fmt"

	"github.com/yusing/goutils/synk"
)

type Config struct{ Name string }

func main() {
	var current synk.Value[*Config]

	fmt.Println(current.Load() == nil) // true: unset returns the zero value of T

	current.Store(&Config{Name: "a"})
	old := current.Swap(&Config{Name: "b"})
	fmt.Println(old.Name, current.Load().Name) // a b
}
```

| Method | Behavior |
| --- | --- |
| `Load() T` | The stored value, or the zero `T` if nothing was stored. |
| `Store(v T)` | Stores `v`. |
| `Swap(v T) T` | Stores `v` and returns the previous value, or the zero `T`. |
| `MarshalJSON()` | Encodes the stored value, or the zero `T` if unset, with the goutils JSON encoder. |

The `atomic.Value` rules apply. Do not copy a `Value` after use. `Store` panics
if the value is a nil interface, and if the concrete type differs from the first
stored one. So `Value[error]` panics when you store two different error
implementations. Use a pointer or struct type, or wrap the interface in a struct.

## Build tags

| Tag | Effect |
| --- | --- |
| `pprof` | Starts a reporter at package initialization that logs `bytes pool stats` at `Info` every 5 seconds, with fields `sizeInUse`, `numReused`, `numDropped`, `numNonPooled`, `numGced`, and the matching `size*` fields. Messages go through [`logging`](../logging/README.md) and appear only when the application installed a logger. |
| `race` | Selected automatically by `-race`. The typed pools use locked shared queues and no private slot so the race detector can check them. Behavior is otherwise the same. |

The `pprof` reporter does not install a signal handler, so Ctrl-C retains its normal behavior.

## Compatibility

The pools use `//go:linkname` to `runtime.procPin` and `runtime.procUnpin` for
per-processor storage. This builds on Go 1.27 without extra linker flags. After a
Go toolchain upgrade, run `go test ./synk` with and without `-race`.

## Maintainer notes

The remainder describes the implementation and records measurements. It is not
needed to use the package.

### Design goals

- Minimize allocations and GC pressure by reusing buffers instead of allocating
  new ones.
- Bound memory waste. Requests map to geometric size tiers, and splitting is
  limited to backing arrays no larger than 2 MiB.
- Prevent leaks with weak references, so collected buffers disappear from pool
  slots.
- Stay fast with typed per-P storage and `weak`/`unsafe` plumbing.

### Dual pool system

`UnsizedBytesPool`:

- one typed per-P pool for general-purpose buffers;
- new buffers start at `MinAllocSize` (4 KiB);
- suited to variable-size use cases such as `io.Copy`.

`SizedBytesPool`:

- eleven tiered pools whose nominal capacities are `2 KiB << i` for
  `i = 0 … 10` (2 KiB through 2 MiB);
- requests below 2 KiB use the first tier;
- requests larger than 2 MiB allocate an exact-size `[]byte`, and `Put` drops the
  allocation instead of retaining aliases to one oversized backing array.

### Weak reference mechanism

The pool stores `weakBuf` values directly in typed per-P queues:

```go
type weakBuf struct {
	ptr weak.Pointer[byte]
	cap int
}
```

Slices are not weak-referenced directly. The data pointer and capacity are stored
so a live `[]byte` can be rebuilt with `unsafe.Slice` when the weak pointer is
still valid. If the GC collects the byte array while it is only weak-reachable,
`getBufFromWeak` returns `nil` and the pool discards that slot and tries another
buffer.

### Pool index

```go
func poolIdx(size int) int {
	if size <= 0 {
		return 0
	}
	return min(SizedPools-1, max(0, bits.Len(uint(size-1))-11))
}
```

`bits.Len(size-1)` locates the highest set bit. Subtracting 11 aligns indexing
with the tier scale. `poolIdx` maps a capacity (or requested size) to the
smallest tier that can hold it.

### Bounded tier fallback (`GetSized`)

Each request checks its smallest fitting tier first, then larger tiers. When a
larger buffer is reused, `GetSized` returns the requested prefix and puts a tail
of at least 2 KiB back into the appropriate tier. The prefix capacity is capped at
its length, keeping simultaneously borrowed pieces disjoint. Because sized tiers
stop at 2 MiB and oversized buffers are dropped, one borrowed piece cannot pin an
unbounded allocation. A complete miss allocates the target tier.

### Typed per-P storage

```go
func poolSharedLimit(idx int) int {
	return max(8, 256>>uint(idx))
}
```

Each P has one private slot and one typed lock-free shared chain. Smaller tiers
(hotter paths) get larger per-P shared limits, and larger tiers get smaller
limits. This removes channel contention and avoids the `any` boxing allocation
incurred by storing `[]byte` in `sync.Pool`. Only weak-reference metadata is
retained: the maximum number of entries is `(shared limit + 1) * GOMAXPROCS` per
tier.

The implementation adapts Go's `sync.Pool` and `poolChain` algorithms and calls
`runtime.procPin`/`runtime.procUnpin` through linkname declarations. This is a
deliberate performance and toolchain coupling. Go upgrades must run the pool
tests, the race detector, and the benchmarks. Race builds use locked shared heads
and omit the private slot so the race detector can verify the queue operations.

### `Put`

- Unsized: `Put` stores a weak handle. If the local P's shared queue is full, the
  buffer is dropped.
- Sized: `Put` routes by capacity to one tier. Buffers below the first tier or
  above the last tier are dropped.

### Benchmarks

`BenchmarkSizedPoolPatterns` covers steady reuse, mixed sizes, concurrent access,
and oversized returns. `BenchmarkSizedPoolArchitectures` compares exact-tier
lookup, whole-buffer fallback, and bounded split fallback for cold tier-skew
bursts and oversized returns.

`BenchmarkSizedPoolBackends` compares the typed per-P pool, weak-channel storage,
and standard `sync.Pool` under identical tier and split policies.
`BenchmarkSizedPoolBackendBursts` additionally exercises the shared chains
instead of only private-slot reuse. The typed pool omits the standard victim cache
because weak entries do not retain backing arrays.

`BenchmarkSizedPoolDeadRecovery` measures post-GC recovery episodes: limiting a
pull to eight dead entries causes 32 consecutive misses for a full 256-entry sized
tier and 512 misses for the 4096-entry unsized pool, while draining the tier
causes one miss. Production therefore drains dead entries until finding a live
buffer or an empty queue.

Recorded implementation comparison on Go 1.26.5, linux/amd64, Intel i5-13500
(`count=8`, median). Cells show time, B/op, and allocs/op.

| Production pattern | Result |
| --- | ---: |
| Sub-tier steady reuse | 34.7 ns, 0 B, 0 allocs |
| Exact-tier steady reuse | 34.1 ns, 0 B, 0 allocs |
| Mixed common sizes | 35.6 ns, 0 B, 0 allocs |
| Parallel 32 KiB, 8 CPUs | 5.38 ns, 0 B, 0 allocs |
| Oversized return | 1.14 ns, 0 B, 0 allocs |

| Cold tier-skew burst | Exact tier | Whole fallback | Split fallback |
| --- | ---: | ---: | ---: |
| 4 KiB → 2 KiB | 3.91 µs, 32 KiB, 16 allocs | 3.15 µs, 16 KiB, 8 allocs | 771 ns, 0 B, 0 allocs |
| 8 KiB → 3 KiB | 7.05 µs, 64 KiB, 16 allocs | 4.71 µs, 32 KiB, 8 allocs | 1.07 µs, 0 B, 0 allocs |
| 256 KiB → 32 KiB | 140 µs, 2 MiB, 64 allocs | 127 µs, 1.75 MiB, 56 allocs | 3.60 µs, 0 B, 0 allocs |

Extreme fallback results compare an experimental 8× search limit with
unrestricted splitting. A single unrestricted fallback pins the 2 MiB seed while
borrowed. The 8× limit leaves that weak entry unused and allocates a 2 KiB buffer.

| 2 MiB → 2 KiB | Exact tier | Whole fallback | Split fallback | Split max 8× |
| --- | ---: | ---: | ---: | ---: |
| One request | 324 ns, 2 KiB, 1 alloc | 203 ns, 0 B, 0 allocs | 234 ns, 0 B, 0 allocs | 348 ns, 2 KiB, 1 alloc |
| 32-request burst | 6.88 µs, 64 KiB, 32 allocs | 9.75 µs, 62 KiB, 31 allocs | 4.77 µs, 0 B, 0 allocs | 7.57 µs, 64 KiB, 32 allocs |

The limit bounds pinning but restores every avoided allocation in a skewed burst.
Because sized backing arrays are already capped at 2 MiB, production keeps
unrestricted in-range splitting.

For a 64 MiB oversized return, dropping takes 1.37 ns median. Recursive splitting
takes 1.13 µs and creates reusable slices that can pin the entire oversized
backing allocation. Sized fallback therefore splits only allocations already
bounded by the 2 MiB maximum tier.

Backend comparison from sequential `go test` processes on the same host
(`count=8`, median):

| Pattern | Weak channel | `sync.Pool` | Typed per-P |
| --- | ---: | ---: | ---: |
| Serial hot tier | 57.6 ns, 0 B, 0 allocs | 33.8 ns, 24 B, 1 alloc | 37.2 ns, 0 B, 0 allocs |
| Serial mixed tiers | 60.4 ns, 0 B, 0 allocs | 34.0 ns, 24 B, 1 alloc | 35.8 ns, 0 B, 0 allocs |
| Parallel hot tier, 8 CPUs | 82.9 ns, 0 B, 0 allocs | 10.0 ns, 24 B, 1 alloc | 5.53 ns, 0 B, 0 allocs |
| Parallel mixed tiers, 8 CPUs | 113 ns, 0 B, 0 allocs | 7.72 ns, 29 B, 1 alloc | 7.75 ns, 0 B, 0 allocs |

Shared-chain burst results (`count=8`, median):

| Pattern | Weak channel | `sync.Pool` | Typed per-P |
| --- | ---: | ---: | ---: |
| Serial burst of 16 | 891 ns, 0 B, 0 allocs | 775 ns, 384 B, 16 allocs | 918 ns, 0 B, 0 allocs |
| Parallel burst of 8, 8 CPUs | 1.63 µs, ~4.8 KiB, 0 allocs | 122 ns, 196 B, 8 allocs | 76.9 ns, 0 B, 0 allocs |

Parallel rows report aggregate `RunParallel` throughput per operation, not
single-operation latency. They expose contention as concurrency increases.

Run the focused pool benchmarks from the repository root with:

```sh
go test ./synk -run='^$' -bench='^BenchmarkSizedPool(Patterns|Architectures)$' -benchmem -count=8
go test ./synk -run='^$' -bench='^BenchmarkSizedPoolBackend(s|Bursts)$' -benchmem -count=8 -cpu=1,8
go test ./synk -run='^$' -bench='^BenchmarkSizedPoolDeadRecovery$' -benchmem -benchtime=100x
```
