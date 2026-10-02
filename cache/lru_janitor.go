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

const (
	cleanupQueueSize = 32
	stateBlockSize   = 32
)

type stateBlock [stateBlockSize]atomic.Pointer[state]
type stateBlocks []*stateBlock

type statesJanitor struct {
	blocks atomic.Pointer[stateBlocks] // immutable directory; slots are published separately
	next   atomic.Int64
	signal chan *state
}

func newStatesJanitor() *statesJanitor {
	j := &statesJanitor{
		signal: make(chan *state, cleanupQueueSize),
	}
	go j.runLoop()
	return j
}

// Add adds a new state to the janitor. The cleanupInterval is the minimum time
// between cleanups for this state.
func (j *statesJanitor) Add(s State, cleanupInterval time.Duration) int {
	idx := int(j.next.Add(1) - 1)
	blockIdx := idx / stateBlockSize
	for {
		old := j.blocks.Load()
		if old != nil && blockIdx < len(*old) {
			(*old)[blockIdx][idx%stateBlockSize].Store(&state{State: s, cleanupInterval: cleanupInterval})
			return idx
		}
		var blocks stateBlocks
		if old != nil {
			blocks = make(stateBlocks, max(blockIdx+1, 2*len(*old)))
			copy(blocks, *old)
		} else {
			blocks = make(stateBlocks, blockIdx+1)
		}
		for i := range blocks {
			if blocks[i] == nil {
				blocks[i] = new(stateBlock)
			}
		}
		j.blocks.CompareAndSwap(old, &blocks)
	}
}

func (j *statesJanitor) loadState(idx int) *state {
	blocks := j.blocks.Load()
	if idx < 0 || int64(idx) >= j.next.Load() || blocks == nil || idx/stateBlockSize >= len(*blocks) {
		panic(fmt.Sprintf("invalid state index: %d", idx))
	}
	return (*blocks)[idx/stateBlockSize][idx%stateBlockSize].Load()
}

// Remove releases the janitor's reference to a state. It is idempotent.
// Already queued cleanups may still run. Indices are never reused.
func (j *statesJanitor) Remove(idx int) {
	j.loadState(idx) // validate the index, including after a previous removal
	blocks := j.blocks.Load()
	(*blocks)[idx/stateBlockSize][idx%stateBlockSize].Store(nil)
}

func (j *statesJanitor) TriggerCleanup(idx int) {
	state := j.loadState(idx)
	if state == nil || !state.pendingCleanup.CompareAndSwap(false, true) {
		// already triggered
		return
	}
	select {
	case j.signal <- state:
	default:
		state.pendingCleanup.Store(false)
	}
}

func (j *statesJanitor) CleanupAll() {
	// Fix the frontier before callbacks can register additional states. Add may have
	// reserved an index without publishing its slot yet; skip it until a later sweep.
	n := int(j.next.Load())
	blocks := j.blocks.Load()
	if blocks == nil {
		return
	}
	for idx := range min(n, len(*blocks)*stateBlockSize) {
		s := (*blocks)[idx/stateBlockSize][idx%stateBlockSize].Load()
		if s == nil {
			continue
		}
		if !s.pendingCleanup.CompareAndSwap(false, true) {
			// already triggered, will be handled in case s := <-j.signal below
			continue
		}
		j.cleanupTriggered(s)
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

func (j *statesJanitor) cleanupTriggered(s *state) {
	defer s.pendingCleanup.Store(false)
	j.cleanup(s)
}

func (j *statesJanitor) runLoop() {
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C: // background cleanup
			j.CleanupAll()
		case s := <-j.signal: // active cleanup
			j.cleanupTriggered(s)
		}
	}
}
