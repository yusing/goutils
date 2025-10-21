package synk

import (
	"bytes"
	"slices"
	"sync"
)

type SizedBytesPoolSync struct {
	// 1024*(2<<i) bytes
	// 4KiB, 8KiB, 16KiB, 32KiB, 64KiB, 128KiB, 256KiB, 512KiB, 1MiB, 2MiB, 4MiB
	pools     [SizedPools]sync.Pool
	smallPool sync.Pool
	largePool sync.Pool
	min, max  int
}

var sizedBytesPoolSync SizedBytesPoolSync

func initSizedBytesPoolSync() {
	for i := range sizedBytesPoolSync.pools {
		sizedBytesPoolSync.pools[i] = sync.Pool{}
	}
	sizedBytesPoolSync.min = allocSize(0)
	sizedBytesPoolSync.max = allocSize(SizedPools - 1)

	sizedBytesPoolSync.smallPool = sync.Pool{}
	sizedBytesPoolSync.largePool = sync.Pool{}
}

func GetSizedBytesPoolSync() *SizedBytesPoolSync {
	return &sizedBytesPoolSync
}

func (p *SizedBytesPoolSync) GetBuffer(size int) *bytes.Buffer {
	return bytes.NewBuffer(p.GetSized(size))
}

// PutBuffer resets and puts the buffer into the pool.
func (p *SizedBytesPoolSync) PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	p.Put(buf.Bytes())
}

// GetSized returns a slice of the given size.
// If the size is 0, the returned slice is from the unsized pool.
// Calling append to returned slice will cause undefined behavior.
func (p *SizedBytesPoolSync) GetSized(size int) []byte {
	if size < p.min {
		return pullOrGrowSync(&p.smallPool, size)
	}

	if size > p.max {
		return pullOrGrowSync(&p.largePool, size)
	}

	targetIdx := poolIdx(size)
	idx := targetIdx
	for idx < SizedPools {
		if bWeak := p.pools[idx].Get(); bWeak != nil {
			b := getBufFromWeakSync(bWeak)
			if b == nil {
				continue // try same pool again
			}
			addReused(size)

			capB := cap(b)
			b = b[:capB] // set len to cap for further slicing

			remainingSize := capB - size
			if remainingSize >= p.min { // remaining part > smallest pool size
				p.put(b[size:], true)
				front := b[:size:size]
				storeFullCap(front, capB)
				return front
			}
			return b[:size]
		}
		idx++ // try next pool if no buffer in current pool
	}

	capacity := allocSize(targetIdx)
	addNonPooled(capacity)
	// Allocate a buffer with the exact pool capacity to ensure it's returned
	// to the correct pool (targetIdx) when released, avoiding misplacement
	// in a smaller pool.
	buf := make([]byte, capacity)
	return buf[:size]
}

func (p *SizedBytesPoolSync) Put(b []byte) {
	p.put(b, false)
}

func (p *SizedBytesPoolSync) put(b []byte, isRemaining bool) {
	if !isRemaining {
		b = withFullCap(b)
	}
	capB := cap(b)
	bWeak := makeWeak(b)

	if capB < p.min {
		p.smallPool.Put(&bWeak)
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
		p.pools[idx].Put(&bWeak)
		if isRemaining {
			addReusedRemaining(capB)
		}
		return
	}

	p.largePool.Put(&bWeak)
}

func pullOrGrowSync(pool *sync.Pool, size int) []byte {
	for {
		bWeak := pool.Get()
		if bWeak != nil {
			b := getBufFromWeakSync(bWeak)
			if b == nil {
				continue
			}
			capB := cap(b)
			if capB < size {
				addDropped(size - capB)
				addNonPooled(size - capB)
				newB := slices.Grow(b, size)
				return newB[:size]
			}
			addReused(capB)
			return b[:size]
		}
		addNonPooled(size)
		return make([]byte, size)
	}
}

//go:inline
func getBufFromWeakSync(w any) []byte {
	return getBufFromWeak(*w.(*weakBuf))
}
