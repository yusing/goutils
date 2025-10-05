package cache

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockState implements the State interface for testing
type mockState struct {
	cleanupCount atomic.Int32
	cleanupFunc  func()
}

func (m *mockState) Cleanup() {
	m.cleanupCount.Add(1)
	if m.cleanupFunc != nil {
		m.cleanupFunc()
	}
}

func (m *mockState) CleanupCount() int {
	return int(m.cleanupCount.Load())
}

func TestStatesJanitor_Add(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	// Test adding states within capacity
	mock1 := &mockState{}
	mock2 := &mockState{}

	idx1 := j.Add(mock1, time.Minute)
	if idx1 != 0 {
		t.Errorf("Expected first state index to be 0, got %d", idx1)
	}

	idx2 := j.Add(mock2, time.Minute)
	if idx2 != 1 {
		t.Errorf("Expected second state index to be 1, got %d", idx2)
	}

	if j.numStates.Load() != 2 {
		t.Errorf("Expected numStates to be 2, got %d", j.numStates.Load())
	}

	// Test that states are properly stored
	if j.states[0].State != mock1 {
		t.Error("First state not stored correctly")
	}
	if j.states[1].State != mock2 {
		t.Error("Second state not stored correctly")
	}
}

func TestStatesJanitor_AddPanicOnOverflow(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	// Fill up to capacity
	for range maxStatesPerJanitor {
		j.Add(&mockState{}, time.Minute)
	}

	// Should panic when trying to add beyond capacity
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when adding too many states")
		}
	}()

	j.Add(&mockState{}, time.Minute)
}

func TestStatesJanitor_TriggerCleanup(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	mock := &mockState{}
	idx := j.Add(mock, time.Minute)

	// Test valid trigger
	j.TriggerCleanup(idx)

	// Manually process the signal
	select {
	case state := <-j.signal:
		j.cleanup(state)
	case <-time.After(time.Second):
		t.Fatal("Cleanup not triggered within timeout")
	}

	if mock.CleanupCount() != 1 {
		t.Errorf("Expected 1 cleanup, got %d", mock.CleanupCount())
	}
}

func TestStatesJanitor_TriggerCleanupInvalidIndex(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	// Test negative index
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for negative index")
		}
	}()
	j.TriggerCleanup(-1)
}

func TestStatesJanitor_TriggerCleanupIndexTooLarge(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	// Test index too large
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for index too large")
		}
	}()
	j.TriggerCleanup(100)
}

func TestStatesJanitor_CleanupAll(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	mock1 := &mockState{}
	mock2 := &mockState{}
	mock3 := &mockState{}

	j.Add(mock1, time.Minute)
	j.Add(mock2, time.Minute)
	j.Add(mock3, time.Minute)

	j.CleanupAll()

	if mock1.CleanupCount() != 1 {
		t.Errorf("Expected mock1 to have 1 cleanup, got %d", mock1.CleanupCount())
	}
	if mock2.CleanupCount() != 1 {
		t.Errorf("Expected mock2 to have 1 cleanup, got %d", mock2.CleanupCount())
	}
	if mock3.CleanupCount() != 1 {
		t.Errorf("Expected mock3 to have 1 cleanup, got %d", mock3.CleanupCount())
	}
}

func TestStatesJanitor_CleanupInterval(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	mock := &mockState{}
	interval := 100 * time.Millisecond
	idx := j.Add(mock, interval)

	// First cleanup should succeed
	j.cleanup(j.states[idx])
	if mock.CleanupCount() != 1 {
		t.Errorf("Expected 1 cleanup, got %d", mock.CleanupCount())
	}

	// Immediate second cleanup should be skipped
	j.cleanup(j.states[idx])
	if mock.CleanupCount() != 1 {
		t.Errorf("Expected cleanup to be skipped, got %d cleanups", mock.CleanupCount())
	}

	// Wait for interval to pass
	time.Sleep(interval + 10*time.Millisecond)

	// Third cleanup should succeed
	j.cleanup(j.states[idx])
	if mock.CleanupCount() != 2 {
		t.Errorf("Expected 2 cleanups, got %d", mock.CleanupCount())
	}
}

func TestStatesJanitor_ConcurrentCleanup(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	mock := &mockState{}
	idx := j.Add(mock, time.Millisecond)

	// Start multiple goroutines triggering cleanup
	var wg sync.WaitGroup
	numGoroutines := 10

	for range numGoroutines {
		wg.Go(func() {
			j.TriggerCleanup(idx)
		})
	}

	// Process cleanups in background
	go func() {
		for {
			select {
			case state := <-j.signal:
				j.cleanup(state)
			default:
				if mock.CleanupCount() > 0 {
					return
				}
				time.Sleep(time.Millisecond)
			}
		}
	}()

	wg.Wait()

	// Give some time for cleanups to process
	time.Sleep(50 * time.Millisecond)

	// Multiple triggers should only trigger once
	if mock.CleanupCount() != 1 {
		t.Errorf("Expected 1 cleanup, got %d", mock.CleanupCount())
	}
}

func TestStatesJanitor_TriggerCleanupIdempotent(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	mock := &mockState{}
	idx := j.Add(mock, time.Minute)

	// Trigger cleanup multiple times quickly
	j.TriggerCleanup(idx)
	j.TriggerCleanup(idx)
	j.TriggerCleanup(idx)

	// Should only process one cleanup
	select {
	case state := <-j.signal:
		j.cleanup(state)
	case <-time.After(time.Second):
		t.Fatal("Expected at least one cleanup signal")
	}

	// Channel should be empty (no duplicate signals)
	select {
	case <-j.signal:
		t.Error("Unexpected duplicate cleanup signal")
	default:
		// Expected - no signal
	}

	if mock.CleanupCount() != 1 {
		t.Errorf("Expected 1 cleanup, got %d", mock.CleanupCount())
	}
}

func TestStatesJanitor_CleanupAllWithPendingCleanup(t *testing.T) {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}

	mock := &mockState{}
	idx := j.Add(mock, time.Minute)

	// Trigger cleanup first
	j.TriggerCleanup(idx)

	// Then call CleanupAll
	j.CleanupAll()

	// Should have triggered cleanup twice (once from trigger, once from CleanupAll)
	time.Sleep(50 * time.Millisecond) // Give time for signal processing

	// Process any pending signals
	for {
		select {
		case state := <-j.signal:
			j.cleanup(state)
		default:
			goto done
		}
	}
done:

	// Should only trigger once
	if mock.CleanupCount() != 1 {
		t.Errorf("Expected 1 cleanup, got %d", mock.CleanupCount())
	}
}

func TestMockState(t *testing.T) {
	mock := &mockState{}

	if mock.CleanupCount() != 0 {
		t.Errorf("Expected initial cleanup count to be 0, got %d", mock.CleanupCount())
	}

	cleanupCalled := false
	mock.cleanupFunc = func() {
		cleanupCalled = true
	}

	mock.Cleanup()

	if mock.CleanupCount() != 1 {
		t.Errorf("Expected cleanup count to be 1, got %d", mock.CleanupCount())
	}

	if !cleanupCalled {
		t.Error("Expected cleanup function to be called")
	}
}
