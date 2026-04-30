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

For sizes in the sized pool’s normal range, **do not append** to the slice returned from `GetSized`; the implementation may rely on exact length and capacity for pooling (`GetSized` documents this as undefined behavior if you append).

## Design Philosophy

This pool implementation follows a tiered memory management strategy with several key goals:

- Minimize allocations and reduce GC pressure — reuse buffers instead of allocating new ones
- Reduce memory waste — match buffer sizes to actual needs through tiered sizing
- Prevent memory leaks — use weak references so collected buffers can disappear from pool slots
- High performance — use buffered channels and `weak`/`unsafe` plumbing with minimal locking

## Core Architecture

### Dual pool system

#### `UnsizedBytesPool`

- Single channel-backed pool for general-purpose buffers
- New buffers start at `MinAllocSize` (4 KiB)
- Suited to variable-size use cases (`io.Copy`, and similar)

#### `SizedBytesPool`

- A **small-size channel** (`smallPool`) for requested lengths **below** the smallest tier capacity (see tiers below)
- **Eleven tiered pools** whose nominal capacities are `allocSizes[i] = 1024 * (2 << i)` for `i = 0 … 10` (2 KiB through 2 MiB)
- Requests **larger** than the largest tier allocate a dedicated `[]byte` with `make`; on `Put`, oversize backing stores are **split in half** recursively until each piece fits a tier (large allocations are rare)

### Weak reference mechanism

The pool stores `weakBuf` values in channels:

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

### Smart buffer splitting (`GetSized`)

When a reused buffer is larger than the requested length, the tail may be returned to the pool:

```go
remainingSize := capB - size
if remainingSize >= p.min {
	p.put(b[size:], true)
	return b[:size:size]
}
return b[:size]
```

- If the **remainder** is at least `p.min`, the tail is `Put` back and the caller gets `b[:size:size]` so the visible **capacity equals the length** (consistent pooling for that logical size).
- If the remainder would be **smaller than `p.min`**, it is not pooled; the caller receives `b[:size]` and keeps the full backing capacity so `Put` can still route the buffer to the correct tier.

### Channel sizing

```go
func poolChannelSize(idx int) int {
	return max(8, 256>>uint(idx))
}
```

Smaller tiers (hotter paths) get larger channels; larger tiers get smaller channels to bound memory.

### `Put`

- **Unsized:** `Put` enqueues a weak handle; if the channel is full, the buffer is dropped.
- **Sized:** `Put` routes by **capacity** to `smallPool`, a tier channel, or splits oversized caps in half until each part fits a tier.

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

Benchmarks might not be exhaustive, but they are representative of the performance of the pool.

#### Randomly sized buffers within 4MB

| Benchmark      | Iterations | ns/op   | B/op      | allocs/op |
| -------------- | ---------- | ------- | --------- | --------- |
| GetAll/unsized | 2,236,105  | 555.9   | 34        | 2         |
| GetAll/sized   | 842,488    | 1,425   | 90        | 4         |
| GetAll/make    | 6,498      | 194,062 | 1,039,898 | 1         |

#### Randomly sized buffers (may exceed 4MB)

| Benchmark                | Iterations | ns/op   | B/op      | allocs/op |
| ------------------------ | ---------- | ------- | --------- | --------- |
| GetAllExceedsMax/unsized | 2,203,759  | 544.3   | 37        | 2         |
| GetAllExceedsMax/sized   | 1,312,588  | 941.9   | 72        | 3         |
| GetAllExceedsMax/make    | 3,937      | 336,113 | 2,126,743 | 1         |

#### Concurrent allocations

| Benchmark          | Iterations | ns/op   | ns/op_alloc | ns/op_total | ns/op_work | B/op    | allocs/op |
| ------------------ | ---------- | ------- | ----------- | ----------- | ---------- | ------- | --------- |
| workers-1-unsized  | 3,978      | 294,875 | 1,302       | 293,926     | 292,624    | 3,408   | 3         |
| workers-1-sized    | 4,286      | 296,334 | 1,237       | 295,218     | 293,981    | 2,930   | 5         |
| workers-1-make     | 3,340      | 415,523 | 97,299      | 415,409     | 318,110    | 492,239 | 2         |
| workers-2-unsized  | 8,292      | 147,362 | 830.0       | 294,031     | 293,201    | 3,269   | 2         |
| workers-2-sized    | 8,336      | 148,185 | 1,176       | 295,486     | 294,310    | 2,436   | 5         |
| workers-2-make     | 5,412      | 219,521 | 93,075      | 440,315     | 347,240    | 493,141 | 2         |
| workers-4-unsized  | 15,853     | 74,667  | 1,378       | 298,151     | 296,773    | 3,417   | 2         |
| workers-4-sized    | 16,075     | 75,170  | 2,202       | 299,441     | 297,239    | 6,767   | 5         |
| workers-4-make     | 9,884      | 115,854 | 81,081      | 462,411     | 381,330    | 486,525 | 1         |
| workers-8-unsized  | 28,244     | 42,162  | 1,735       | 336,454     | 334,719    | 3,830   | 2         |
| workers-8-sized    | 26,013     | 46,224  | 2,310       | 368,261     | 365,951    | 4,932   | 5         |
| workers-8-make     | 16,420     | 71,518  | 108,927     | 570,788     | 461,861    | 496,645 | 1         |
| workers-16-unsized | 43,272     | 29,189  | 3,696       | 463,896     | 460,200    | 4,983   | 2         |
| workers-16-sized   | 36,670     | 34,641  | 6,803       | 548,070     | 541,267    | 7,382   | 5         |
| workers-16-make    | 19,166     | 65,258  | 299,753     | 1,011,464   | 711,711    | 489,229 | 2         |
| workers-32-unsized | 32,059     | 38,423  | 19,663      | 749,792     | 730,129    | 13,229  | 2         |
| workers-32-sized   | 35,071     | 37,116  | 23,897      | 726,144     | 702,247    | 15,378  | 5         |
| workers-32-make    | 19,437     | 63,402  | 578,680     | 1,356,542   | 777,862    | 466,336 | 2         |
