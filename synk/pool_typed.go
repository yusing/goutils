// Copyright 2013 The Go Authors. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
//   * Redistributions of source code must retain the above copyright notice,
//     this list of conditions and the following disclaimer.
//   * Redistributions in binary form must reproduce the above copyright
//     notice, this list of conditions and the following disclaimer in the
//     documentation and/or other materials provided with the distribution.
//   * Neither the name of Google LLC nor the names of its contributors may be
//     used to endorse or promote products derived from this software without
//     specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.
//
// This typed pool adapts Go's sync.Pool, poolChain, and poolDequeue algorithms.
// It stores weakBuf directly instead of boxing values as any, and omits victim
// caches because weakBuf does not retain its buffer.
// Source: Go 1.26.5 src/sync/pool.go (Pool, poolLocal) and
// src/sync/poolqueue.go (poolDequeue, poolChain).

package synk

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	typedPoolDequeueBits  = 32
	typedPoolDequeueLimit = (1 << typedPoolDequeueBits) / 4
)

type typedPoolSlot struct {
	value    weakBuf
	occupied atomic.Bool
}

type typedPoolDequeue struct {
	headTail atomic.Uint64
	vals     []typedPoolSlot
	mask     uint32
}

func (d *typedPoolDequeue) unpack(ptrs uint64) (head, tail uint32) {
	const mask = 1<<typedPoolDequeueBits - 1
	return uint32((ptrs >> typedPoolDequeueBits) & mask), uint32(ptrs & mask)
}

func (d *typedPoolDequeue) pack(head, tail uint32) uint64 {
	const mask = 1<<typedPoolDequeueBits - 1
	return uint64(head)<<typedPoolDequeueBits | uint64(tail&mask)
}

func (d *typedPoolDequeue) pushHead(value weakBuf) bool {
	ptrs := d.headTail.Load()
	head, tail := d.unpack(ptrs)
	if (tail+d.mask+1)&(1<<typedPoolDequeueBits-1) == head {
		return false
	}

	slot := &d.vals[head&d.mask]
	if slot.occupied.Load() {
		return false
	}

	slot.value = value
	slot.occupied.Store(true)
	d.headTail.Add(1 << typedPoolDequeueBits)
	return true
}

func (d *typedPoolDequeue) popHead() (weakBuf, bool) {
	var slot *typedPoolSlot
	for {
		ptrs := d.headTail.Load()
		head, tail := d.unpack(ptrs)
		if tail == head {
			return weakBuf{}, false
		}

		head--
		if d.headTail.CompareAndSwap(ptrs, d.pack(head, tail)) {
			slot = &d.vals[head&d.mask]
			break
		}
	}

	value := slot.value
	slot.value = weakBuf{}
	slot.occupied.Store(false)
	return value, true
}

func (d *typedPoolDequeue) popTail() (weakBuf, bool) {
	var slot *typedPoolSlot
	for {
		ptrs := d.headTail.Load()
		head, tail := d.unpack(ptrs)
		if tail == head {
			return weakBuf{}, false
		}

		if d.headTail.CompareAndSwap(ptrs, d.pack(head, tail+1)) {
			slot = &d.vals[tail&d.mask]
			break
		}
	}

	value := slot.value
	slot.value = weakBuf{}
	slot.occupied.Store(false)
	return value, true
}

type typedPoolChain struct {
	head *typedPoolChainElt
	tail atomic.Pointer[typedPoolChainElt]
}

type typedPoolChainElt struct {
	typedPoolDequeue

	next atomic.Pointer[typedPoolChainElt]
	prev atomic.Pointer[typedPoolChainElt]
}

func (c *typedPoolChain) pushHead(value weakBuf) {
	d := c.head
	if d == nil {
		const initialSize = 8
		d = new(typedPoolChainElt)
		d.vals = make([]typedPoolSlot, initialSize)
		d.mask = initialSize - 1
		c.head = d
		c.tail.Store(d)
	}

	if d.pushHead(value) {
		return
	}

	newSize := min((d.mask+1)*2, typedPoolDequeueLimit)
	d2 := new(typedPoolChainElt)
	d2.prev.Store(d)
	d2.vals = make([]typedPoolSlot, newSize)
	d2.mask = newSize - 1
	c.head = d2
	d.next.Store(d2)
	d2.pushHead(value)
}

func (c *typedPoolChain) popHead() (weakBuf, bool) {
	for d := c.head; d != nil; d = d.prev.Load() {
		if value, ok := d.popHead(); ok {
			return value, true
		}
	}
	return weakBuf{}, false
}

func (c *typedPoolChain) popTail() (weakBuf, bool) {
	d := c.tail.Load()
	if d == nil {
		return weakBuf{}, false
	}

	for {
		d2 := d.next.Load()
		if value, ok := d.popTail(); ok {
			return value, true
		}
		if d2 == nil {
			return weakBuf{}, false
		}
		if c.tail.CompareAndSwap(d, d2) {
			d2.prev.Store(nil)
		}
		d = d2
	}
}

type typedPoolLocalInternal struct {
	private    weakBuf
	privateSet bool
	shared     typedPoolChain
	sharedSize atomic.Int64
	headMu     sync.Mutex
}

type typedPoolLocal struct {
	typedPoolLocalInternal

	pad [128 - unsafe.Sizeof(typedPoolLocalInternal{})%128]byte
}

type typedWeakPool struct {
	mu        sync.Mutex
	local     unsafe.Pointer
	localSize uintptr
	limit     int64
}

func newTypedWeakPool(limit int) *typedWeakPool {
	if limit < 1 {
		panic("synk: typed pool limit must be positive")
	}
	return &typedWeakPool{limit: int64(limit)}
}

func (p *typedWeakPool) Put(value weakBuf) bool {
	l, _ := p.pin()
	if typedPoolRaceEnabled {
		if !l.reserveShared(p.limit) {
			typedRuntimeProcUnpin()
			return false
		}
		l.headMu.Lock()
		l.shared.pushHead(value)
		l.headMu.Unlock()
		typedRuntimeProcUnpin()
		return true
	}
	if !l.privateSet {
		l.private = value
		l.privateSet = true
	} else {
		if !l.reserveShared(p.limit) {
			typedRuntimeProcUnpin()
			return false
		}
		l.shared.pushHead(value)
	}
	typedRuntimeProcUnpin()
	return true
}

func (l *typedPoolLocal) reserveShared(limit int64) bool {
	if l.sharedSize.Add(1) <= limit {
		return true
	}
	l.sharedSize.Add(-1)
	return false
}

func (p *typedWeakPool) Get() (weakBuf, bool) {
	l, pid := p.pin()
	if typedPoolRaceEnabled {
		l.headMu.Lock()
		value, ok := l.shared.popHead()
		l.headMu.Unlock()
		if ok {
			l.sharedSize.Add(-1)
		} else {
			value, ok = p.getSlow(pid)
		}
		typedRuntimeProcUnpin()
		return value, ok
	}
	if l.privateSet {
		value := l.private
		l.private = weakBuf{}
		l.privateSet = false
		typedRuntimeProcUnpin()
		return value, true
	}

	value, ok := l.shared.popHead()
	if ok {
		l.sharedSize.Add(-1)
	} else {
		value, ok = p.getSlow(pid)
	}
	typedRuntimeProcUnpin()
	return value, ok
}

func (p *typedWeakPool) getSlow(pid int) (weakBuf, bool) {
	size := atomic.LoadUintptr(&p.localSize)
	locals := atomic.LoadPointer(&p.local)
	for i := range int(size) {
		l := indexTypedPoolLocal(locals, (pid+i+1)%int(size))
		if value, ok := l.shared.popTail(); ok {
			l.sharedSize.Add(-1)
			return value, true
		}
	}
	return weakBuf{}, false
}

func (p *typedWeakPool) pin() (*typedPoolLocal, int) {
	if p == nil {
		panic("nil typedWeakPool")
	}

	pid := typedRuntimeProcPin()
	size := atomic.LoadUintptr(&p.localSize)
	locals := atomic.LoadPointer(&p.local)
	if uintptr(pid) < size {
		return indexTypedPoolLocal(locals, pid), pid
	}
	return p.pinSlow()
}

func (p *typedWeakPool) pinSlow() (*typedPoolLocal, int) {
	typedRuntimeProcUnpin()
	p.mu.Lock()
	defer p.mu.Unlock()

	pid := typedRuntimeProcPin()
	size := p.localSize
	if uintptr(pid) < size {
		return indexTypedPoolLocal(p.local, pid), pid
	}

	locals := make([]typedPoolLocal, runtime.GOMAXPROCS(0))
	atomic.StorePointer(&p.local, unsafe.Pointer(&locals[0]))
	atomic.StoreUintptr(&p.localSize, uintptr(len(locals)))
	return &locals[pid], pid
}

func indexTypedPoolLocal(locals unsafe.Pointer, i int) *typedPoolLocal {
	ptr := unsafe.Add(locals, uintptr(i)*unsafe.Sizeof(typedPoolLocal{}))
	return (*typedPoolLocal)(ptr)
}

//go:linkname typedRuntimeProcPin runtime.procPin
func typedRuntimeProcPin() int

//go:linkname typedRuntimeProcUnpin runtime.procUnpin
func typedRuntimeProcUnpin()
