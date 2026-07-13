package synk

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testBytesPool = GetSizedBytesPool()

func underlyingPtr(b []byte) uintptr {
	return reflect.ValueOf(b).Pointer()
}

func TestUnsized(t *testing.T) {
	t.Cleanup(initAll)
	b := unsizedBytesPool.Get()
	assert.Equal(t, cap(b), MinAllocSize)
	unsizedBytesPool.Put(b)
	assert.Equal(t, underlyingPtr(b), underlyingPtr(unsizedBytesPool.Get()))
}

func TestGetSizedExactMatch(t *testing.T) {
	t.Cleanup(initAll)
	// Test exact size match reuse
	size := allocSize(0)
	b1 := testBytesPool.GetSized(size)
	assert.Equal(t, size, len(b1))
	assert.Equal(t, size, cap(b1))

	// Put back into pool
	testBytesPool.Put(b1)

	// Get same size - should reuse the same buffer
	b2 := testBytesPool.GetSized(size)
	assert.Equal(t, size, len(b2))
	assert.Equal(t, size, cap(b2))
	assert.Equal(t, underlyingPtr(b1), underlyingPtr(b2))
}

func TestGetSizedFallsBackAndSplitsLargerTier(t *testing.T) {
	t.Cleanup(initAll)

	largeSize := allocSize(1)
	large := testBytesPool.GetSized(largeSize)
	testBytesPool.Put(large)

	small := testBytesPool.GetSized(allocSize(0))
	assert.Equal(t, underlyingPtr(large), underlyingPtr(small))
	assert.Equal(t, len(small), cap(small))

	remainder := testBytesPool.GetSized(allocSize(0))
	assert.Equal(t, underlyingPtr(large)+uintptr(len(small)), underlyingPtr(remainder))
}

func TestGetSizedZeroOwnsSmallestTier(t *testing.T) {
	t.Cleanup(initAll)

	buf := testBytesPool.GetSized(0)
	assert.Empty(t, buf)
	assert.Equal(t, allocSize(0), cap(buf))
}

func TestGetSizedReusesSubTierBuffer(t *testing.T) {
	t.Cleanup(initAll)

	size := allocSize(0) / 2
	b1 := testBytesPool.GetSized(size)
	testBytesPool.Put(b1)
	b2 := testBytesPool.GetSized(size)

	assert.Equal(t, underlyingPtr(b1), underlyingPtr(b2))
}

func TestSizedPoolDropsOutOfRangeBuffers(t *testing.T) {
	t.Cleanup(initAll)

	for _, capacity := range []int{1, allocSize(SizedPools-1) + 1} {
		t.Run(fmt.Sprintf("capacity=%d", capacity), func(t *testing.T) {
			initAll()
			testBytesPool.Put(make([]byte, capacity))
			for i, pool := range testBytesPool.pools {
				_, ok := pool.Get()
				assert.False(t, ok, "pool %d", i)
			}

			if capacity < allocSize(0) {
				b := testBytesPool.GetSized(1)
				assert.Len(t, b, 1)
				assert.Equal(t, allocSize(0), cap(b))
			}
		})
	}
}

func TestSizedBufferStartsEmpty(t *testing.T) {
	t.Cleanup(initAll)

	buf := testBytesPool.GetBuffer(allocSize(0))
	assert.Zero(t, buf.Len())
	assert.GreaterOrEqual(t, buf.Cap(), allocSize(0))

	_, err := buf.WriteString("payload")
	require.NoError(t, err)
	assert.Equal(t, "payload", buf.String())
	testBytesPool.PutBuffer(buf)
}

func TestGetSizedBufferTooSmall(t *testing.T) {
	t.Cleanup(initAll)
	// Test when pool buffer is smaller than requested size
	smallSize := allocSize(0)
	largeSize := allocSize(1)

	// Put small buffer in pool
	b1 := testBytesPool.GetSized(smallSize)
	assert.Equal(t, smallSize, len(b1))
	assert.Equal(t, smallSize, cap(b1))
	testBytesPool.Put(b1)

	// Request larger size - should create new buffer, not reuse small one
	b2 := testBytesPool.GetSized(largeSize)
	assert.Equal(t, largeSize, len(b2))
	assert.Equal(t, largeSize, cap(b2))
	assert.NotEqual(t, underlyingPtr(b1), underlyingPtr(b2))

	// The small buffer should still be in pool
	b3 := testBytesPool.GetSized(smallSize)
	assert.Equal(t, underlyingPtr(b1), underlyingPtr(b3))
}

func TestPullDropsBufferTooSmall(t *testing.T) {
	pool := newTypedWeakPool(1)
	buf := make([]byte, allocSize(0))
	require.True(t, pool.Put(makeWeak(buf)))

	assert.Nil(t, pull(pool, allocSize(1)))
	runtime.KeepAlive(buf)
}

func TestGetSizedLargeBuffer(t *testing.T) {
	t.Cleanup(initAll)
	largeSize := allocSize(SizedPools-1) * 2
	b := testBytesPool.GetSized(largeSize)
	assert.Equal(t, largeSize, len(b))
	assert.Equal(t, largeSize, cap(b))
	testBytesPool.Put(b)
}

func TestPoolIdx(t *testing.T) {
	for i := range SizedPools {
		size := allocSize(i)
		expectedIdx := i
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			idx := poolIdx(size)
			assert.Equal(t, expectedIdx, idx, "poolIdx(%d) should return %d", size, expectedIdx)
			assert.Equal(t, size, allocSize(idx), "Pool size %d should be %d", size, allocSize(idx))
		})
	}
	t.Run("verify_enough_pool_size", func(t *testing.T) {
		for i := range allocSize(SizedPools - 1) {
			idx := poolIdx(i)
			assert.GreaterOrEqual(t, allocSize(idx), i, "Pool size %d should be >= %d", allocSize(idx), i)
		}
	})
}
