package synk

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testBytesPool = GetSizedBytesPool()

func underlyingPtr(b []byte) uintptr {
	return reflect.ValueOf(b).Pointer()
}

func fill(b []byte) {
	for i := range len(b) {
		b[i] = byte(i % 256)
	}
}

func verify(t *testing.T, b []byte) {
	for i := range len(b) {
		require.Equal(t, byte(i%256), b[i], "Data should be preserved after split at index %d", i)
	}
}

func TestSized(t *testing.T) {
	t.Cleanup(initAll)
	size := allocSize(1)
	b := testBytesPool.GetSized(size)
	assert.Equal(t, cap(b), size)
	testBytesPool.Put(b)
	assert.Equal(t, underlyingPtr(b), underlyingPtr(testBytesPool.GetSized(size)))
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

func TestGetSizedBufferSplit(t *testing.T) {
	t.Cleanup(initAll)
	// Test buffer splitting when capacity > requested size
	largeSize := allocSize(4)
	requestedSize := largeSize - allocSize(1)

	// Create a large buffer and put it in pool
	b1 := testBytesPool.GetSized(largeSize)
	assert.Equal(t, largeSize, len(b1))
	assert.Equal(t, largeSize, cap(b1))

	testBytesPool.Put(b1)

	// Request smaller size - should split the buffer
	b2 := testBytesPool.GetSized(requestedSize)
	assert.Equal(t, requestedSize, len(b2))
	assert.Equal(t, requestedSize, cap(b2)) // capacity should remain the original
	assert.Equal(t, underlyingPtr(b1), underlyingPtr(b2))

	// The remaining part should be put back in pool
	// Request the remaining size to verify
	remainingSize := largeSize - requestedSize
	b3 := testBytesPool.GetSized(remainingSize)
	assert.Equal(t, remainingSize, len(b3))
	assert.Equal(t, remainingSize, cap(b3))

	// Verify the remaining buffer points to the correct memory location
	originalPtr := underlyingPtr(b1)
	remainingPtr := underlyingPtr(b3)

	// The remaining buffer should start at original + requestedSize
	expectedOffset := uintptr(originalPtr) + uintptr(requestedSize)
	actualOffset := uintptr(remainingPtr)
	assert.Equal(t, expectedOffset, actualOffset, "Remaining buffer should point to correct offset")
}

func TestGetSizedSmallRemainder(t *testing.T) {
	t.Cleanup(initAll)
	poolSize := allocSize(1)
	requestedSize := allocSize(0)
	remainderSize := poolSize - requestedSize

	// Create buffer and put in pool
	b1 := testBytesPool.GetSized(poolSize)
	testBytesPool.Put(b1)

	// Request size that leaves small remainder
	b2 := testBytesPool.GetSized(requestedSize)
	assert.Equal(t, requestedSize, len(b2))
	assert.GreaterOrEqual(t, cap(b2), requestedSize)

	// The small remainder (100 bytes) should NOT be put back in sized pool
	// Try to get the remainder size - should create new buffer
	b3 := testBytesPool.GetSized(remainderSize)
	assert.Equal(t, remainderSize, len(b3))
	assert.GreaterOrEqual(t, cap(b3), remainderSize)
	assert.NotEqual(t, underlyingPtr(b2), underlyingPtr(b3))
}

func TestGetSizedSmallBufferBypass(t *testing.T) {
	t.Cleanup(initAll)
	// Test that small buffers (< p.min) don't use sized pool
	smallSize := allocSize(0) - 1

	b1 := testBytesPool.GetSized(smallSize)
	assert.Equal(t, smallSize, len(b1))

	b2 := testBytesPool.GetSized(smallSize)
	assert.Equal(t, smallSize, len(b2))

	// Should be different buffers (not pooled)
	assert.NotEqual(t, underlyingPtr(b1), underlyingPtr(b2))
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

func TestGetSizedLargeBuffer(t *testing.T) {
	t.Cleanup(initAll)
	largeSize := allocSize(SizedPools-1) * 2
	b := testBytesPool.GetSized(largeSize)
	assert.Equal(t, largeSize, len(b))
	assert.Equal(t, largeSize, cap(b))
	testBytesPool.Put(b)
}

func TestGetSizedMultipleSplits(t *testing.T) {
	t.Cleanup(initAll)

	firstSize := allocSize(SizedPools - 3)
	secondSize := allocSize(SizedPools - 4)
	thirdSize := allocSize(SizedPools - 5)
	totalSize := firstSize + secondSize + thirdSize

	b := testBytesPool.GetSized(totalSize)
	ptr := underlyingPtr(b)
	testBytesPool.Put(b)

	part1 := testBytesPool.GetSized(firstSize)
	assert.Equal(t, firstSize, len(part1))
	assert.Equal(t, firstSize, cap(part1))
	assert.Equal(t, ptr, underlyingPtr(part1))

	part2 := testBytesPool.GetSized(secondSize)
	assert.Equal(t, secondSize, len(part2))
	assert.Equal(t, secondSize, cap(part2))
	assert.Equal(t, ptr+uintptr(firstSize), underlyingPtr(part2))

	part3 := testBytesPool.GetSized(thirdSize)
	assert.Equal(t, thirdSize, len(part3))
	assert.Equal(t, thirdSize, cap(part3))
	assert.Equal(t, ptr+uintptr(firstSize+secondSize), underlyingPtr(part3))

	testBytesPool.Put(part1)
	testBytesPool.Put(part2)
	testBytesPool.Put(part3)
}

func TestGetSizedMemorySafety(t *testing.T) {
	t.Cleanup(initAll)
	// Test that split buffers don't interfere with each other
	remainingSize := allocSize(2)
	totalSize := allocSize(4)
	firstSize := totalSize - remainingSize
	require.Equal(t, poolIdx(totalSize), poolIdx(firstSize))

	// Create buffer and split it
	b1 := testBytesPool.GetSized(totalSize)
	fill(b1)
	testBytesPool.Put(b1)

	// Get first part
	first := testBytesPool.GetSized(firstSize)
	assert.Equal(t, firstSize, len(first))
	assert.Equal(t, firstSize, cap(first))

	require.Equal(t, underlyingPtr(b1), underlyingPtr(first))

	// Verify data integrity
	verify(t, first)

	// Get remaining part
	remaining := testBytesPool.GetSized(remainingSize)
	assert.Equal(t, remainingSize, len(remaining))
	assert.Equal(t, remainingSize, cap(remaining))

	// remaining should be first + remainingSize
	require.Equal(t, underlyingPtr(first)+uintptr(firstSize), underlyingPtr(remaining))

	verify(t, first)
	verify(t, remaining)
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
		for i := range testBytesPool.max {
			idx := poolIdx(i)
			assert.GreaterOrEqual(t, allocSize(idx), i, "Pool size %d should be >= %d", allocSize(idx), i)
		}
	})
}
