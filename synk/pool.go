package synk

import (
	"bytes"
	"math/bits"
	"unsafe"
	"weak"
)

type UnsizedBytesPool struct {
	pool *typedWeakPool
}

type SizedBytesPool struct {
	// 2KiB, 4KiB, 8KiB, 16KiB, 32KiB, 64KiB,
	// 128KiB, 256KiB, 512KiB, 1MiB, 2MiB.
	pools [SizedPools]*typedWeakPool
}

const (
	kb = 1024
	mb = 1024 * kb
)

const (
	UnsizedPoolLimit = 16 * mb

	MinAllocSize    = 4 * kb
	UnsizedPoolSize = UnsizedPoolLimit / MinAllocSize

	SizedPools = 11
)

// poolSharedLimit returns the per-P shared queue limit for a pool index.
// Smaller buffers (lower idx) are used more frequently, so they get larger queues.
func poolSharedLimit(idx int) int {
	return max(8, 256>>uint(idx))
}

var (
	unsizedBytesPool UnsizedBytesPool
	sizedBytesPool   SizedBytesPool
)

func allocSize(idx int) int {
	return 2 * kb << idx
}

// poolIdx returns the index of the pool that guarantees the pool size is greater than or equal to the given size.
func poolIdx(size int) int {
	if size <= 0 {
		return 0
	}
	return min(SizedPools-1, max(0, bits.Len(uint(size-1))-11))
}

func init() {
	initAll()
}

func initAll() {
	unsizedBytesPool.pool = newTypedWeakPool(UnsizedPoolSize)

	for i := range sizedBytesPool.pools {
		sizedBytesPool.pools[i] = newTypedWeakPool(poolSharedLimit(i))
	}

	initPoolStats()
}

func GetUnsizedBytesPool() UnsizedBytesPool {
	return unsizedBytesPool
}

func GetSizedBytesPool() *SizedBytesPool {
	return &sizedBytesPool
}

func (p UnsizedBytesPool) GetBuffer() *bytes.Buffer {
	return bytes.NewBuffer(p.Get())
}

func (p UnsizedBytesPool) GetBufferAtLeast(size int) *bytes.Buffer {
	return bytes.NewBuffer(p.GetAtLeast(size))
}

// PutBuffer resets buf and returns it to the pool. The caller must not access
// buf after PutBuffer. Buffer contents are not cleared before reuse.
func (p UnsizedBytesPool) PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	p.Put(buf.Bytes())
}

func (p *SizedBytesPool) GetBuffer(size int) *bytes.Buffer {
	return bytes.NewBuffer(p.GetSized(size)[:0])
}

// PutBuffer resets buf and returns it to the pool. The caller must not access
// buf after PutBuffer. Buffer contents are not cleared before reuse.
func (p *SizedBytesPool) PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	p.Put(buf.Bytes())
}

func (p UnsizedBytesPool) Get() []byte {
	b := pull(p.pool, 0)
	if b != nil {
		b = b[:0]
		addSizeInUse(b)
		return b
	}

	addNonPooled(MinAllocSize)
	b = make([]byte, 0, MinAllocSize)
	addSizeInUse(b)
	return b
}

func (p UnsizedBytesPool) GetAtLeast(n int) []byte {
	b := p.Get()
	if n <= cap(b) {
		return b
	}
	// discard the buffer
	removeSizeInUse(b)
	b = make([]byte, 0, n)
	addSizeInUse(b)
	return b
}

// GetSized returns a slice of the given size.
// If size is 0, the returned slice has zero length and the capacity of the
// smallest sized tier.
func (p *SizedBytesPool) GetSized(size int) []byte {
	if size > allocSize(SizedPools-1) {
		addNonPooled(size)
		b := make([]byte, size)
		addSizeInUse(b)
		return b
	}

	targetIdx := poolIdx(size)
	for idx := targetIdx; idx < SizedPools; idx++ {
		buf := pull(p.pools[idx], size)
		if buf == nil {
			continue
		}

		capB := cap(buf)
		buf = buf[:capB]
		if size > 0 && capB-size >= allocSize(0) {
			p.put(buf[size:])
			buf = buf[:size:size]
		} else {
			buf = buf[:size]
		}
		addSizeInUse(buf)
		return buf
	}

	capacity := allocSize(targetIdx)
	addNonPooled(capacity)
	buf := make([]byte, size, capacity)
	addSizeInUse(buf)
	return buf
}

// Put returns b to the pool. The caller must not access b after Put.
// Buffer contents are not cleared before reuse.
//
//go:inline
func (p UnsizedBytesPool) Put(b []byte) {
	removeSizeInUse(b)
	put(b, p.pool)
}

// Put returns b to the pool. The caller must not access b after Put.
// Buffer contents are not cleared before reuse.
func (p *SizedBytesPool) Put(b []byte) {
	removeSizeInUse(b)
	p.put(b)
}

func (p *SizedBytesPool) put(b []byte) {
	capB := cap(b)

	if capB < allocSize(0) || capB > allocSize(SizedPools-1) {
		addDropped(capB)
		return
	}

	idx := poolIdx(capB)
	// Capacities between tiers belong to the largest tier they can satisfy.
	if capB < allocSize(idx) {
		idx--
	}
	put(b, p.pools[idx])
}

func pull(pool *typedWeakPool, size int) []byte {
	for {
		bWeak, ok := pool.Get()
		if !ok {
			return nil
		}
		b := getBufFromWeak(bWeak)
		if b == nil {
			continue
		}
		capB := cap(b)
		if capB < size {
			addDropped(capB)
			continue
		}
		addReused(capB)
		return b
	}
}

//go:inline
func put(b []byte, pool *typedWeakPool) {
	w := makeWeak(b)
	if !pool.Put(w) {
		addDropped(w.cap)
	}
}

type weakBuf struct {
	ptr weak.Pointer[byte]
	cap int
}

//go:inline
func makeWeak(b []byte) weakBuf {
	return weakBuf{
		ptr: weak.Make(unsafe.SliceData(b)),
		cap: cap(b),
	}
}

//go:inline
func getBufFromWeak(w weakBuf) []byte {
	ptr := w.ptr.Value()
	if ptr != nil {
		return unsafe.Slice(ptr, w.cap)
	}

	// nil pointer returned from weak.Pointer.Value()
	// means the buffer is garbage collected
	addGced(w.cap)
	return nil
}
