package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildWithReleaseRemovesRegistration(t *testing.T) {
	idx := int(Janitor.next.Load())
	var calls atomic.Int32
	fn, release := NewKeyFunc(func(_ context.Context, key int) (int, error) {
		calls.Add(1)
		return key, nil
	}).WithMaxEntries(1).BuildWithRelease()
	t.Cleanup(release)
	if Janitor.loadState(idx) == nil {
		t.Fatal("bounded cache was not registered")
	}
	if got, err := fn(t.Context(), 7); err != nil || got != 7 {
		t.Fatalf("lookup = %d, %v", got, err)
	}
	release()
	release()
	if Janitor.loadState(idx) != nil {
		t.Fatal("released cache remains in the janitor slot")
	}
	Janitor.TriggerCleanup(idx)
	// Release only unregisters cleanup, so existing callers may still finish
	// or use cached data without a new lifetime/closed-state error.
	if got, err := fn(t.Context(), 7); err != nil || got != 7 || calls.Load() != 1 {
		t.Fatalf("cached lookup after release = %d, %v; calls = %d", got, err, calls.Load())
	}

	otherIdx := int(Janitor.next.Load())
	builder := NewKeyFunc(func(_ context.Context, key int) (int, error) { return key, nil }).WithMaxEntries(1)
	// Use no throttle so this test exercises janitor eviction without waiting
	// for the normal production cleanup interval.
	builder.cleanupInterval = 0
	other, releaseOther := builder.BuildWithRelease()
	t.Cleanup(releaseOther)
	if otherIdx <= idx {
		t.Fatal("registration indices must not be reused")
	}
	otherState := Janitor.loadState(otherIdx).State.(*CachedContextKeyFuncState[int, int])
	for _, key := range []int{1, 2, 3} {
		if got, err := other(t.Context(), key); err != nil || got != key {
			t.Fatalf("other lookup = %d, %v", got, err)
		}
	}
	release() // A stale release must not affect the next registration.
	deadline := time.Now().Add(3 * time.Second)
	for otherState.entries.Size() > 1 && time.Now().Before(deadline) {
		Janitor.CleanupAll()
		time.Sleep(time.Millisecond)
	}
	if got := otherState.entries.Size(); got != 1 {
		t.Fatalf("other cache entries = %d, want 1 after eviction", got)
	}
	if Janitor.loadState(otherIdx) == nil {
		t.Fatal("releasing the old cache removed another registration")
	}
}

func TestBuildWithReleaseUnbounded(t *testing.T) {
	// An unbounded cache must not accidentally remove slot zero (its default index).
	previous := Janitor
	Janitor = &statesJanitor{signal: make(chan *state, cleanupQueueSize)}
	t.Cleanup(func() { Janitor = previous })
	sentinel := &mockState{}
	idx := Janitor.Add(sentinel, 0)
	for _, limit := range []int{0, -1} {
		before := Janitor.next.Load()
		fn, release := NewKeyFunc(func(_ context.Context, key int) (int, error) { return key, nil }).WithMaxEntries(limit).BuildWithRelease()
		release()
		release()
		if Janitor.next.Load() != before || Janitor.loadState(idx) == nil {
			t.Fatalf("limit %d changed janitor registrations", limit)
		}
		Janitor.CleanupAll()
		if got, err := fn(t.Context(), 9); err != nil || got != 9 {
			t.Fatalf("lookup = %d, %v", got, err)
		}
	}
	if sentinel.CleanupCount() != 2 {
		t.Fatal("unbounded cache release disabled another cache's cleanup")
	}
}

func TestBuildWithReleaseInFlightCall(t *testing.T) {
	entered := make(chan struct{})
	resume := make(chan struct{})
	var unblock sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(resume) }) })
	fn, release := NewKeyFunc(func(_ context.Context, key int) (int, error) {
		close(entered)
		<-resume
		return key, nil
	}).WithMaxEntries(1).BuildWithRelease()
	t.Cleanup(release)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if got, err := fn(t.Context(), 42); err != nil || got != 42 {
			t.Errorf("in-flight lookup = %d, %v", got, err)
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("lookup did not start")
	}
	release()
	release()
	unblock.Do(func() { close(resume) })
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("lookup did not finish after release")
	}
}

func TestBuildWithReleaseConcurrentCallsAndCleanup(t *testing.T) {
	idx := int(Janitor.next.Load())
	fn, release := NewKeyFunc(func(_ context.Context, key int) (int, error) { return key, nil }).WithMaxEntries(8).BuildWithRelease()
	t.Cleanup(release)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Go(func() {
			<-start
			for i := range 128 {
				key := worker*128 + i
				if got, err := fn(t.Context(), key); err != nil || got != key {
					t.Errorf("lookup = %d, %v, want %d", got, err, key)
					return
				}
			}
		})
	}
	wg.Go(func() {
		<-start
		for range 128 {
			Janitor.TriggerCleanup(idx)
			Janitor.CleanupAll()
		}
	})
	for range 2 {
		wg.Go(func() {
			<-start
			for range 128 {
				release()
			}
		})
	}
	close(start)
	wg.Wait()
	if Janitor.loadState(idx) != nil {
		t.Fatal("concurrently released cache remains registered")
	}
}

func TestJanitorRemoveAllowsInFlightCleanup(t *testing.T) {
	j := &statesJanitor{signal: make(chan *state, cleanupQueueSize)}
	entered := make(chan struct{})
	resume := make(chan struct{})
	var unblock sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(resume) }) })
	s := &mockState{cleanupFunc: func() {
		close(entered)
		<-resume
	}}
	idx := j.Add(s, 0)
	done := make(chan struct{})
	go func() {
		defer close(done)
		j.CleanupAll()
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup did not start")
	}
	j.Remove(idx)
	j.Remove(idx)
	if j.loadState(idx) != nil {
		t.Fatal("removed slot is not nil")
	}
	j.TriggerCleanup(idx)
	if len(j.signal) != 0 {
		t.Fatal("removed slot queued another cleanup")
	}
	unblock.Do(func() { close(resume) })
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight cleanup did not finish")
	}
	j.CleanupAll()
	if got := s.CleanupCount(); got != 1 {
		t.Fatalf("cleanup count = %d, want only the in-flight cleanup", got)
	}
}
