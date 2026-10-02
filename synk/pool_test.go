package synk

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

var testBytesPool = GetSizedBytesPool()

func underlyingPtr(b []byte) uintptr {
	return reflect.ValueOf(b).Pointer()
}

func TestUnsized(t *testing.T) {
	t.Cleanup(initAll)
	b := unsizedBytesPool.Get()
	expect.Equal(t, MinAllocSize, cap(b))
	unsizedBytesPool.Put(b)
	expect.Equal(t, underlyingPtr(unsizedBytesPool.Get()), underlyingPtr(b))
}

func TestGetSizedExactMatch(t *testing.T) {
	t.Cleanup(initAll)
	// Test exact size match reuse
	size := allocSize(0)
	b1 := testBytesPool.GetSized(size)
	expect.Equal(t, len(b1), size)
	expect.Equal(t, cap(b1), size)

	// Put back into pool
	testBytesPool.Put(b1)

	// Get same size - should reuse the same buffer
	b2 := testBytesPool.GetSized(size)
	expect.Equal(t, len(b2), size)
	expect.Equal(t, cap(b2), size)
	expect.Equal(t, underlyingPtr(b2), underlyingPtr(b1))
}

func TestGetSizedFallsBackAndSplitsLargerTier(t *testing.T) {
	t.Cleanup(initAll)

	largeSize := allocSize(1)
	large := testBytesPool.GetSized(largeSize)
	testBytesPool.Put(large)

	small := testBytesPool.GetSized(allocSize(0))
	expect.Equal(t, underlyingPtr(small), underlyingPtr(large))
	expect.Equal(t, cap(small), len(small))

	remainder := testBytesPool.GetSized(allocSize(0))
	expect.Equal(t, underlyingPtr(remainder), underlyingPtr(large)+uintptr(len(small)))
}

func TestGetSizedZeroOwnsSmallestTier(t *testing.T) {
	t.Cleanup(initAll)

	buf := testBytesPool.GetSized(0)
	expect.Empty(t, buf)
	expect.Equal(t, cap(buf), allocSize(0))
}

func TestGetSizedReusesSubTierBuffer(t *testing.T) {
	t.Cleanup(initAll)

	size := allocSize(0) / 2
	b1 := testBytesPool.GetSized(size)
	testBytesPool.Put(b1)
	b2 := testBytesPool.GetSized(size)

	expect.Equal(t, underlyingPtr(b2), underlyingPtr(b1))
}

func TestSizedPoolDropsOutOfRangeBuffers(t *testing.T) {
	t.Cleanup(initAll)

	for _, capacity := range []int{1, allocSize(SizedPools-1) + 1} {
		t.Run(fmt.Sprintf("capacity=%d", capacity), func(t *testing.T) {
			initAll()
			testBytesPool.Put(make([]byte, capacity))
			for i, pool := range testBytesPool.pools {
				_, ok := pool.Get()
				expect.False(t, ok, "pool %d", i)
			}

			if capacity < allocSize(0) {
				b := testBytesPool.GetSized(1)
				expect.Equal(t, len(b), 1)
				expect.Equal(t, cap(b), allocSize(0))
			}
		})
	}
}

func TestSizedBufferStartsEmpty(t *testing.T) {
	t.Cleanup(initAll)

	buf := testBytesPool.GetBuffer(allocSize(0))
	if buf.Len() != 0 {
		t.Fatalf("expected zero, got %v", buf.Len())
	}
	expect.GreaterOrEqual(t, buf.Cap(), allocSize(0))

	_, err := buf.WriteString("payload")
	expect.NoError(t, err)
	expect.Equal(t, buf.String(), "payload")
	testBytesPool.PutBuffer(buf)
}

func TestGetSizedBufferTooSmall(t *testing.T) {
	t.Cleanup(initAll)
	// Test when pool buffer is smaller than requested size
	smallSize := allocSize(0)
	largeSize := allocSize(1)

	// Put small buffer in pool
	b1 := testBytesPool.GetSized(smallSize)
	expect.Equal(t, len(b1), smallSize)
	expect.Equal(t, cap(b1), smallSize)
	testBytesPool.Put(b1)

	// Request larger size - should create new buffer, not reuse small one
	b2 := testBytesPool.GetSized(largeSize)
	expect.Equal(t, len(b2), largeSize)
	expect.Equal(t, cap(b2), largeSize)
	expect.NotEqual(t, underlyingPtr(b2), underlyingPtr(b1))

	// The small buffer should still be in pool
	b3 := testBytesPool.GetSized(smallSize)
	expect.Equal(t, underlyingPtr(b3), underlyingPtr(b1))
}

func TestPullDropsBufferTooSmall(t *testing.T) {
	pool := newTypedWeakPool(1)
	buf := make([]byte, allocSize(0))
	expect.True(t, pool.Put(makeWeak(buf)))

	expect.Nil(t, pull(pool, allocSize(1)))
	runtime.KeepAlive(buf)
}

func TestGetSizedLargeBuffer(t *testing.T) {
	t.Cleanup(initAll)
	largeSize := allocSize(SizedPools-1) * 2
	b := testBytesPool.GetSized(largeSize)
	expect.Equal(t, len(b), largeSize)
	expect.Equal(t, cap(b), largeSize)
	testBytesPool.Put(b)
}

func TestPoolIdx(t *testing.T) {
	for i := range SizedPools {
		size := allocSize(i)
		expectedIdx := i
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			idx := poolIdx(size)
			expect.Equal(t, idx, expectedIdx, "poolIdx(%d) should return %d", size, expectedIdx)
			expect.Equal(t, allocSize(idx), size, "Pool size %d should be %d", size, allocSize(idx))
		})
	}
	t.Run("verify_enough_pool_size", func(t *testing.T) {
		for i := range allocSize(SizedPools - 1) {
			idx := poolIdx(i)
			expect.GreaterOrEqual(t, allocSize(idx), i, "Pool size %d should be >= %d", allocSize(idx), i)
		}
	})
}
