package synk

import (
	"context"
	"crypto/rand"
	"fmt"
	"runtime"
	"slices"
	"sync"
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

type benchmarkSizedPoolArchitecture struct {
	name string
	get  func(*SizedBytesPool, int) []byte
}

type benchmarkSyncSizedPool struct {
	pools [SizedPools]sync.Pool
}

type benchmarkChannelSizedPool struct {
	pools [SizedPools]chan weakBuf
}

func newBenchmarkChannelSizedPool() *benchmarkChannelSizedPool {
	p := new(benchmarkChannelSizedPool)
	for i := range p.pools {
		p.pools[i] = make(chan weakBuf, poolSharedLimit(i))
	}
	return p
}

func (p *benchmarkChannelSizedPool) GetSized(size int) []byte {
	if size > allocSize(SizedPools-1) {
		return make([]byte, size)
	}

	targetIdx := poolIdx(size)
	for idx := targetIdx; idx < SizedPools; idx++ {
		if buf := benchmarkChannelPull(p.pools[idx], size); buf != nil {
			capB := cap(buf)
			buf = buf[:capB]
			if size > 0 && capB-size >= allocSize(0) {
				p.Put(buf[size:])
				return buf[:size:size]
			}
			return buf[:size]
		}
	}
	return make([]byte, size, allocSize(targetIdx))
}

func (p *benchmarkChannelSizedPool) Put(buf []byte) {
	capB := cap(buf)
	if capB < allocSize(0) || capB > allocSize(SizedPools-1) {
		return
	}

	idx := poolIdx(capB)
	if capB < allocSize(idx) {
		idx--
	}
	benchmarkChannelPut(buf, p.pools[idx])
}

func (p *benchmarkSyncSizedPool) GetSized(size int) []byte {
	if size > allocSize(SizedPools-1) {
		return make([]byte, size)
	}

	targetIdx := poolIdx(size)
	for idx := targetIdx; idx < SizedPools; idx++ {
		for {
			value := p.pools[idx].Get()
			if value == nil {
				break
			}

			buf := value.([]byte)
			capB := cap(buf)
			if capB < size {
				continue
			}

			buf = buf[:capB]
			if size > 0 && capB-size >= allocSize(0) {
				p.Put(buf[size:])
				return buf[:size:size]
			}
			return buf[:size]
		}
	}

	return make([]byte, size, allocSize(targetIdx))
}

func (p *benchmarkSyncSizedPool) Put(buf []byte) {
	capB := cap(buf)
	if capB < allocSize(0) || capB > allocSize(SizedPools-1) {
		return
	}

	idx := poolIdx(capB)
	if capB < allocSize(idx) {
		idx--
	}
	p.pools[idx].Put(buf)
}

type benchmarkSizedPoolBackend struct {
	name string
	new  func() (get func(int) []byte, put func([]byte))
}

func runSizedPoolBackendBenchmark(
	b *testing.B,
	backend benchmarkSizedPoolBackend,
	sizes []int,
	parallel bool,
	burst int,
) {
	b.Helper()
	get, put := backend.new()
	b.ReportAllocs()

	if burst == 1 {
		if parallel {
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					buf := get(sizes[i%len(sizes)])
					buf[0]++
					put(buf)
					runtime.KeepAlive(buf)
					i++
				}
			})
			return
		}

		i := 0
		for b.Loop() {
			buf := get(sizes[i%len(sizes)])
			buf[0]++
			put(buf)
			runtime.KeepAlive(buf)
			i++
		}
		return
	}

	if parallel {
		b.RunParallel(func(pb *testing.PB) {
			bufs := make([][]byte, burst)
			i := 0
			for pb.Next() {
				for j := range bufs {
					bufs[j] = get(sizes[(i+j)%len(sizes)])
					bufs[j][0]++
				}
				for _, buf := range bufs {
					put(buf)
				}
				runtime.KeepAlive(bufs)
				i += burst
			}
		})
		return
	}

	bufs := make([][]byte, burst)
	i := 0
	for b.Loop() {
		for j := range bufs {
			bufs[j] = get(sizes[(i+j)%len(sizes)])
			bufs[j][0]++
		}
		for _, buf := range bufs {
			put(buf)
		}
		runtime.KeepAlive(bufs)
		i += burst
	}
}

func benchmarkSizedPoolBackends() []benchmarkSizedPoolBackend {
	return []benchmarkSizedPoolBackend{
		{
			name: "weak-channel",
			new: func() (func(int) []byte, func([]byte)) {
				pool := newBenchmarkChannelSizedPool()
				return pool.GetSized, pool.Put
			},
		},
		{
			name: "sync-pool",
			new: func() (func(int) []byte, func([]byte)) {
				pool := new(benchmarkSyncSizedPool)
				return pool.GetSized, pool.Put
			},
		},
		{
			name: "typed-per-p",
			new: func() (func(int) []byte, func([]byte)) {
				pool := newBenchmarkSizedPool()
				return pool.GetSized, pool.Put
			},
		},
	}
}

func newBenchmarkSizedPool() *SizedBytesPool {
	p := new(SizedBytesPool)
	for i := range p.pools {
		p.pools[i] = newTypedWeakPool(poolSharedLimit(i))
	}
	return p
}

func benchmarkChannelPull(pool <-chan weakBuf, size int) []byte {
	for {
		select {
		case bWeak := <-pool:
			buf := getBufFromWeak(bWeak)
			if buf == nil || cap(buf) < size {
				continue
			}
			return buf
		default:
			return nil
		}
	}
}

func benchmarkChannelPut(buf []byte, pool chan<- weakBuf) {
	select {
	case pool <- makeWeak(buf):
	default:
	}
}

func benchmarkPullBounded(pool <-chan weakBuf, size, maxAttempts int) []byte {
	for range maxAttempts {
		select {
		case bWeak := <-pool:
			buf := getBufFromWeak(bWeak)
			if buf == nil || cap(buf) < size {
				continue
			}
			return buf
		default:
			return nil
		}
	}
	return nil
}

func benchmarkDeadWeakBufs(b *testing.B, count, size int) []weakBuf {
	b.Helper()

	dead := make([]weakBuf, count)
	for i := range dead {
		buf := make([]byte, size)
		dead[i] = makeWeak(buf)
	}
	runtime.GC()

	for i, ref := range dead {
		if ref.ptr.Value() != nil {
			b.Fatalf("weak buffer %d remains live after GC", i)
		}
	}
	return dead
}

func benchmarkGetExact(p *SizedBytesPool, size int) []byte {
	if size > allocSize(SizedPools-1) {
		return make([]byte, size)
	}

	idx := poolIdx(size)
	if buf := pull(p.pools[idx], size); buf != nil {
		return buf[:size]
	}
	return make([]byte, size, allocSize(idx))
}

func benchmarkGetWholeFallback(p *SizedBytesPool, size int) []byte {
	if size > allocSize(SizedPools-1) {
		return make([]byte, size)
	}

	targetIdx := poolIdx(size)
	for idx := targetIdx; idx < SizedPools; idx++ {
		if buf := pull(p.pools[idx], size); buf != nil {
			return buf[:size]
		}
	}
	return make([]byte, size, allocSize(targetIdx))
}

func benchmarkGetSplitLimit8x(p *SizedBytesPool, size int) []byte {
	if size > allocSize(SizedPools-1) {
		return make([]byte, size)
	}

	targetIdx := poolIdx(size)
	maxCapacity := max(size, allocSize(0)) * 8
	maxIdx := poolIdx(maxCapacity)
	if allocSize(maxIdx) > maxCapacity {
		maxIdx--
	}
	for idx := targetIdx; idx <= maxIdx; idx++ {
		buf := pull(p.pools[idx], size)
		if buf == nil {
			continue
		}

		capB := cap(buf)
		buf = buf[:capB]
		if size > 0 && capB-size >= allocSize(0) {
			p.put(buf[size:])
			return buf[:size:size]
		}
		return buf[:size]
	}
	return make([]byte, size, allocSize(targetIdx))
}

func benchmarkPutSplitOversize(p *SizedBytesPool, buf []byte) {
	if cap(buf) <= allocSize(SizedPools-1) {
		p.put(buf)
		return
	}

	buf = buf[:cap(buf)]
	mid := cap(buf) / 2
	benchmarkPutSplitOversize(p, buf[:mid:mid])
	benchmarkPutSplitOversize(p, buf[mid:])
}

func drainBenchmarkPool(p *SizedBytesPool) {
	for _, pool := range p.pools {
		for {
			if _, ok := pool.Get(); !ok {
				break
			}
		}
	}
}

func BenchmarkSizedPoolPatterns(b *testing.B) {
	patterns := []struct {
		name  string
		sizes []int
	}{
		{name: "sub-tier", sizes: []int{allocSize(0) / 2}},
		{name: "exact-tier", sizes: []int{allocSize(3)}},
		{name: "mixed-common", sizes: []int{1024, 2048, 4096, 32 * kb, 256 * kb}},
	}

	for _, pattern := range patterns {
		b.Run(pattern.name, func(b *testing.B) {
			pool := newBenchmarkSizedPool()
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				buf := pool.GetSized(pattern.sizes[i%len(pattern.sizes)])
				buf[0] = byte(i)
				pool.Put(buf)
				runtime.KeepAlive(buf)
				i++
			}
		})
	}

	b.Run("parallel-32KiB", func(b *testing.B) {
		pool := newBenchmarkSizedPool()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				buf := pool.GetSized(32 * kb)
				buf[0]++
				pool.Put(buf)
				runtime.KeepAlive(buf)
			}
		})
	})

	b.Run("oversize-return", func(b *testing.B) {
		pool := newBenchmarkSizedPool()
		buf := make([]byte, allocSize(SizedPools-1)*32)
		b.ReportAllocs()
		for b.Loop() {
			pool.Put(buf)
			runtime.KeepAlive(buf)
		}
	})
}

// BenchmarkSizedPoolBackends compares typed per-P storage, channel storage,
// and sync.Pool under the same tier lookup and splitting policy. Run with
// -cpu=1,2,4,8 to expose single-tier channel contention as parallelism grows.
func BenchmarkSizedPoolBackends(b *testing.B) {
	patterns := []struct {
		name     string
		sizes    []int
		parallel bool
	}{
		{name: "serial-hot-tier", sizes: []int{32 * kb}},
		{name: "serial-mixed-tiers", sizes: []int{2 * kb, 4 * kb, 32 * kb, 256 * kb}},
		{name: "parallel-hot-tier", sizes: []int{32 * kb}, parallel: true},
		{name: "parallel-mixed-tiers", sizes: []int{2 * kb, 4 * kb, 32 * kb, 256 * kb}, parallel: true},
	}

	for _, pattern := range patterns {
		b.Run(pattern.name, func(b *testing.B) {
			for _, backend := range benchmarkSizedPoolBackends() {
				b.Run(backend.name, func(b *testing.B) {
					runSizedPoolBackendBenchmark(b, backend, pattern.sizes, pattern.parallel, 1)
				})
			}
		})
	}
}

func BenchmarkSizedPoolBackendBursts(b *testing.B) {
	patterns := []struct {
		name     string
		parallel bool
		burst    int
	}{
		{name: "serial-16", burst: 16},
		{name: "parallel-8", parallel: true, burst: 8},
	}

	for _, pattern := range patterns {
		b.Run(pattern.name, func(b *testing.B) {
			for _, backend := range benchmarkSizedPoolBackends() {
				b.Run(backend.name, func(b *testing.B) {
					runSizedPoolBackendBenchmark(b, backend, []int{32 * kb}, pattern.parallel, pattern.burst)
				})
			}
		})
	}
}

func BenchmarkSizedPoolDeadRecovery(b *testing.B) {
	const maxAttempts = 8

	cases := []struct {
		name, size string
		deadCount  int
		bufferSize int
	}{
		{name: "sized", size: "256-dead", deadCount: poolSharedLimit(0), bufferSize: allocSize(0)},
		{name: "unsized", size: "4096-dead", deadCount: UnsizedPoolSize, bufferSize: MinAllocSize},
	}

	for _, tc := range cases {
		b.Run(tc.name+"/"+tc.size, func(b *testing.B) {
			dead := benchmarkDeadWeakBufs(b, tc.deadCount, tc.bufferSize)
			replacements := make([][]byte, (tc.deadCount+maxAttempts-1)/maxAttempts)
			for i := range replacements {
				replacements[i] = make([]byte, tc.bufferSize)
			}

			for _, bounded := range []bool{false, true} {
				name := "drain-all"
				if bounded {
					name = "limit-8"
				}
				b.Run(name, func(b *testing.B) {
					var totalMisses int64
					b.ReportAllocs()

					for b.Loop() {
						pool := make(chan weakBuf, tc.deadCount)
						for _, ref := range dead {
							pool <- ref
						}

						replacement := 0
						for range tc.deadCount + 1 {
							var buf []byte
							if bounded {
								buf = benchmarkPullBounded(pool, tc.bufferSize, maxAttempts)
							} else {
								buf = benchmarkChannelPull(pool, tc.bufferSize)
							}
							if buf == nil {
								buf = replacements[replacement]
								replacement++
								totalMisses++
							}
							benchmarkChannelPut(buf, pool)
						}
					}

					b.ReportMetric(float64(totalMisses)/float64(b.N), "misses/episode")
					runtime.KeepAlive(replacements)
				})
			}
		})
	}
}

func BenchmarkSizedPoolArchitectures(b *testing.B) {
	architectures := []benchmarkSizedPoolArchitecture{
		{name: "exact-tier", get: benchmarkGetExact},
		{name: "whole-fallback", get: benchmarkGetWholeFallback},
		{name: "split-fallback", get: (*SizedBytesPool).GetSized},
		{name: "split-max-8x", get: benchmarkGetSplitLimit8x},
	}

	burstPatterns := []struct {
		name                    string
		seedSize, requestSize   int
		seedCount, requestCount int
	}{
		{name: "half-tier", seedSize: 4 * kb, requestSize: 2 * kb, seedCount: 8, requestCount: 16},
		{name: "uneven-tier", seedSize: 8 * kb, requestSize: 3 * kb, seedCount: 8, requestCount: 16},
		{name: "wide-tier", seedSize: 256 * kb, requestSize: 32 * kb, seedCount: 8, requestCount: 64},
		{name: "extreme-single", seedSize: 2 * mb, requestSize: 2 * kb, seedCount: 1, requestCount: 1},
		{name: "extreme-burst", seedSize: 2 * mb, requestSize: 2 * kb, seedCount: 1, requestCount: 32},
	}
	for _, pattern := range burstPatterns {
		b.Run("cold-burst/"+pattern.name, func(b *testing.B) {
			for _, architecture := range architectures {
				b.Run(architecture.name, func(b *testing.B) {
					pool := newBenchmarkSizedPool()
					seed := make([][]byte, pattern.seedCount)
					seedRefs := make([]weakBuf, pattern.seedCount)
					for i := range seed {
						seed[i] = make([]byte, pattern.seedSize)
						seedRefs[i] = makeWeak(seed[i])
					}
					borrowed := make([][]byte, pattern.requestCount)
					b.ReportAllocs()
					for b.Loop() {
						clear(borrowed)
						drainBenchmarkPool(pool)
						for _, ref := range seedRefs {
							if !pool.pools[poolIdx(ref.cap)].Put(ref) {
								b.Fatal("seed does not fit in benchmark pool")
							}
						}
						for i := range borrowed {
							borrowed[i] = architecture.get(pool, pattern.requestSize)
						}
						runtime.KeepAlive(borrowed)
					}
					runtime.KeepAlive(seed)
				})
			}
		})
	}

	b.Run("oversize-return", func(b *testing.B) {
		for _, split := range []bool{false, true} {
			name := "drop"
			if split {
				name = "split"
			}
			b.Run(name, func(b *testing.B) {
				pool := newBenchmarkSizedPool()
				buf := make([]byte, allocSize(SizedPools-1)*32)
				b.ReportAllocs()
				for b.Loop() {
					if split {
						benchmarkPutSplitOversize(pool, buf)
					} else {
						pool.Put(buf)
					}
					runtime.KeepAlive(buf)
				}
			})
		}
	})
}

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
		sizes[i] = 1 + psrng(allocSize(SizedPools-1)-1)
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
		sizes[i] = 1 + psrng(allocSize(SizedPools-1)*2)
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
