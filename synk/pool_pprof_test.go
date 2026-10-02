//go:build pprof

package synk

import (
	"testing"

	expect "github.com/yusing/goutils/testing"
)

func TestSizeInUse(t *testing.T) {
	pool := UnsizedBytesPool{pool: newTypedWeakPool(1)}
	before := sizeInUse.Load()

	b := pool.GetAtLeast(2 * MinAllocSize)
	expect.Equal(t, sizeInUse.Load(), before+uint64(cap(b)))

	pool.Put(b)
	expect.Equal(t, sizeInUse.Load(), before)

	b = pool.Get()
	b = b[:0:1]
	pool.Put(b)
	expect.Equal(t, sizeInUse.Load(), before, "return must remove the originally tracked capacity")

	pool.Put(make([]byte, MinAllocSize))
	expect.Equal(t, sizeInUse.Load(), before, "foreign buffers must not underflow the metric")
}
