package cache

import (
	"context"
	"sync"
	"testing"
)

func TestBuildMoreThan32BoundedCachesRegression(t *testing.T) {
	for i := range 96 {
		fn := NewKeyFunc(func(_ context.Context, key int) (int, error) { return key, nil }).WithMaxEntries(1).Build()
		for _, key := range []int{i, i + 1} {
			got, err := fn(t.Context(), key)
			if err != nil || got != key {
				t.Fatalf("cache %d: got %d, %v", i, got, err)
			}
		}
	}
}

func TestJanitorConcurrentGrowthCleanupRegression(t *testing.T) {
	j := &statesJanitor{signal: make(chan *state, cleanupQueueSize)}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 64 {
				s := &mockState{}
				idx := j.Add(s, 0)
				j.TriggerCleanup(idx)
				j.CleanupAll()
			}
		})
	}
	wg.Go(func() {
		for range 512 {
			select {
			case s := <-j.signal:
				j.cleanupTriggered(s)
			default:
			}
			j.CleanupAll()
		}
	})
	wg.Wait()
	for {
		select {
		case s := <-j.signal:
			j.cleanupTriggered(s)
		default:
			goto drained
		}
	}
drained:
	j.CleanupAll()
	if int(j.next.Load()) != 512 {
		t.Fatalf("states = %d, want 512", int(j.next.Load()))
	}
	for i := range int(j.next.Load()) {
		s := j.loadState(i)
		if s.State.(*mockState).CleanupCount() == 0 {
			t.Errorf("state %d was never cleaned", i)
		}
	}
}

func TestJanitorCleanupCanRegisterStateRegression(t *testing.T) {
	j := &statesJanitor{signal: make(chan *state, cleanupQueueSize)}
	second := &mockState{}
	var once sync.Once
	first := &mockState{cleanupFunc: func() { once.Do(func() { j.Add(second, 0) }) }}
	j.Add(first, 0)
	j.CleanupAll()
	if first.CleanupCount() != 1 || second.CleanupCount() != 0 {
		t.Fatal("new registration must wait for the next cleanup sweep")
	}
	j.CleanupAll()
	if second.CleanupCount() != 1 {
		t.Fatalf("new state cleanup count=%d", second.CleanupCount())
	}
}

func TestJanitorConcurrentIndexPublication(t *testing.T) {
	j := &statesJanitor{signal: make(chan *state, cleanupQueueSize)}
	type registration struct {
		idx int
		s   *mockState
	}
	const count = 1024
	registered := make(chan registration, count)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range count / 16 {
				s := &mockState{}
				idx := j.Add(s, 0)
				// Completed registration must already be usable, even while
				// other goroutines are extending the directory.
				j.TriggerCleanup(idx)
				registered <- registration{idx, s}
			}
		})
	}
	wg.Wait()
	close(registered)
	seen := make([]bool, count)
	for r := range registered {
		if r.idx < 0 || r.idx >= count || seen[r.idx] {
			t.Fatalf("duplicate or invalid index %d", r.idx)
		}
		seen[r.idx] = true
		if got := j.loadState(r.idx).State; got != r.s {
			t.Fatalf("index %d refers to a different registration", r.idx)
		}
	}
	for {
		select {
		case s := <-j.signal:
			j.cleanupTriggered(s)
		default:
			j.CleanupAll()
			for idx, present := range seen {
				if !present || j.loadState(idx).State.(*mockState).CleanupCount() == 0 {
					t.Fatalf("registration %d lost or not cleaned", idx)
				}
			}
			return
		}
	}
}

func TestJanitorSweepSkipsUnpublishedSlot(t *testing.T) {
	j := &statesJanitor{signal: make(chan *state, cleanupQueueSize)}
	// Model an Add paused after reserving index zero. A later registration
	// must be usable without waiting for the earlier one to publish.
	j.next.Add(1)
	later := &mockState{}
	if idx := j.Add(later, 0); idx != 1 {
		t.Fatalf("index=%d, want 1", idx)
	}
	j.CleanupAll()
	if later.CleanupCount() != 1 {
		t.Fatal("unpublished slot blocked cleanup of a later registration")
	}
	earlier := &mockState{}
	(*j.blocks.Load())[0][0].Store(&state{State: earlier})
	j.CleanupAll()
	if earlier.CleanupCount() != 1 || later.CleanupCount() != 2 {
		t.Fatal("a delayed publication was lost")
	}
}
