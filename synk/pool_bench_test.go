package synk

import (
	"crypto/rand"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

var sink atomic.Value

func escape(b []byte) {
	sink.Store(b)
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
			b.Run("sync", func(b *testing.B) {
				b.Cleanup(initAll)
				bytesPoolSync := GetSizedBytesPoolSync()
				for b.Loop() {
					buf := bytesPoolSync.GetSized(size)
					escape(buf)
					bytesPoolSync.Put(buf)
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
	b.Run("sync", func(b *testing.B) {
		b.Cleanup(initAll)
		bytesPoolSync := GetSizedBytesPoolSync()
		i := 0
		for b.Loop() {
			buf := bytesPoolSync.GetSized(sizes[i%len(sizes)])
			escape(buf)
			bytesPoolSync.Put(buf)
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
	b.Run("sync", func(b *testing.B) {
		b.Cleanup(initAll)
		bytesPoolSync := GetSizedBytesPoolSync()
		i := 0
		for b.Loop() {
			buf := bytesPoolSync.GetSized(sizes[i%len(sizes)])
			escape(buf)
			bytesPoolSync.Put(buf)
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

// simulateWork performs realistic buffer operations that mimic HTTP body processing
// or data transformation work. Returns a checksum to prevent compiler optimization.
func simulateWork(buf []byte, workload int) uint64 {
	var checksum uint64
	length := len(buf)

	switch workload {
	case 0: // Light work: fill with pattern (like preparing response)
		for i := range buf {
			buf[i] = byte(i & 0xff)
			checksum += uint64(buf[i])
		}
	case 1: // Medium work: read + transform (like parsing/encoding)
		for i := 0; i < length; i += 8 {
			end := min(i+8, length)
			for j := i; j < end; j++ {
				buf[j] = byte((j * 31) & 0xff)
				checksum ^= uint64(buf[j]) << (uint(j) & 7)
			}
		}
	case 2: // Heavy work: multiple passes (like compression/encryption simulation)
		for pass := range 3 {
			for i := 0; i < length; i += 64 {
				end := min(i+64, length)
				for j := i; j < end; j++ {
					buf[j] = byte((buf[j] + byte(pass*17)) ^ byte(j))
					checksum += uint64(buf[j]) * uint64(pass+1)
				}
			}
		}
	}
	return checksum
}

// BenchmarkBytesPool_ConcurrentAllocations simulates real-world concurrent usage patterns
// where a fixed number of workers continuously get/use/put buffers with realistic operations.
func BenchmarkBytesPool_ConcurrentAllocations(b *testing.B) {
	// Generate size distribution biased toward smaller allocations (more realistic)
	// 50% small (4-32KB), 30% medium (32-256KB), 20% large (256KB-4MB)
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

	// Workload distribution: 60% light, 30% medium, 10% heavy
	workloads := make([]int, 100)
	for i := range workloads {
		r := psrng(100)
		switch {
		case r < 60:
			workloads[i] = 0 // light
		case r < 90:
			workloads[i] = 1 // medium
		default:
			workloads[i] = 2 // heavy
		}
	}

	concurrencyLevels := []int{1, 2, 4, 8, 16, 32}

	for _, concurrency := range concurrencyLevels {
		for _, poolType := range []string{"unsized", "sized", "sync", "make"} {
			b.Run(fmt.Sprintf("workers-%d-%s", concurrency, poolType), func(b *testing.B) {
				b.Cleanup(initAll)

				// Create work queue
				workChan := make(chan int, concurrency*2)
				var wg sync.WaitGroup

				// Track allocation and work time separately
				var totalAllocTime atomic.Int64
				var totalWorkTime atomic.Int64
				var checksumSink atomic.Uint64

				// Start fixed pool of workers (before timer)
				for range concurrency {
					wg.Go(func() {
						sizeIdx := 0
						workloadIdx := 0

						for range workChan {
							size := sizes[sizeIdx%len(sizes)]
							workload := workloads[workloadIdx%len(workloads)]
							sizeIdx++
							workloadIdx++

							// Measure allocation time
							allocStart := time.Now()

							var buf []byte
							switch poolType {
							case "unsized":
								buf = slices.Grow(unsizedBytesPool.Get(), size)[:size]
							case "sized":
								buf = sizedBytesPool.GetSized(size)
							case "sync":
								buf = sizedBytesPoolSync.GetSized(size)
							case "make":
								buf = make([]byte, size)
							}

							allocEnd := time.Now()
							totalAllocTime.Add(int64(allocEnd.Sub(allocStart)))

							// Simulate realistic buffer work (HTTP body processing, encoding, etc)
							workStart := time.Now()
							checksum := simulateWork(buf, workload)
							checksumSink.Add(checksum)
							workEnd := time.Now()
							totalWorkTime.Add(int64(workEnd.Sub(workStart)))

							escape(buf)

							// Return to pool
							switch poolType {
							case "unsized":
								unsizedBytesPool.Put(buf)
							case "sized":
								sizedBytesPool.Put(buf)
							case "sync":
								sizedBytesPoolSync.Put(buf)
							}
						}
					})
				}

				b.ResetTimer()

				// Feed work to workers
				for b.Loop() {
					workChan <- 1
				}

				close(workChan)
				wg.Wait()

				// Report detailed metrics
				b.StopTimer()
				avgAllocTime := time.Duration(totalAllocTime.Load() / int64(b.N))
				avgWorkTime := time.Duration(totalWorkTime.Load() / int64(b.N))
				avgTotalTime := avgAllocTime + avgWorkTime

				b.ReportMetric(float64(avgAllocTime.Nanoseconds()), "ns/op_alloc")
				b.ReportMetric(float64(avgWorkTime.Nanoseconds()), "ns/op_work")
				b.ReportMetric(float64(avgTotalTime.Nanoseconds()), "ns/op_total")

				// Prevent optimization
				if checksumSink.Load() == 0 {
					b.Fatal("checksum should not be zero")
				}
			})
		}
	}
}
