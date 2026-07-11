//go:build pprof

package synk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSizeInUse(t *testing.T) {
	pool := UnsizedBytesPool{pool: make(chan weakBuf, 1)}
	before := sizeInUse.Load()

	b := pool.GetAtLeast(2 * MinAllocSize)
	assert.Equal(t, before+uint64(cap(b)), sizeInUse.Load())

	pool.Put(b)
	assert.Equal(t, before, sizeInUse.Load())

	b = pool.Get()
	b = b[:0:1]
	pool.Put(b)
	assert.Equal(t, before, sizeInUse.Load(), "return must remove the originally tracked capacity")

	pool.Put(make([]byte, MinAllocSize))
	assert.Equal(t, before, sizeInUse.Load(), "foreign buffers must not underflow the metric")
}
