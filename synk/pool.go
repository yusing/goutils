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
	// 4KiB, 8KiB, 16KiB, 32KiB, 64KiB, 128KiB, 256KiB, 512KiB, 1MiB, 2MiB, 4MiB
	pools     [SizedPools]chan weakBuf
	largePool chan weakBuf
	min, max  int
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
	sizedFullCaps    *xsync.Map[uintptr, int]
)

func allocSize(idx int) int {
	return 1024 * (2 << idx)
}

func init() {
	initAll()
}

func initAll() {
	unsizedBytesPool.pool = make(chan weakBuf, UnsizedPoolSize)
	sizedBytesPool.min = allocSize(0)
	sizedBytesPool.max = allocSize(SizedPools - 1)
	sizedFullCaps = xsync.NewMap[uintptr, int]()
	for i := range sizedBytesPool.pools {
		sizedBytesPool.pools[i] = make(chan weakBuf, poolChannelSize(i))
	}
	sizedBytesPool.largePool = make(chan weakBuf, LargePoolChannelSize)

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
			return b
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
		// min of unsized is MinAllocSize, which is larger than p.min
		b := unsizedBytesPool.Get()
		setLen(&b, size)
		return b
	}

	if size > p.max {
		for {
			select {
			case bWeak := <-p.largePool:
				b := getBufFromWeak(bWeak)
				if b == nil {
					continue
				}
				capB := cap(b)
				if capB < size {
					addDropped(size - capB)
					addNonPooled(size - capB)
					newB := slices.Grow(b, size)
					storeFullCap(newB, cap(newB))
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

	targetIdx := poolIdx(size)
	idx := targetIdx
	for idx < SizedPools {
		select {
		case bWeak := <-p.pools[idx]:
			b := getBufFromWeak(bWeak)
			if b == nil {
				continue // try same pool again
			}
			addReused(size)

			capB := cap(b)
			setLen(&b, capB)

			remainingSize := capB - size
			if remainingSize > p.min { // remaining part > smallest pool size
				p.put(b[size:], true)
				front := b[:size:size]
				storeFullCap(front, capB)
				return front
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
	restoreFullCap(&b)
	capB := cap(b)

	if capB < p.min {
		unsizedBytesPool.Put(b)
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
			addReusedRemaining(b)
		}
		return
	}

	put(b, p.largePool)
}

// poolIdx returns the index of the pool that guarantees the pool size is greater than or equal to the given size.
func poolIdx(size int) int {
	if size <= 0 {
		return 0
	}
	return min(SizedPools-1, max(0, bits.Len(uint(size-1))-11))
}

//go:inline
func put(b []byte, pool chan weakBuf) {
	// TODO: optimize to not call it for unsized bytes
	restoreFullCap(&b)
	size := cap(b)

	b = b[:0]
	w := makeWeak(&b)
	select {
	case pool <- w:
	default:
		addDropped(size)
		// just drop it
	}
}

type weakBuf = weak.Pointer[[]byte]

func makeWeak(b *[]byte) weakBuf {
	return weak.Make(b)
}

//go:inline
func getBufFromWeak(w weakBuf) []byte {
	ptr := w.Value()
	if ptr != nil {
		return *ptr
	}
	return nil
}

// it should be used for sized bytes pool only,
// since unsized bytes can grow and causes entries leaked in sizedFullCaps
func storeFullCap(b []byte, c int) {
	if c <= 0 {
		return
	}
	ptr := sliceStruct(&b).ptr
	if ptr == 0 {
		return
	}
	if c == cap(b) {
		return
	}
	sizedFullCaps.Store(ptr, c)
}

func restoreFullCap(b *[]byte) {
	ptr := sliceStruct(b).ptr
	if ptr == 0 {
		return
	}
	if fullCap, ok := sizedFullCaps.LoadAndDelete(ptr); ok {
		setCap(b, fullCap)
	}
}

type sliceInternal struct {
	ptr uintptr
	len int
	cap int
}

//go:inline
func sliceStruct(b *[]byte) *sliceInternal {
	return (*sliceInternal)(unsafe.Pointer(b))
}

func setLen(b *[]byte, len int) {
	sliceStruct(b).len = len
}

func setCap(b *[]byte, cap int) {
	sliceStruct(b).cap = cap
}
