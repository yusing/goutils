package cache

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/puzpuzpuz/xsync/v4"
)

const maxEntries = 10000

func BenchmarkCacheFuncHitSerial(b *testing.B) {
	var fnCalls atomic.Int64
	cachedFunc := NewFunc(func(ctx context.Context) (int, error) {
		fnCalls.Add(1)
		return 42, nil
	}).Build()

	ctx := b.Context()
	_, _ = cachedFunc(ctx)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = cachedFunc(ctx)
	}
	b.StopTimer()

	b.ReportMetric(float64(fnCalls.Load())/float64(b.N+1), "fn/op")
}

func BenchmarkCacheFuncHitParallel(b *testing.B) {
	var fnCalls atomic.Int64
	cachedFunc := NewFunc(func(ctx context.Context) (int, error) {
		fnCalls.Add(1)
		return 42, nil
	}).Build()

	ctx := b.Context()
	_, _ = cachedFunc(ctx)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = cachedFunc(ctx)
		}
	})
	b.StopTimer()

	b.ReportMetric(float64(fnCalls.Load())/float64(b.N+1), "fn/op")
}

func BenchmarkCacheKeyUnboundedSameKeyParallel(b *testing.B) {
	benchmarkKeyedParallel(b, benchmarkKeyedConfig{
		name: "unbounded_same_key_parallel",
		builder: func(fn CachedContextKeyFunc[int, int]) CachedContextKeyFunc[int, int] {
			return NewKeyFunc(fn).Build()
		},
		keys:    []int{1},
		prewarm: []int{1},
	})
}

func BenchmarkCacheKeyBoundedHotSetParallel(b *testing.B) {
	keys := makeRoundRobinKeys(128)
	benchmarkKeyedParallel(b, benchmarkKeyedConfig{
		name: "bounded_hotset_fit_parallel",
		builder: func(fn CachedContextKeyFunc[int, int]) CachedContextKeyFunc[int, int] {
			return NewKeyFunc(fn).WithMaxEntries(maxEntries).Build()
		},
		keys:    keys,
		prewarm: keys,
	})
}

func BenchmarkCacheKeyBoundedHighChurnSerial(b *testing.B) {
	keys := makeRoundRobinKeys(4096)
	benchmarkKeyedSerial(b, benchmarkKeyedConfig{
		name: "bounded_high_churn_serial",
		builder: func(fn CachedContextKeyFunc[int, int]) CachedContextKeyFunc[int, int] {
			return NewKeyFunc(fn).WithMaxEntries(maxEntries).Build()
		},
		keys: keys,
	})
}

func BenchmarkCacheKeyBoundedSkewedParallel(b *testing.B) {
	keys := makeSkewedKeys(1<<16, 64, 4096, 95)
	benchmarkKeyedParallel(b, benchmarkKeyedConfig{
		name: "bounded_skewed_parallel",
		builder: func(fn CachedContextKeyFunc[int, int]) CachedContextKeyFunc[int, int] {
			return NewKeyFunc(fn).WithMaxEntries(maxEntries).Build()
		},
		keys:    keys,
		prewarm: makeRoundRobinKeys(64),
	})
}

func BenchmarkCacheKeyCleanupOverflow(b *testing.B) {
	for _, overflow := range []int{32, 256, 1024} {
		b.Run(fmt.Sprintf("overflow_%d", overflow), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				state := newBenchmarkCleanupState(maxEntries, overflow)
				b.StartTimer()
				state.Cleanup()
			}
		})
	}
}

type benchmarkKeyedConfig struct {
	name    string
	builder func(fn CachedContextKeyFunc[int, int]) CachedContextKeyFunc[int, int]
	keys    []int
	prewarm []int
}

func benchmarkKeyedSerial(b *testing.B, cfg benchmarkKeyedConfig) {
	var fnCalls atomic.Int64
	cachedFunc := cfg.builder(func(ctx context.Context, key int) (int, error) {
		fnCalls.Add(1)
		return key * 2, nil
	})

	ctx := b.Context()
	for _, key := range cfg.prewarm {
		_, _ = cachedFunc(ctx, key)
	}

	var idx uint64
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		key := cfg.keys[idx%uint64(len(cfg.keys))]
		idx++
		_, _ = cachedFunc(ctx, key)
	}
	b.StopTimer()

	b.ReportMetric(float64(fnCalls.Load())/float64(b.N+len(cfg.prewarm)), "fn/op")
}

func benchmarkKeyedParallel(b *testing.B, cfg benchmarkKeyedConfig) {
	var fnCalls atomic.Int64
	cachedFunc := cfg.builder(func(ctx context.Context, key int) (int, error) {
		fnCalls.Add(1)
		return key * 2, nil
	})

	ctx := b.Context()
	for _, key := range cfg.prewarm {
		_, _ = cachedFunc(ctx, key)
	}

	var idx atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := cfg.keys[(idx.Add(1)-1)%uint64(len(cfg.keys))]
			_, _ = cachedFunc(ctx, key)
		}
	})
	b.StopTimer()

	b.ReportMetric(float64(fnCalls.Load())/float64(b.N+len(cfg.prewarm)), "fn/op")
}

func newBenchmarkCleanupState(maxEntries, overflow int) *CachedContextKeyFuncState[int, int] {
	state := &CachedContextKeyFuncState[int, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[int, int]{
			maxEntries: maxEntries,
		},
		entries:    xsync.NewMap[int, *CacheEntry[int]](),
		accessLog:  make([]cleanupCandidate[int], 0, initialAccessLogCap(maxEntries)),
		cleanupLog: make([]cleanupCandidate[int], 0, initialAccessLogCap(maxEntries)),
	}

	totalEntries := maxEntries + overflow
	for key := range totalEntries {
		entry := &CacheEntry[int]{}
		entry.cached.Store(&cachedValue[int]{result: key})
		state.entries.Store(key, entry)
		state.touchEntry(key, entry)
	}

	return state
}

func makeRoundRobinKeys(keyCount int) []int {
	keys := make([]int, keyCount)
	for i := range keyCount {
		keys[i] = i
	}
	return keys
}

func makeSkewedKeys(total, hotSet, coldSet, hotPercent int) []int {
	keys := make([]int, total)
	for i := range total {
		if i%100 < hotPercent {
			keys[i] = i % hotSet
			continue
		}
		keys[i] = hotSet + (i % coldSet)
	}
	return keys
}
