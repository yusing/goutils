package events

import (
	"context"
	"encoding/json"
	"io"
	"sync"
)

const maxHistorySize = 100
const listenerChanBufSize = 64

type History struct {
	events [maxHistorySize]Event
	index  int
	count  int

	listeners   map[*listener]struct{}
	listenersMu sync.RWMutex

	mu sync.RWMutex
}

type listener struct {
	ch chan Event
	mu sync.Mutex
}

var Global = NewHistory()

func NewHistory() *History {
	return &History{
		listeners: make(map[*listener]struct{}),
	}
}

func (h *History) Add(event Event) {
	h.mu.Lock()
	h.addToArrayLocked(event)
	listeners := h.listenersSnapshotLocked()
	h.mu.Unlock()

	h.notifyListeners(event, listeners)
}

func (h *History) AddAll(events []Event) {
	h.mu.Lock()
	for _, event := range events {
		h.addToArrayLocked(event)
	}
	listeners := h.listenersSnapshotLocked()
	h.mu.Unlock()

	h.notifyListenersAll(events, listeners)
}

func (h *History) addToArrayLocked(event Event) {
	h.events[h.index] = event
	h.index = (h.index + 1) % maxHistorySize
	if h.count < maxHistorySize {
		h.count++
	}
}

func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = [maxHistorySize]Event{}
	h.index = 0
	h.count = 0
}

func (h *History) listenersSnapshotLocked() []*listener {
	h.listenersMu.RLock()
	listeners := make([]*listener, 0, len(h.listeners))
	for l := range h.listeners {
		listeners = append(listeners, l)
	}
	h.listenersMu.RUnlock()
	return listeners
}

func (h *History) notifyListeners(event Event, listeners []*listener) {
	for _, l := range listeners {
		l.mu.Lock()
		if l.ch == nil { // channel is closed
			l.mu.Unlock()
			continue
		}
		select {
		case l.ch <- event:
		default:
			// channel full, drop event
		}
		l.mu.Unlock()
	}
}

func (h *History) notifyListenersAll(events []Event, listeners []*listener) {
	for _, l := range listeners {
		l.mu.Lock()
		if l.ch == nil { // channel is closed
			l.mu.Unlock()
			continue
		}
	nextListener:
		for _, event := range events {
			select {
			case l.ch <- event:
			default:
				// channel full, drop events
				continue nextListener
			}
		}
		l.mu.Unlock()
	}
}

// Get returns a copy of the current events in the history.
func (h *History) Get() []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.snapshotLocked()
}

// SnapshotAndListen atomically captures current history and registers a listener.
func (h *History) SnapshotAndListen() (current []Event, ch <-chan Event, cancel func()) {
	l := &listener{ch: make(chan Event, listenerChanBufSize)}

	h.mu.Lock()
	current = h.snapshotLocked()

	h.listenersMu.Lock()
	h.listeners[l] = struct{}{}
	h.listenersMu.Unlock()
	h.mu.Unlock()

	var once sync.Once
	ch = l.ch
	cancel = func() {
		once.Do(func() {
			h.listenersMu.Lock()
			delete(h.listeners, l)
			h.listenersMu.Unlock()

			l.mu.Lock()
			if l.ch != nil {
				close(l.ch)
				l.ch = nil
			}
			l.mu.Unlock()
		})
	}
	return
}

// snapshotLocked returns the current history snapshot.
// h.mu must be held by the caller.
func (h *History) snapshotLocked() []Event {
	res := make([]Event, h.count)
	if h.count < maxHistorySize {
		copy(res, h.events[:h.count])
	} else {
		copy(res, h.events[h.index:])
		copy(res[maxHistorySize-h.index:], h.events[:h.index])
	}
	return res
}

// ListenJSON listens for events and writes them to the writer in JSON format.
//
// It does send the current events to the writer.
func (h *History) ListenJSON(ctx context.Context, w io.Writer) error {
	current, ch, cancel := h.SnapshotAndListen()
	defer cancel()

	enc := json.NewEncoder(w)

	for _, event := range current {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := enc.Encode(event); err != nil {
				return err
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-ch:
			if err := enc.Encode(event); err != nil {
				return err
			}
		}
	}
}
