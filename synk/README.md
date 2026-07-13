# goutils/synk

A pool of byte buffers that are reused instead of allocated and freed.

## Usage

```go
import "github.com/yusing/goutils/synk"

sizedPool := synk.GetSizedBytesPool()
unsizedPool := synk.GetUnsizedBytesPool()

// as []byte
buf := sizedPool.GetSized(size)
defer sizedPool.Put(buf)

buf = unsizedPool.Get()
defer unsizedPool.Put(buf)

// Unsized: grow backing store when the default capacity is not enough
buf = unsizedPool.GetAtLeast(n)
defer unsizedPool.Put(buf)

// as *bytes.Buffer
buf = unsizedPool.GetBuffer()
defer unsizedPool.PutBuffer(buf)

buf = unsizedPool.GetBufferAtLeast(n)
defer unsizedPool.PutBuffer(buf)

buf = sizedPool.GetBuffer(size)
defer sizedPool.PutBuffer(buf)
```

`GetSized` returns the requested length. The caller owns the returned slice and
its visible capacity until `Put`. A prefix split from a larger tier has
`cap == len`, so appending allocates before reaching the separately pooled tail.

`Put` and `PutBuffer` transfer ownership to the pool. After either call, do not
read, write, append to, or return the same buffer again. Reused memory is not
zeroed; overwrite the required region before reading it or exposing it outside
the process. Callers handling secrets must clear them before `Put`.

## Design Philosophy

This pool implementation follows a tiered memory management strategy with several key goals:

- Minimize allocations and reduce GC pressure — reuse buffers instead of allocating new ones
- Bound memory waste — match requests to geometric size tiers and limit
  splitting to backing arrays no larger than 2 MiB
- Prevent memory leaks — use weak references so collected buffers can disappear from pool slots
- High performance — use typed per-P storage and `weak`/`unsafe` plumbing

## Core Architecture

### Dual pool system

#### `UnsizedBytesPool`

- Single typed per-P pool for general-purpose buffers
- New buffers start at `MinAllocSize` (4 KiB)
- Suited to variable-size use cases (`io.Copy`, and similar)

#### `SizedBytesPool`

- **Eleven tiered pools** whose nominal capacities are `2 KiB << i` for
  `i = 0 … 10` (2 KiB through 2 MiB)
- Requests below 2 KiB use the first tier
- Requests larger than 2 MiB allocate an exact-size `[]byte`; `Put` drops the
  allocation instead of retaining aliases to one oversized backing array

### Weak reference mechanism

The pool stores `weakBuf` values directly in typed per-P queues:

```go
type weakBuf struct {
	ptr weak.Pointer[byte]
	cap int
}
```

`slices` are not weak-referenced directly; the data pointer and capacity are stored so a live `[]byte` can be rebuilt with `unsafe.Slice` when the weak pointer is still valid.

If the GC collects the byte array while it is only weak-reachable, `getBufFromWeak` returns `nil` and the pool discards that slot and tries another buffer.

### Pool index

```go
func poolIdx(size int) int {
	if size <= 0 {
		return 0
	}
	return min(SizedPools-1, max(0, bits.Len(uint(size-1))-11))
}
```

`bits.Len(size-1)` locates the highest set bit; subtracting 11 aligns indexing with the tier scale. `poolIdx` maps a **capacity** (or requested size) to the smallest tier that can hold it.

### Bounded tier fallback (`GetSized`)

Each request checks its smallest fitting tier first, then larger tiers. When a
larger buffer is reused, `GetSized` returns the requested prefix and puts a tail
of at least 2 KiB back into the appropriate tier. The prefix capacity is capped
at its length, keeping simultaneously borrowed pieces disjoint. Because sized
tiers stop at 2 MiB and oversized buffers are dropped, one borrowed piece cannot
pin an unbounded allocation. A complete miss allocates the target tier.

### Typed per-P storage

```go
func poolSharedLimit(idx int) int {
	return max(8, 256>>uint(idx))
}
```

Each P has one private slot and one typed lock-free shared chain. Smaller tiers
(hotter paths) get larger per-P shared limits; larger tiers get smaller limits.
This removes channel contention and avoids the `any` boxing allocation incurred
by storing `[]byte` in `sync.Pool`. Only weak-reference metadata is retained:
the maximum number of entries is `(shared limit + 1) * GOMAXPROCS` per tier.

The implementation adapts Go's `sync.Pool` and `poolChain` algorithms and calls
`runtime.procPin`/`runtime.procUnpin` through linkname declarations. This is a
deliberate performance/toolchain coupling; Go upgrades must run the pool tests,
race detector, and benchmarks. Race builds use locked shared heads and omit the
private slot so the race detector can verify the queue operations.

### `Put`

- **Unsized:** `Put` stores a weak handle; if the local P's shared queue is full, the buffer is dropped.
- **Sized:** `Put` routes by **capacity** to one tier. Buffers below the first
  tier or above the last tier are dropped.

## Usage patterns

### Unsized pool

Use when buffer sizes vary (e.g. streaming `io.Reader` / `io.Writer`):

```go
var reader io.Reader = // ...
buf := unsizedPool.GetBuffer()
defer unsizedPool.PutBuffer(buf)

if _, err := io.Copy(buf, reader); err != nil {
	return err
}
```

### Sized pool

Use when lengths are predictable (e.g. fixed read buffers):

```go
var reader io.Reader = // ...
bytes := sizedPool.GetSized(size)
defer sizedPool.Put(bytes)

_, err := io.ReadFull(reader, bytes)
if err != nil {
    return err
}
```

### Benchmarks

`BenchmarkSizedPoolPatterns` covers steady reuse, mixed sizes,
concurrent access, and oversized returns. `BenchmarkSizedPoolArchitectures`
compares exact-tier lookup, whole-buffer fallback, and bounded split fallback
for cold tier-skew bursts and oversized returns.

`BenchmarkSizedPoolBackends` compares the typed per-P pool, weak-channel
storage, and standard `sync.Pool` under identical tier and
split policies. `BenchmarkSizedPoolBackendBursts` additionally exercises the
shared chains instead of only private-slot reuse. The typed pool omits the
standard victim cache because weak entries do not retain backing arrays.

`BenchmarkSizedPoolDeadRecovery` measures post-GC recovery episodes: limiting
a pull to eight dead entries causes 32 consecutive misses for a full 256-entry
sized tier and 512 misses for the 4096-entry unsized pool, while draining the
tier causes one miss. Production therefore drains dead entries until finding a
live buffer or an empty queue.

Recorded implementation comparison on Go 1.26.5, linux/amd64, Intel i5-13500
(`count=8`, median):
cells show time, B/op, and allocs/op.

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
unrestricted splitting. A single unrestricted fallback pins the 2 MiB seed while borrowed;
the 8× limit leaves that weak entry unused and allocates a 2 KiB buffer.

| 2 MiB → 2 KiB | Exact tier | Whole fallback | Split fallback | Split max 8× |
| --- | ---: | ---: | ---: | ---: |
| One request | 324 ns, 2 KiB, 1 alloc | 203 ns, 0 B, 0 allocs | 234 ns, 0 B, 0 allocs | 348 ns, 2 KiB, 1 alloc |
| 32-request burst | 6.88 µs, 64 KiB, 32 allocs | 9.75 µs, 62 KiB, 31 allocs | 4.77 µs, 0 B, 0 allocs | 7.57 µs, 64 KiB, 32 allocs |

The limit bounds pinning but restores every avoided allocation in a skewed
burst. Because sized backing arrays are already capped at 2 MiB, production
keeps unrestricted in-range splitting.

For a 64 MiB oversized return, dropping takes 1.37 ns median; recursive
splitting takes 1.13 µs and creates reusable slices that can pin the entire
oversized backing allocation. Sized fallback therefore splits only allocations
already bounded by the 2 MiB maximum tier.

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

Run focused pool benchmarks from `goutils/` with:

```sh
shadowtree test ./synk -run=^$ -bench='^BenchmarkSizedPool(Patterns|Architectures)$' -benchmem -count=8
shadowtree test ./synk -run=^$ -bench='^BenchmarkSizedPoolBackend(s|Bursts)$' -benchmem -count=8 -cpu=1,8
shadowtree test ./synk -run=^$ -bench='^BenchmarkSizedPoolDeadRecovery$' -benchmem -benchtime=100x
```
