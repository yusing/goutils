package synk

import (
	"crypto/rand"
	"fmt"
	"slices"
	"testing"
	"unsafe"
)

var sink []byte

func escape(b []byte) {
	sink = b
}

func BenchmarkBytesPool_Get(b *testing.B) {
	var sizes = make([]int, 0, SizedPools)
	for i := range SizedPools {
		sizes = append(sizes, allocSize(i))
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("unsized", func(b *testing.B) {
				b.Cleanup(initAll)
				for b.Loop() {
					buf := slices.Grow(unsizedBytesPool.Get(), size)
					escape(buf)
					unsizedBytesPool.Put(buf)
				}
			})
			b.Run("sized", func(b *testing.B) {
				b.Cleanup(initAll)
				bytesPoolWithMemory := GetSizedBytesPool()
				for b.Loop() {
					buf := bytesPoolWithMemory.GetSized(size)
					escape(buf)
					bytesPoolWithMemory.Put(buf)
				}
			})
			b.Run("make", func(b *testing.B) {
				for b.Loop() {
					buf := make([]byte, size)
					escape(buf)
				}
			})
		})
	}
}

func psrng(n int) int {
	var b [32]byte
	rand.Read(b[:])
	return int(*(*uint32)(unsafe.Pointer(&b)) % uint32(n))
}

func BenchmarkBytesPool_GetAll(b *testing.B) {
	sizes := make([]int, 1000)
	for i := range sizes {
		sizes[i] = 1 + psrng(sizedBytesPool.max-1)
	}

	b.Logf("sizes: %v", sizes)

	b.Run("unsized", func(b *testing.B) {
		b.Cleanup(initAll)
		i := 0
		for b.Loop() {
			buf := slices.Grow(unsizedBytesPool.Get(), sizes[i%len(sizes)])
			escape(buf)
			unsizedBytesPool.Put(buf)
			i++
		}
	})
	b.Run("sized", func(b *testing.B) {
		b.Cleanup(initAll)
		bytesPoolWithMemory := GetSizedBytesPool()
		i := 0
		for b.Loop() {
			buf := bytesPoolWithMemory.GetSized(sizes[i%len(sizes)])
			escape(buf)
			bytesPoolWithMemory.Put(buf)
			i++
		}
	})
	b.Run("make", func(b *testing.B) {
		i := 0
		for b.Loop() {
			buf := make([]byte, sizes[i%len(sizes)])
			escape(buf)
			i++
		}
	})

	printPoolStats()
}

func BenchmarkBytesPool_GetAllExceedsMax(b *testing.B) {
	sizes := make([]int, 1000)
	for i := range sizes {
		sizes[i] = 1 + psrng(sizedBytesPool.max*2)
	}

	b.Logf("sizes: %v", sizes)

	b.Run("unsized", func(b *testing.B) {
		b.Cleanup(initAll)
		i := 0
		for b.Loop() {
			buf := slices.Grow(unsizedBytesPool.Get(), sizes[i%len(sizes)])
			escape(buf)
			unsizedBytesPool.Put(buf)
			i++
		}
	})
	b.Run("sized", func(b *testing.B) {
		b.Cleanup(initAll)
		bytesPoolWithMemory := GetSizedBytesPool()
		i := 0
		for b.Loop() {
			buf := bytesPoolWithMemory.GetSized(sizes[i%len(sizes)])
			escape(buf)
			bytesPoolWithMemory.Put(buf)
			i++
		}
	})
	b.Run("make", func(b *testing.B) {
		i := 0
		for b.Loop() {
			buf := make([]byte, sizes[i%len(sizes)])
			escape(buf)
			i++
		}
	})

	printPoolStats()
}
