package cache

import (
	"fmt"
	"sync/atomic"
	"time"
)

var Janitor = newStatesJanitor()

type State interface {
	// Cleanup must be concurrency-safe.
	Cleanup()
}

type state struct {
	State

	cleanupInterval time.Duration
	lastCleanup     time.Time
	pendingCleanup  atomic.Bool
}

const maxStatesPerJanitor = 32

type statesJanitor struct {
	states    [maxStatesPerJanitor]*state
	numStates atomic.Int32
	signal    chan *state
}

func newStatesJanitor() *statesJanitor {
	j := &statesJanitor{
		signal: make(chan *state, maxStatesPerJanitor),
	}
	go j.runLoop()
	return j
}

// Add adds a new state to the janitor. Once the state is added,
// it cannot be removed. The cleanupInterval is the minimum time
// between cleanups for this state.
func (j *statesJanitor) Add(s State, cleanupInterval time.Duration) int {
	idx := int(j.numStates.Add(1)) - 1
	if idx >= len(j.states) {
		panic(fmt.Sprintf("too many states: %d", idx))
	}
	j.states[idx] = &state{State: s, cleanupInterval: cleanupInterval}
	return idx
}

func (j *statesJanitor) TriggerCleanup(idx int) {
	if idx < 0 || idx >= len(j.states) {
		panic(fmt.Sprintf("invalid state index: %d", idx))
	}
	state := j.states[idx]
	if !state.pendingCleanup.CompareAndSwap(false, true) {
		// already triggered
		return
	}
	select {
	case j.signal <- state:
	default:
	}
}

func (j *statesJanitor) CleanupAll() {
	states := j.states[:j.numStates.Load()]
	for _, s := range states {
		if !s.pendingCleanup.CompareAndSwap(false, true) {
			// already triggered, will be handled in case s := <-j.signal below
			continue
		}
		j.cleanup(s)
		s.pendingCleanup.Store(false)
	}
}

func (j *statesJanitor) cleanup(s *state) {
	now := time.Now()
	if now.Sub(s.lastCleanup) < s.cleanupInterval {
		// skip cleanup if it's too soon, must've been triggered recently
		return
	}
	s.Cleanup()
	s.lastCleanup = time.Now()
}

func (j *statesJanitor) runLoop() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C: // background cleanup
			j.CleanupAll()
		case s := <-j.signal: // active cleanup
			j.cleanup(s)
		}
	}
}
