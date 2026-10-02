package synk

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

func TestTypedWeakPoolQueue(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })

	const sharedLimit = 32
	entries := sharedLimit
	if !typedPoolRaceEnabled {
		entries++ // One private slot per P is outside the shared limit.
	}
	pool := newTypedWeakPool(sharedLimit)
	for i := 1; i <= entries; i++ {
		expect.True(t, pool.Put(weakBuf{cap: i}))
	}
	expect.False(t, pool.Put(weakBuf{cap: entries + 1}))

	seen := make(map[int]bool, entries)
	for range entries {
		value, ok := pool.Get()
		expect.True(t, ok, "typed pool lost an entry")
		expect.False(t, seen[value.cap], "typed pool returned capacity %d twice", value.cap)
		seen[value.cap] = true
	}
	_, ok := pool.Get()
	expect.False(t, ok, "typed pool returned an entry after drain")
}

func TestTypedWeakPoolConcurrent(t *testing.T) {
	const (
		workers    = 8
		iterations = 10_000
	)

	pool := newTypedWeakPool(64)
	var gets atomic.Int64
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			for i := range iterations {
				pool.Put(weakBuf{cap: worker*iterations + i + 1})
				if _, ok := pool.Get(); ok {
					gets.Add(1)
				}
			}
		})
	}
	wg.Wait()
	if gets.Load() <= 0 {
		t.Fatalf("expected positive, got %v", gets.Load())
	}
}
