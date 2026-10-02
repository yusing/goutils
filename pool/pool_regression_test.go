package pool

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yusing/goutils/events"
)

type regressionObject struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
}

type blockingNameObject struct {
	id      string
	block   atomic.Bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (o *blockingNameObject) Key() string { return o.id }
func (o *blockingNameObject) Name() string {
	if o.block.Load() {
		o.once.Do(func() { close(o.entered) })
		<-o.release
	}
	return o.id
}

func TestClearDoesNotWaitForOldMutation(t *testing.T) {
	p := New[*blockingNameObject]("test", "test")
	p.DisableLog(true)
	old := &blockingNameObject{id: "a", entered: make(chan struct{}), release: make(chan struct{})}
	p.Add(old)
	old.block.Store(true)
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(old.release) }) })
	deleted := make(chan struct{})
	go func() { p.DelKey("a"); close(deleted) }()
	select {
	case <-old.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not reach the old object")
	}
	cleared := make(chan struct{})
	fresh := &blockingNameObject{id: "a"}
	go func() { p.Clear(); p.Add(fresh); close(cleared) }()
	select {
	case <-cleared:
	case <-time.After(5 * time.Second):
		t.Fatal("Clear or Add blocked on the old generation")
	}
	release.Do(func() { close(old.release) })
	select {
	case <-deleted:
	case <-time.After(5 * time.Second):
		t.Fatal("old delete did not finish")
	}
	if got, ok := p.Get("a"); !ok || got != fresh {
		t.Fatal("old delete affected the new registry")
	}
	if p.state.Load().tombs.Load() != 0 {
		t.Fatal("old delete affected the new tombstone count")
	}
}

func TestConcurrentTombstoneTransitions(t *testing.T) {
	p := regressionPool()
	s := p.state.Load()
	for i := range 128 {
		key := fmt.Sprint(i)
		s.m.Store(key, entry[regressionObject]{tomb: true, removed: removedInfo{name: key, removedAt: time.Now().Add(-2 * time.Second)}})
	}
	s.tombs.Store(128)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range 128 {
				key := fmt.Sprint(i)
				p.AddIfNotExists(regressionObject{ID: key})
				p.DelKey(key)
				p.Add(regressionObject{ID: key})
				p.PurgeExpiredTombs()
			}
		})
	}
	wg.Wait()
	var tombs int64
	for _, v := range s.m.Range {
		if v.tomb {
			tombs++
		}
	}
	if got := s.tombs.Load(); got != tombs {
		t.Fatalf("tombstone count=%d, actual=%d", got, tombs)
	}
	// Every writer finishes each key by adding it. Purging may not delete
	// that replacement even if it previously observed an expired tombstone.
	if got := len(p.Slice()); got != 128 || tombs != 0 {
		t.Fatalf("live=%d tombs=%d, want 128 live entries", got, tombs)
	}
}

func (o regressionObject) Key() string  { return o.ID }
func (o regressionObject) Name() string { return o.ID }
func (o regressionObject) PreferOver(other any) bool {
	return o.Priority > other.(regressionObject).Priority
}

func regressionPool() *Pool[regressionObject] {
	p := New[regressionObject]("test", "test")
	p.DisableLog(true)
	return p
}

func TestSliceAfterDeleteClear(t *testing.T) {
	p := regressionPool()
	p.Add(regressionObject{ID: "a"})
	p.DelKey("a")
	p.Clear()
	if got := p.Slice(); len(got) != 0 {
		t.Fatalf("Slice = %v", got)
	}
	p.Add(regressionObject{ID: "b"})
	if p.state.Load().tombs.Load() != 0 || len(p.Slice()) != 1 {
		t.Fatal("stale tombstone accounting")
	}
}

func TestAddIfNotExistsRemovedKeys(t *testing.T) {
	for _, age := range []time.Duration{0, 2 * time.Second} {
		t.Run(age.String(), func(t *testing.T) {
			p := regressionPool()
			p.state.Load().m.Store("a", entry[regressionObject]{tomb: true, removed: removedInfo{name: "a", removedAt: time.Now().Add(-age)}})
			p.state.Load().tombs.Store(1)
			obj := regressionObject{ID: "a", Priority: 1}
			if actual, added := p.AddIfNotExists(obj); !added || actual != obj {
				t.Fatalf("actual=%v added=%v", actual, added)
			}
			if p.state.Load().tombs.Load() != 0 {
				t.Fatal("tombstone not consumed")
			}
			if actual, added := p.AddIfNotExists(regressionObject{ID: "a", Priority: 2}); added || actual != obj {
				t.Fatalf("live entry replaced: %v %v", actual, added)
			}
		})
	}
}

func TestConcurrentPoolWriters(t *testing.T) {
	p := regressionPool()
	var wg sync.WaitGroup
	var added atomic.Int32
	for range 128 {
		wg.Go(func() {
			if _, ok := p.AddIfNotExists(regressionObject{ID: "a"}); ok {
				added.Add(1)
			}
		})
	}
	wg.Wait()
	if added.Load() != 1 {
		t.Fatalf("added %d times", added.Load())
	}
	for priority := range 128 {
		wg.Go(func() { p.AddKey("a", regressionObject{ID: "a", Priority: priority}) })
	}
	wg.Wait()
	if actual, _ := p.Get("a"); actual.Priority != 127 {
		t.Fatalf("preferable result=%v", actual)
	}
	for range 128 {
		wg.Go(func() {
			p.DelKey("a")
			p.Add(regressionObject{ID: "a"})
			p.Clear()
			_ = p.Slice()
			p.PurgeExpiredTombs()
		})
	}
	wg.Wait()
	p.Clear()
	if p.state.Load().tombs.Load() != 0 || len(p.Slice()) != 0 {
		t.Fatal("inconsistent final accounting")
	}
}

type cancelWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.cancel()
	return n, err
}
func TestRemovedEventListenJSON(t *testing.T) {
	p := regressionPool()
	history := events.NewHistory()
	p.SetEventHistory(history)
	p.state.Load().m.Store("a", entry[regressionObject]{tomb: true, removed: removedInfo{name: "a", display: "A", removedAt: time.Now().Add(-2 * time.Second)}})
	p.state.Load().tombs.Store(1)
	if n := p.PurgeExpiredTombs(); n != 1 {
		t.Fatalf("purged %d", n)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	writer := &cancelWriter{cancel: cancel}
	if err := history.ListenJSON(ctx, writer); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListenJSON: %v", err)
	}
	var event struct {
		Action string `json:"action"`
		Data   struct {
			Name      string    `json:"name"`
			Display   string    `json:"display"`
			RemovedAt time.Time `json:"removed_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(writer.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.Data.Name != "a" || event.Data.Display != "A" || event.Data.RemovedAt.IsZero() {
		t.Fatalf("event=%+v", event)
	}
}

func TestPurgeKeepsRecentAndLiveEntries(t *testing.T) {
	p := regressionPool()
	history := events.NewHistory()
	p.SetEventHistory(history)
	p.state.Load().m.Store("expired", entry[regressionObject]{tomb: true, removed: removedInfo{name: "expired", removedAt: time.Now().Add(-2 * time.Second)}})
	p.state.Load().m.Store("recent", entry[regressionObject]{tomb: true, removed: removedInfo{name: "recent", removedAt: time.Now()}})
	p.state.Load().m.Store("live", entry[regressionObject]{obj: regressionObject{ID: "live"}})
	p.state.Load().tombs.Store(2)
	if n := p.PurgeExpiredTombs(); n != 1 {
		t.Fatalf("purged %d, want 1", n)
	}
	if p.state.Load().tombs.Load() != 1 || p.Size() != 2 {
		t.Fatalf("tombs=%d size=%d", p.state.Load().tombs.Load(), p.Size())
	}
	if _, exists := p.state.Load().m.Load("recent"); !exists {
		t.Fatal("recent tombstone purged")
	}
	if _, exists := p.Get("live"); !exists {
		t.Fatal("live entry purged")
	}
	if events := history.Get(); len(events) != 1 {
		t.Fatalf("events=%v", events)
	}
	if n := p.PurgeExpiredTombs(); n != 0 {
		t.Fatalf("second purge removed %d entries", n)
	}
}
