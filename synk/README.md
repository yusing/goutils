# synk/pool

A pool of bytes buffers that are reused instead of allocated and freed.

## Usage

```go
sizedPool := pool.GetSizedPool()
unsizedPool := pool.GetUnsizedPool()

// as []byte
buf := sizedPool.GetSized(size)
defer sizedPool.Put(buf)

buf = unsizedPool.Get()
defer unsizedPool.Put(buf)

// as *bytes.Buffer
buf := unsizedPool.GetBuffer()
defer unsizedPool.PutBuffer(buf)

buf := sizedPool.GetSizedBuffer(size)
defer sizedPool.PutSizedBuffer(buf)
```

## Design Philosophy

This pool implementation follows a tiered memory management strategy with several key goals:

- Minimize allocations and reduce GC pressure - Reuse buffers instead of allocating new ones
- Reduce memory waste - Match buffer sizes to actual needs through tiered sizing
- Prevent memory leaks - Use weak references to allow GC cleanup
- High performance - Use lock-free channels and unsafe optimizations

## Core Architecture

### Dual Pool System

#### UnsizedBytesPool

- Single pool for general-purpose buffers
- All buffers start at MinAllocSize (4KB)
- Good for variable-size use cases

#### SizedBytesPool

- 11 tiered pools: 4KB, 8KB, 16KB, 32KB, 64KB, 128KB, 256KB, 512KB, 1MB, 2MB, 4MB
- Plus a large pool for buffers >4MB (very large buffers are rare)
- Reduces memory waste by size-matching

### Weak Reference Mechanism

The pool uses `weak.Pointer[[]byte]`

```go
type weakBuf = weak.Pointer[[]byte]

func makeWeak(b *[]byte) weakBuf {
    return weak.Make(b)
}
```

Goal:

If the GC needs memory and no strong references exist to a buffer, it can be collected even though the buffer is still in the pool channel. This prevents memory leaks when pools are underutilized.

## Sized Pool Allocation Strategy

### Pool Index Calculation

```go
func (p *SizedBytesPool) poolIdx(size int) int {
    if size <= 0 {
        return 0
    }
    return min(SizedPools-1, max(0, bits.Len(uint(size-1))-11))
}
```

This uses bit manipulation to find the appropriate tier:
`bits.Len(size-1)` finds the position of the highest set bit
Subtract 11 to align with 4KB (2^12) base size
Maps sizes efficiently to the nearest power-of-2 tier

### Smart Buffer Splitting

When a larger buffer is retrieved but only part is needed, the excess is returned to the pool:

```go
remainingSize := capB - size
if remainingSize > p.min { // only split if remainder is useful
    p.put(b[size:], true)  // return excess to pool
    front := b[:size:size]  // use requested portion
    storeFullCap(front, capB)  // remember original capacity
    return front
}
```

### Capacity Restoration System

The pool maintains full capacity information for buffers that have been sliced:

```go
func storeFullCap(b []byte, c int) {
    if c == cap(b) {
        return  // no change needed
    }
    ptr := sliceStruct(&b).ptr
    sizedFullCaps.Store(ptr, c)  // store original capacity
}

func restoreFullCap(b *[]byte) {
    ptr := sliceStruct(b).ptr
    if fullCap, ok := sizedFullCaps.LoadAndDelete(ptr); ok {
        setCap(b, fullCap)  // restore original capacity
    }
}
```

This uses unsafe pointer manipulation to preserve the original capacity across slice operations.

## Performance Optimizations

### Channel Sizing

```go
func poolChannelSize(idx int) int {
    return max(8, 256>>uint(idx))
}
```

Smaller buffers (used more frequently) get larger channels to reduce contention, while larger buffers get smaller channels to save memory.

### Put() operations

```go
func (p UnsizedBytesPool) Put(b []byte) {
    if b == nil {
        return
    }
    put(b, p.pool)
}
```

### Lock-free Operations

All pool operations use channel selects instead of mutexes, enabling concurrent access without blocking.

### Usage Patterns

#### Unsized Pool

Unsized Pool: Good for when buffer sizes vary unpredictably (e.g. io.Reader or io.Writer).

```go
var reader io.Reader = ...
buf := unsizedBytesPool.GetBuffer() // returns a *bytes.Buffer
defer unsizedBytesPool.PutBuffer(buf)

_, err := io.Copy(buf, reader)
if err != nil {
    return err
}
```

#### Sized Pool

Sized Pool: Good for when buffer sizes are known and predictable (e.g. HTTP responses).

```go
var reader io.Reader = ...
bytes := sizedBytesPool.GetSized(size) // returns a []byte
defer sizedBytesPool.Put(bytes)

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
