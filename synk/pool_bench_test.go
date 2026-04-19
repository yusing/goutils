package synk

import (
	"context"
	"crypto/rand"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/yusing/goutils/synk/workerpool"
)

var sink atomic.Value

func escape(b []byte) {
	sink.Store(b)
}

func deterministicJitter(i int, maxDelay time.Duration) {
	if maxDelay <= 0 {
		return
	}
	d := time.Duration((uint64(i) * 0x9e3779b97f4a7c15) % (uint64(maxDelay) + 1))
	time.Sleep(d)
}

var cpuWorkSink atomic.Uint64

//go:noinline
func cpuWork(i, maxIter int) {
	n := (uint64(i) * 0x9e3779b97f4a7c15) % uint64(maxIter+1)
	x := uint64(i) | 1
	for range n {
		x = x*6364136223846793005 + 1442695040888963407
	}
	cpuWorkSink.Store(x)
}

func BenchmarkBytesPool_Get(b *testing.B) {
	sizes := make([]int, 0, SizedPools)
	for i := range SizedPools {
		sizes = append(sizes, allocSize(i))
	}

	maxDelay := 50 * time.Microsecond

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			b.Run("unsized", func(b *testing.B) {
				b.Cleanup(initAll)
				pool := workerpool.New(b.Context())
				for b.Loop() {
					pool.Go(func(_ context.Context, i int) {
						buf := slices.Grow(unsizedBytesPool.Get(), size)
						deterministicJitter(i, maxDelay)
						escape(buf)
						unsizedBytesPool.Put(buf)
					})
				}
				pool.Wait()
			})
			b.Run("sized", func(b *testing.B) {
				b.Cleanup(initAll)
				bytesPoolWithMemory := GetSizedBytesPool()
				pool := workerpool.New(b.Context())
				for b.Loop() {
					pool.Go(func(_ context.Context, i int) {
						buf := bytesPoolWithMemory.GetSized(size)
						deterministicJitter(i, maxDelay)
						escape(buf)
						bytesPoolWithMemory.Put(buf)
					})
				}
				pool.Wait()
			})
			b.Run("make", func(b *testing.B) {
				pool := workerpool.New(b.Context())
				for b.Loop() {
					pool.Go(func(_ context.Context, i int) {
						buf := make([]byte, size)
						deterministicJitter(i, maxDelay)
						escape(buf)
					})
				}
				pool.Wait()
			})
		})
	}
}

// avoid uniform distribution to simulate real-world usage
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

	maxDelay := 50 * time.Microsecond

	b.Run("unsized", func(b *testing.B) {
		b.Cleanup(initAll)
		pool := workerpool.New(b.Context())
		for b.Loop() {
			pool.Go(func(_ context.Context, i int) {
				buf := slices.Grow(unsizedBytesPool.Get(), sizes[i%len(sizes)])
				deterministicJitter(i, maxDelay)
				escape(buf)
				unsizedBytesPool.Put(buf)
			})
		}
		pool.Wait()
	})
	b.Run("sized", func(b *testing.B) {
		b.Cleanup(initAll)
		bytesPoolWithMemory := GetSizedBytesPool()
		pool := workerpool.New(b.Context())
		for b.Loop() {
			pool.Go(func(_ context.Context, i int) {
				buf := bytesPoolWithMemory.GetSized(sizes[i%len(sizes)])
				deterministicJitter(i, maxDelay)
				escape(buf)
				bytesPoolWithMemory.Put(buf)
			})
		}
		pool.Wait()
	})
	b.Run("make", func(b *testing.B) {
		pool := workerpool.New(b.Context())
		for b.Loop() {
			pool.Go(func(_ context.Context, i int) {
				buf := make([]byte, sizes[i%len(sizes)])
				deterministicJitter(i, maxDelay)
				escape(buf)
			})
		}
		pool.Wait()
	})

	printPoolStats()
}

func BenchmarkBytesPool_GetAllExceedsMax(b *testing.B) {
	sizes := make([]int, 1000)
	for i := range sizes {
		sizes[i] = 1 + psrng(sizedBytesPool.max*2)
	}

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

// BenchmarkBytesPool_ConcurrentAllocations simulates real-world concurrent usage patterns
// where a fixed number of workers continuously get/use/put buffers with realistic operations.
func BenchmarkBytesPool_ConcurrentAllocations(b *testing.B) {
	// Generate size distribution biased toward smaller allocations (more realistic)
	// 50% small (2-4KB), 30% medium (32KB-544KB), 20% large (258KB-12MB)
	sizes := make([]int, 1000)
	for i := range sizes {
		r := psrng(100)
		switch {
		case r < 50: // 50% - small allocations
			sizes[i] = 2*kb + psrng(2*kb)
		case r < 80: // 30% - medium allocations
			sizes[i] = 32*kb + allocSize(psrng(SizedPools-2))
		default: // 20% - large allocations
			sizes[i] = 256*kb + ((1 + psrng(3)) * allocSize(psrng(SizedPools-1)))
		}
	}

	concurrencyLevels := []int{1, 2, 4, 8, 16, 32}
	const workIterations = 500 // CPU-bound iterations to simulate buffer usage

	for _, concurrency := range concurrencyLevels {
		for _, poolType := range []string{"unsized", "sized", "make"} {
			b.Run(fmt.Sprintf("workers-%d-%s", concurrency, poolType), func(b *testing.B) {
				b.Cleanup(initAll)
				b.ReportAllocs()

				pool := workerpool.New(b.Context(), workerpool.WithN(concurrency))
				for b.Loop() {
					pool.Go(func(_ context.Context, i int) {
						size := sizes[i%len(sizes)]

						var buf []byte
						switch poolType {
						case "unsized":
							buf = unsizedBytesPool.GetAtLeast(size)[:size]
						case "sized":
							buf = sizedBytesPool.GetSized(size)
						case "make":
							buf = make([]byte, size)
						}

						cpuWork(i, workIterations)
						escape(buf)

						switch poolType {
						case "unsized":
							unsizedBytesPool.Put(buf)
						case "sized":
							sizedBytesPool.Put(buf)
						}
					})
				}
				pool.Wait()
			})
		}
	}
}
