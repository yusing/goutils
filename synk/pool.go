package synk

import (
	"bytes"
	"math/bits"
	"slices"
	"unsafe"
	"weak"

	"github.com/puzpuzpuz/xsync/v4"
)

type UnsizedBytesPool struct {
	pool chan weakBuf
}

type SizedBytesPool struct {
	// 1024*(2<<i) bytes
	// 4KiB, 8KiB, 16KiB, 32KiB, 64KiB
	// , 128KiB, 256KiB, 512KiB, 1MiB, 2MiB, 4MiB
	pools     [SizedPools]chan weakBuf
	smallPool chan weakBuf // everything smaller than allocSize(0)
	largePool chan weakBuf // everything larger than allocSize(SizedPools-1)

	min, max int
}

const (
	kb = 1024
	mb = 1024 * kb
)

const (
	UnsizedPoolLimit = 16 * mb

	MinAllocSize    = 4 * kb
	UnsizedPoolSize = UnsizedPoolLimit / MinAllocSize

	SizedPools           = 11
	SmallPoolChannelSize = UnsizedPoolSize
	LargePoolChannelSize = 16
)

// poolChannelSize returns the channel size for a given pool index.
// Smaller buffers (lower idx) are used more frequently, so they get larger channels.
func poolChannelSize(idx int) int {
	return max(8, 256>>uint(idx))
}

var (
	unsizedBytesPool UnsizedBytesPool
	sizedBytesPool   SizedBytesPool
	sizedFullCaps    *xsync.Map[*byte, int]
)

var allocSizes [SizedPools]int

func allocSize(idx int) int {
	return allocSizes[idx]
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
	sizedFullCaps = xsync.NewMap[*byte, int]()

	unsizedBytesPool.pool = make(chan weakBuf, UnsizedPoolSize)

	for i := range allocSizes {
		allocSizes[i] = 1024 * (2 << i)
	}

	sizedBytesPool.min = allocSize(0)
	sizedBytesPool.max = allocSize(SizedPools - 1)
	sizedBytesPool.smallPool = make(chan weakBuf, SmallPoolChannelSize)
	sizedBytesPool.largePool = make(chan weakBuf, LargePoolChannelSize)
	for i := range sizedBytesPool.pools {
		sizedBytesPool.pools[i] = make(chan weakBuf, poolChannelSize(i))
	}

	// Initialize sync pool version
	initSizedBytesPoolSync()

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

func (p UnsizedBytesPool) PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	p.Put(buf.Bytes())
}

func (p *SizedBytesPool) GetBuffer(size int) *bytes.Buffer {
	return bytes.NewBuffer(p.GetSized(size))
}

// PutBuffer resets and puts the buffer into the pool.
func (p *SizedBytesPool) PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	p.Put(buf.Bytes())
}

func (p UnsizedBytesPool) Get() []byte {
	for {
		select {
		case bWeak := <-p.pool:
			b := getBufFromWeak(bWeak)
			if b == nil {
				continue
			}
			addReused(cap(b))
			return b[:0]
		default:
			addNonPooled(MinAllocSize)
			return make([]byte, 0, MinAllocSize)
		}
	}
}

// GetSized returns a slice of the given size.
// If the size is 0, the returned slice is from the unsized pool.
// Calling append to returned slice will cause undefined behavior.
func (p *SizedBytesPool) GetSized(size int) []byte {
	if size < p.min {
		return pullOrGrow(p.smallPool, size)
	}

	if size > p.max {
		return pullOrGrow(p.largePool, size)
	}

	targetIdx := poolIdx(size)
	idx := targetIdx
	for idx < SizedPools {
		select {
		case bWeak := <-p.pools[idx]:
			b := getBufFromWeak(bWeak)
			if b == nil {
				continue // try same pool again
			}

			// FIXME: this should not happen, but it does
			// cap(p.pools[poolIdx(size)]) should be >= allocSize(idx)
			capB := cap(b)
			if capB < size {
				p.put(b, false)
				continue
			}
			addReused(size)
			b = b[:capB] // set len to cap for further slicing

			remainingSize := capB - size
			if remainingSize >= p.min { // remaining part > smallest pool size
				p.put(b[size:], true)
				return b[:size:size]
			}
			return b[:size]
		default:
			idx++ // try next pool if no buffer in current pool
		}
	}

	capacity := allocSize(targetIdx)
	addNonPooled(capacity)
	// Allocate a buffer with the exact pool capacity to ensure it's returned
	// to the correct pool (targetIdx) when released, avoiding misplacement
	// in a smaller pool.
	buf := make([]byte, capacity)
	return buf[:size]
}

//go:inline
func (p UnsizedBytesPool) Put(b []byte) {
	put(b, p.pool)
}

func (p *SizedBytesPool) Put(b []byte) {
	p.put(b, false)
}

func (p *SizedBytesPool) put(b []byte, isRemaining bool) {
	capB := cap(b)

	if capB < p.min {
		put(b, p.smallPool)
		return
	}

	if capB <= p.max {
		idx := poolIdx(capB)
		// e.g. cap=8190, allocSize will be 8192, so we need to put it in the previous pool
		// since the `if capB < p.min` check has already failed,
		// capB < allocSize(idx) only happens when idx > 0
		if capB < allocSize(idx) {
			idx--
		}
		put(b, p.pools[idx])
		if isRemaining {
			addReusedRemaining(capB)
		}
		return
	}

	put(b, p.largePool)
}

func pullOrGrow(pool chan weakBuf, size int) []byte {
	for {
		select {
		case bWeak := <-pool:
			b := getBufFromWeak(bWeak)
			if b == nil {
				continue
			}
			capB := cap(b)
			if capB < size {
				addDropped(capB)
				addNonPooled(size)
				// slices.Grow may reallocate; discard old buffer to avoid reuse
				newB := slices.Grow(b[:0], size)
				return newB[:size]
			}
			addReused(capB)
			return b[:size]
		default:
			addNonPooled(size)
			return make([]byte, size)
		}
	}
}

//go:inline
func put(b []byte, pool chan weakBuf) {
	w := makeWeak(b)

	select {
	case pool <- w:
	default:
		addDropped(w.cap)
		// just drop it
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
